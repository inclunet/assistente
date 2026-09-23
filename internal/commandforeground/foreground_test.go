package commandforeground

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeReader struct {
	snapshot Snapshot
	err      error
	calls    int
	calledAt *[]string
}

func (f *fakeReader) Capture(context.Context) (Snapshot, error) {
	f.calls++
	if f.calledAt != nil {
		*f.calledAt = append(*f.calledAt, "capture")
	}
	return f.snapshot, f.err
}

func TestCaptureBeforeShowCapturesAndShowsInOrder(t *testing.T) {
	order := []string{}
	want := Snapshot{
		Identity:   Identity{window: 7, process: 11},
		Version:    "fixture.v1",
		CapturedAt: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
		Summary:    Summary{Executable: "code.exe", WindowClass: "fixture", ProviderVersion: "fixture.v1"},
	}
	reader := &fakeReader{snapshot: want, calledAt: &order}
	got, err := CaptureBeforeShow(context.Background(), reader, func() error {
		order = append(order, "show")
		return nil
	})
	if err != nil {
		t.Fatalf("CaptureBeforeShow: %v", err)
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(order, []string{"capture", "show"}) {
		t.Fatalf("got snapshot/order = %#v/%#v, want %#v/[capture show]", got, order, want)
	}
}

func TestCaptureBeforeShowCaptureFailureIsFailClosed(t *testing.T) {
	captureErr := errors.New("capture failed")
	reader := &fakeReader{err: captureErr}
	showed := false
	got, err := CaptureBeforeShow(context.Background(), reader, func() error {
		showed = true
		return nil
	})
	if !errors.Is(err, captureErr) || showed || !got.Identity.IsZero() {
		t.Fatalf("got snapshot=%#v err=%v showed=%v", got, err, showed)
	}
}

func TestCaptureBeforeShowPreservesSnapshotWhenShowFails(t *testing.T) {
	want := validSnapshot()
	showErr := errors.New("show failed")
	got, err := CaptureBeforeShow(context.Background(), &fakeReader{snapshot: want}, func() error {
		return showErr
	})
	if !errors.Is(err, showErr) || !reflect.DeepEqual(got, want) {
		t.Fatalf("got snapshot=%#v err=%v, want snapshot=%#v and show error", got, err, want)
	}
}

func TestForegroundFactVersionIsStableForSameIdentity(t *testing.T) {
	identity := Identity{window: 7, process: 11, creationTime: 13}
	summary := Summary{Executable: "code.exe", WindowClass: "fixture", ProviderVersion: ProviderVersion}
	first := Snapshot{Identity: identity, Version: foregroundFactVersion(identity, summary), CapturedAt: time.Unix(1, 0), Summary: summary}
	second := Snapshot{Identity: identity, Version: foregroundFactVersion(identity, summary), CapturedAt: time.Unix(2, 0), Summary: summary}
	if first.Version != second.Version || first.Version == ProviderVersion {
		t.Fatalf("versão não estável/independente do provider: first=%q second=%q", first.Version, second.Version)
	}

	otherIdentity := Identity{window: 7, process: 12, creationTime: 13}
	if got := foregroundFactVersion(otherIdentity, summary); got == first.Version {
		t.Fatalf("identidades diferentes produziram a mesma versão: %q", got)
	}
}

func TestCaptureBeforeShowCancellationIsFailClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	reader := &fakeReader{snapshot: validSnapshot()}
	got, err := CaptureBeforeShow(ctx, reader, func() error {
		calls++
		cancel()
		return nil
	})
	if err != nil || calls != 1 || got.Version != "v1" {
		t.Fatalf("initial capture/show = snapshot=%#v err=%v calls=%d", got, err, calls)
	}

	cancelled, err := CaptureBeforeShow(ctx, reader, func() error {
		t.Fatal("show chamado com contexto cancelado")
		return nil
	})
	if !errors.Is(err, context.Canceled) || !cancelled.Identity.IsZero() {
		t.Fatalf("cancelado = snapshot=%#v err=%v", cancelled, err)
	}
}

