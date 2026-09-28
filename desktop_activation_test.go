package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	application "assistente/internal/app"
	"assistente/internal/desktopinstance"
	"github.com/wailsapp/wails/v2/pkg/options"
)

type desktopTestCloser struct{ closed bool }

func (c *desktopTestCloser) Close() error { c.closed = true; return nil }

func isolateDesktopReservation(t *testing.T) *desktopTestCloser {
	t.Helper()
	previousResolve, previousAcquire := resolveDesktopDatabase, acquireDesktopInstance
	previousRetained := desktopProcessReservation
	t.Cleanup(func() {
		resolveDesktopDatabase, acquireDesktopInstance = previousResolve, previousAcquire
		desktopProcessReservation = previousRetained
	})
	path := filepath.Join(t.TempDir(), "conversations.db")
	resolveDesktopDatabase = func() (string, error) { return path, nil }
	closer := &desktopTestCloser{}
	acquireDesktopInstance = func(got string, _ func()) (io.Closer, error) {
		if got != path {
			t.Fatalf("reserved %q instead of %q", got, path)
		}
		return closer, nil
	}
	return closer
}

func TestRunRetainsReservationUntilProcessExitAfterStartupFailure(t *testing.T) {
	closer := isolateDesktopReservation(t)
	previousRun, previousStart, previousQuit, previousNative := runDesktop, startDesktop, quitDesktop, showNativeFatalError
	t.Cleanup(func() {
		runDesktop, startDesktop, quitDesktop, showNativeFatalError = previousRun, previousStart, previousQuit, previousNative
	})
	startDesktop = func(*application.App, context.Context) error { return errors.New("partial startup") }
	quitDesktop = func(context.Context) {}
	showNativeFatalError = func(string, string) {}
	runDesktop = func(o *options.App) error {
		o.OnStartup(context.Background())
		return nil
	}
	if code := run([]string{"assistente"}); code != 1 || closer.closed || desktopProcessReservation != closer {
		t.Fatalf("exit=%d closed=%v retained=%v", code, closer.closed, desktopProcessReservation == closer)
	}
}

func TestRunSecondInstanceDoesNotStartDesktop(t *testing.T) {
	isolateDesktopReservation(t)
	previous := runDesktop
	t.Cleanup(func() { runDesktop = previous })
	runDesktop = func(*options.App) error { t.Fatal("second instance started desktop"); return nil }
	acquireDesktopInstance = func(string, func()) (io.Closer, error) {
		return nil, desktopinstance.ErrAlreadyRunning
	}
	if code := run([]string{"assistente"}); code != 0 {
		t.Fatalf("exit=%d", code)
	}
}

func TestRunInstanceFailureDoesNotStartDesktop(t *testing.T) {
	isolateDesktopReservation(t)
	previousRun, previousNative := runDesktop, showNativeFatalError
	t.Cleanup(func() { runDesktop, showNativeFatalError = previousRun, previousNative })
	runDesktop = func(*options.App) error { t.Fatal("failed guard started desktop"); return nil }
	var message string
	showNativeFatalError = func(_, text string) { message = text }
	acquireDesktopInstance = func(string, func()) (io.Closer, error) {
		return nil, errors.New("notification timeout")
	}
	if code := run([]string{"assistente"}); code != 1 || !strings.Contains(message, "notification timeout") {
		t.Fatalf("exit=%d message=%q", code, message)
	}
}

func TestRunRetainsReservationAfterRunnerReturns(t *testing.T) {
	closer := isolateDesktopReservation(t)
	previous := runDesktop
	t.Cleanup(func() { runDesktop = previous })
	runDesktop = func(*options.App) error {
		if closer.closed {
			t.Fatal("reservation released before runner")
		}
		return nil
	}
	if code := run([]string{"assistente"}); code != 0 || closer.closed || desktopProcessReservation != closer {
		t.Fatalf("exit=%d closed=%v retained=%v", code, closer.closed, desktopProcessReservation == closer)
	}
}

func TestDesktopActivationQueuesBeforeReadyAndStops(t *testing.T) {
	requests := make(chan struct{}, 1)
	requests <- struct{}{}
	lifetime, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		serveDesktopActivations(lifetime, context.Background(), requests, func(context.Context) { called <- struct{}{} })
	}()
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("pending activation was lost")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("activation worker did not stop")
	}
}

func TestDesktopActivationDoesNotPresentAfterCancellation(t *testing.T) {
	lifetime, cancel := context.WithCancel(context.Background())
	cancel()
	requests := make(chan struct{}, 1)
	requests <- struct{}{}
	serveDesktopActivations(lifetime, context.Background(), requests, func(context.Context) { t.Fatal("closed window activated") })
}
