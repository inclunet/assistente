package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

// O leitor não pode reter um cancelamento, exigir outra conexão do pool nem
// reutilizar dados/handles de um banco fechado após sua reabertura.
func TestTimelinePreparedOpeningCancellationConnectionAndReopen(t *testing.T) {
	f := newSnapshotFixture(t)
	raw, err := f.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, err := f.reader.ReadCanonicalWindow(canceled, query); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// As páginas compilam, mas falta o catálogo exigido pela leitura das fontes.
	// Depois de restaurado o schema, o mesmo leitor precisa tentar novamente.
	if err := f.db.Exec("ALTER TABLE tool_catalog RENAME TO unavailable_catalog").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadCanonicalWindow(ctx, query); err == nil {
		t.Fatal("preparação aceitou schema incompleto")
	}
	if err := f.db.Exec("ALTER TABLE unavailable_catalog RENAME TO tool_catalog").Error; err != nil {
		t.Fatal(err)
	}
	// Todas disputam a primeira preparação, usando um pool de uma conexão.
	opened := make(chan error, 8)
	for range cap(opened) {
		go func() {
			_, err := f.reader.ReadCanonicalWindow(ctx, query)
			opened <- err
		}()
	}
	for range cap(opened) {
		if err := <-opened; err != nil {
			t.Fatal(err)
		}
	}
	before, err := f.reader.ReadCanonicalWindow(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Items) != 2 {
		t.Fatalf("itens: %d", len(before.Items))
	}
	// Descarta a conexão que compilou os planos. O próximo uso deve preparar na
	// nova conexão e continuar a respeitar a autorização e a transação vigente.
	raw.SetMaxIdleConns(0)
	if _, err := f.reader.ReadCanonicalWindow(database.WithUserID(ctx, "other-user"), query); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadCanonicalWindow(ctx, query); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadCanonicalWindow(ctx, query); err == nil {
		t.Fatal("leitor antigo aceitou banco fechado")
	}
	reopened, err := gorm.Open(sqlite.Open(f.path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	reopenedSQL, err := reopened.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopenedSQL.Close() })
	if err := reopened.Model(&database.ChatMessage{}).Where("id=?", f.answer).Update("content", "resposta após reabertura").Error; err != nil {
		t.Fatal(err)
	}
	after, err := NewDBTimelineReader(reopened).ReadCanonicalWindow(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version == before.Version {
		t.Fatal("reabertura não observou a revisão gravada")
	}
	if len(after.Items) != len(before.Items) {
		t.Fatal("reabertura alterou identidades")
	}
	for i := range before.Items {
		if after.Items[i].ItemID != before.Items[i].ItemID {
			t.Fatal("reabertura alterou identidade")
		}
	}
}

// Outra abertura não pode herdar a espera de uma preparação que disputa o pool.
func TestTimelinePreparedOpeningCancelsWhileAnotherOpeningWaitsForConnection(t *testing.T) {
	f := newSnapshotFixture(t)
	raw, err := f.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	ctx, stop := context.WithTimeout(f.ctx, 5*time.Second)
	defer stop()
	held, err := raw.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	query := database.MessageWindowQuery{ConversationID: f.conversation, Anchor: "end", Direction: "before", Limit: 20}
	waits := raw.Stats().WaitCount
	first := make(chan error, 1)
	go func() { _, err := f.reader.ReadCanonicalWindow(ctx, query); first <- err }()
	// WaitCount prova que o primeiro preparo já detém a exclusão e espera o pool.
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for raw.Stats().WaitCount == waits {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("primeira abertura não aguardou o pool")
		}
	}
	canceled, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	second := make(chan error, 1)
	go func() { _, err := f.reader.ReadCanonicalWindow(canceled, query); second <- err }()
	select {
	case err := <-second:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cancelamento: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("abertura cancelada ficou presa à preparação de outra abertura")
	}
	select {
	case err := <-first:
		t.Fatalf("primeira abertura não deveria concluir com pool ocupado: %v", err)
	default:
	}
	if err := held.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if _, err := f.reader.ReadCanonicalWindow(ctx, query); err != nil {
		t.Fatal(err)
	}
}
