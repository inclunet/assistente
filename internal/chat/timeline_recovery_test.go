package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/database"
)

type recoveryReader struct {
	read func() (*TimelineTurnSnapshot, error)
}

func (r recoveryReader) ReadTurn(context.Context, string, string) (*TimelineTurnSnapshot, error) {
	return r.read()
}
func (r recoveryReader) ReadVersion(context.Context, string) (database.ChatTimelineVersion, error) {
	panic("somente fixture de recuperação")
}

func recoveryStored(active string, revision int64) *TimelineTurnSnapshot {
	t := &database.ChatTurn{UserMessageID: "question", ConversationID: "conversation"}
	if active != "" {
		t.ActiveExecutionID = &active
	}
	return &TimelineTurnSnapshot{Version: database.ChatTimelineVersion{Incarnation: "storage", Revision: revision}, Turn: t}
}

func TestTurnRecoverySnapshotDeltaBarrier(t *testing.T) {
	p, ctx, e := liveFixture(t)
	s := e.StartStream(0)
	s.Append("antes")
	reading, release := make(chan struct{}), make(chan struct{})
	cleanupRecoveryGate(t, release)
	done := make(chan *TurnRecovery, 1)
	go func() {
		r, _ := p.ReadRecovery(ctx, "conversation", "question", recoveryReader{func() (*TimelineTurnSnapshot, error) {
			close(reading)
			<-release
			return recoveryStored("execution", 2), nil
		}})
		done <- r
	}()
	<-reading
	appended := make(chan struct{})
	go func() { s.Append(" depois"); close(appended) }()
	select {
	case <-appended:
		t.Fatal("delta atravessou snapshot")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	r := <-done
	<-appended
	if r.Active == nil || testLiveSegment(r.Active.Streams[0], "text", 0).Text != "antes" || r.CanUsePersistedPatch("execution") {
		t.Fatalf("snapshot incoerente: %+v", r)
	}
	snap, _ := p.Snapshot(ctx, "conversation", "question")
	if testLiveSegment(snap.Streams[0], "text", 0).Text != "antes depois" {
		t.Fatal("delta perdido")
	}
}

func TestTurnRecoveryFirstReadBlocksAdmissionOnlyForItsConversation(t *testing.T) {
	p := NewLiveProjection()
	ctx := database.WithUserID(context.Background(), "user")
	reading, release := make(chan struct{}), make(chan struct{})
	cleanupRecoveryGate(t, release)
	readDone := make(chan error, 1)
	go func() {
		_, err := p.ReadRecovery(ctx, "conversation", "question", recoveryReader{func() (*TimelineTurnSnapshot, error) { close(reading); <-release; return recoveryStored("", 1), nil }})
		readDone <- err
	}()
	<-reading
	admitted := make(chan error, 1)
	go func() {
		_, err := beginTestProjection(p, ctx, "conversation", "question", "execution")
		admitted <- err
	}()
	other := make(chan error, 1)
	go func() {
		_, err := beginTestProjection(p, ctx, "other", "other-question", "other-execution")
		other <- err
	}()
	select {
	case err := <-other:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("I/O bloqueou outra conversa")
	}
	select {
	case err := <-admitted:
		t.Fatalf("admissão atravessou primeira leitura: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if err := <-admitted; err != nil {
		t.Fatal(err)
	}
}

func TestTurnRecoveryTerminalCommitBarrierAndPositiveVersion(t *testing.T) {
	p, ctx, e := liveFixture(t)
	e.StartStream(0).Seal("final", "")
	writing, release := make(chan struct{}), make(chan struct{})
	cleanupRecoveryGate(t, release)
	committed := make(chan error, 1)
	version := database.ChatTimelineVersion{Incarnation: "storage", Revision: 9}
	go func() {
		committed <- e.CommitResponse(database.TurnCompleted, "completed", "final", "", func() (database.ChatTimelineVersion, error) { close(writing); <-release; return version, nil })
	}()
	<-writing
	readEntered := make(chan struct{})
	snapshot := make(chan *TurnRecovery, 1)
	go func() {
		r, _ := p.ReadRecovery(ctx, "conversation", "question", recoveryReader{func() (*TimelineTurnSnapshot, error) { close(readEntered); return recoveryStored("", 9), nil }})
		snapshot <- r
	}()
	select {
	case <-readEntered:
		t.Fatal("leitura atravessou commit")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-committed; err != nil {
		t.Fatal(err)
	}
	r := <-snapshot
	if !r.CanUsePersistedPatch("execution") || r.Active != nil || len(r.Retained) != 0 {
		t.Fatalf("commit não confirmado: %+v", r)
	}
	changed, _ := p.ReadRecovery(ctx, "conversation", "question", recoveryReader{func() (*TimelineTurnSnapshot, error) { return recoveryStored("", 10), nil }})
	if changed.CanUsePersistedPatch("execution") || !changed.RecoveryRequired {
		t.Fatal("versão externa atribuída ao terminal anterior")
	}
}

func TestTurnRecoveryOldTerminalCannotPublishNewAttempt(t *testing.T) {
	p, ctx, e := liveFixture(t)
	e.StartStream(0).Seal("A", "")
	if err := e.CommitResponse(database.TurnCompleted, "completed", "A", "", func() (database.ChatTimelineVersion, error) {
		return database.ChatTimelineVersion{Incarnation: "storage", Revision: 2}, nil
	}); err != nil {
		t.Fatal(err)
	}
	next, err := beginTestProjection(p, ctx, "conversation", "question", "retry")
	if err != nil {
		t.Fatal(err)
	}
	r, _ := p.ReadRecovery(ctx, "conversation", "question", recoveryReader{func() (*TimelineTurnSnapshot, error) { return recoveryStored("retry", 3), nil }})
	if r.CanUsePersistedPatch("execution") {
		t.Fatal("terminal A aceitou dados de B ativa")
	}
	next.StartStream(0).Seal("B", "")
	if err := next.CommitResponse(database.TurnCompleted, "completed", "B", "", func() (database.ChatTimelineVersion, error) {
		return database.ChatTimelineVersion{Incarnation: "storage", Revision: 4}, nil
	}); err != nil {
		t.Fatal(err)
	}
	r, _ = p.ReadRecovery(ctx, "conversation", "question", recoveryReader{func() (*TimelineTurnSnapshot, error) { return recoveryStored("", 4), nil }})
	if r.CanUsePersistedPatch("execution") || !r.CanUsePersistedPatch("retry") {
		t.Fatal("terminal A aceitou dados de B concluída")
	}
}

func TestTurnRecoveryFailureDoesNotExposeMemoryOrDiscard(t *testing.T) {
	p, ctx, e := liveFixture(t)
	e.StartStream(0).Append("privado")
	denied := errors.New("leitura recusada")
	r := recoveryReader{func() (*TimelineTurnSnapshot, error) { return nil, denied }}
	if result, err := p.ReadRecovery(ctx, "conversation", "question", r); result != nil || !errors.Is(err, denied) {
		t.Fatal("retornou memória sem autorização")
	}
	if err := p.DiscardRetained(ctx, "conversation", "question", "execution", r); !errors.Is(err, denied) {
		t.Fatal("descarte ignorou recusa")
	}
	snap, _ := p.Snapshot(ctx, "conversation", "question")
	if testLiveSegment(snap.Streams[0], "text", 0).Text != "privado" {
		t.Fatal("falha de leitura perdeu memória")
	}
}

func TestTurnRecoveryIncarnationDetachesAndPreservesContent(t *testing.T) {
	p, ctx, e := liveFixture(t)
	s := e.StartStream(0)
	s.Append("anterior")
	reader := recoveryReader{func() (*TimelineTurnSnapshot, error) {
		r := recoveryStored("", 1)
		r.Version.Incarnation = "replacement"
		return r, nil
	}}
	before, _ := p.Snapshot(ctx, "conversation", "question")
	r, err := p.ReadRecovery(ctx, "conversation", "question", reader)
	if err != nil || !r.RecoveryRequired || r.Epoch == before.Epoch || r.Active != nil || len(r.Retained) != 1 || !r.Retained[0].Detached {
		t.Fatalf("encarnação: %+v %v", r, err)
	}
	if s.Append("atrasado") {
		t.Fatal("aceitou callback após desvincular")
	}
	if err := p.DiscardRetained(ctx, "conversation", "question", "execution", reader); err != nil {
		t.Fatal(err)
	}
	if err := p.DiscardRetained(ctx, "conversation", "question", "execution", reader); err != nil {
		t.Fatal("descarte não idempotente")
	}
}

func TestTurnRecoveryRefusesDiscardOfActiveExecution(t *testing.T) {
	p, ctx, _ := liveFixture(t)
	r := recoveryReader{func() (*TimelineTurnSnapshot, error) { return recoveryStored("execution", 2), nil }}
	if err := p.DiscardRetained(ctx, "conversation", "question", "execution", r); !errors.Is(err, ErrProjectionActive) {
		t.Fatalf("descartou execução ativa: %v", err)
	}
}

func cleanupRecoveryGate(t *testing.T, ch chan struct{}) {
	t.Helper()
	t.Cleanup(func() {
		select {
		case <-ch:
		default:
			close(ch)
		}
	})
}