func TestCaptureBeforeShowRejectsInvalidSnapshotAndTypedNilReader(t *testing.T) {
	showed := false
	_, err := CaptureBeforeShow(context.Background(), &fakeReader{snapshot: Snapshot{Version: "v1"}}, func() error {
		showed = true
		return nil
	})
	if !errors.Is(err, ErrInvalidSnapshot) || showed {
		t.Fatalf("snapshot inválido = err=%v showed=%v", err, showed)
	}

	var typedNil *fakeReader
	_, err = CaptureBeforeShow(context.Background(), typedNil, func() error {
		t.Fatal("show chamado com reader typed-nil")
		return nil
	})
	if err == nil {
		t.Fatal("reader typed-nil deveria falhar")
	}
}

func TestNormalizeExecutableBaseRedactsPath(t *testing.T) {
	tests := map[string]string{
		` C:\Program Files\Code\Code.EXE `: "code.exe",
		`/opt/apps/Assistente`:             "assistente",
		`\\server\share\App.Exe` + "\x00":  "app.exe",
		`   `:                              "",
	}
	for input, want := range tests {
		if got := normalizeExecutableBase(input); got != want {
			t.Errorf("normalizeExecutableBase(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSummaryHasOnlyAllowlistedFields(t *testing.T) {
	type summaryFields struct {
		Executable      string
		WindowClass     string
		ProviderVersion string
	}
	if reflect.TypeOf(Summary{}).NumField() != reflect.TypeOf(summaryFields{}).NumField() {
		t.Fatalf("Summary ganhou campos fora da allowlist: %#v", reflect.TypeOf(Summary{}))
	}
	if got := reflect.TypeOf(Summary{}).Field(1).Tag.Get("json"); got != "window_class" {
		t.Fatalf("tag da classe = %q, want window_class", got)
	}
	if got := reflect.TypeOf(Summary{}).Field(2).Tag.Get("json"); got != "provider_version" {
		t.Fatalf("tag da versão do provider = %q, want provider_version", got)
	}
	if got := reflect.TypeOf(Snapshot{}).Field(2).Tag.Get("json"); got != "captured_at" {
		t.Fatalf("tag de CapturedAt = %q, want captured_at", got)
	}
	if got := normalizeWindowClass("  Chrome_WidgetWin_1\x00 "); got != "Chrome_WidgetWin_1" {
		t.Fatalf("classe normalizada = %q", got)
	}
	for _, input := range []string{"Chrome\nWidget", string([]byte{0xff}), strings.Repeat("a", maxWindowClassRunes+1)} {
		if got := normalizeWindowClass(input); got != "" {
			t.Errorf("normalizeWindowClass(%q) = %q, want empty", input, got)
		}
	}
}

func TestValidateSnapshotRejectsFutureAndExpiredPhysicalOrigins(t *testing.T) {
	base := Snapshot{Identity: Identity{window: 1, process: 2}, Version: "v1", CapturedAt: time.Now().UTC(), Summary: Summary{Executable: "code.exe", WindowClass: "fixture", ProviderVersion: ProviderVersion}}
	if err := ValidateSnapshot(base, time.Minute); err != nil {
		t.Fatalf("snapshot válido rejeitado: %v", err)
	}
	future := base
	future.CapturedAt = time.Now().UTC().Add(time.Second)
	if !errors.Is(ValidateSnapshot(future, time.Minute), ErrInvalidSnapshot) {
		t.Fatal("snapshot futuro aceito")
	}
	expired := base
	expired.CapturedAt = time.Now().UTC().Add(-2 * time.Minute)
	if !errors.Is(ValidateSnapshot(expired, time.Minute), ErrInvalidSnapshot) {
		t.Fatal("snapshot expirado aceito")
	}
}

func validSnapshot() Snapshot {
	return Snapshot{
		Identity:   Identity{window: 1, process: 2},
		Version:    "v1",
		CapturedAt: time.Unix(1, 0),
	}
}
