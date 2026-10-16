package mcp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOAuthHandshakeBudgetExcludesInteraction(t *testing.T) {
	session := withOAuthInteraction(context.Background())
	end := beginOAuthInteraction(session)
	ctx, cancel := oauthHandshakeContext(context.Background(), session, 40*time.Millisecond)
	defer cancel()
	select {
	case <-ctx.Done():
		t.Fatalf("interaction consumed handshake budget: %v", ctx.Err())
	case <-time.After(100 * time.Millisecond):
	}
	end()
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			t.Fatal(ctx.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("handshake did not expire after interaction")
	}
}
func TestOAuthHandshakeInteractionRetainsCancellation(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		parent, parentCancel := context.WithCancel(context.Background())
		session, sessionCancel := context.WithCancel(withOAuthInteraction(context.Background()))
		end := beginOAuthInteraction(session)
		ctx, cancel := oauthHandshakeContext(parent, session, time.Second)
		if disconnect {
			sessionCancel()
		} else {
			parentCancel()
		}
		select {
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatal(ctx.Err())
			}
		case <-time.After(time.Second):
			t.Fatal("cancellation blocked by interaction")
		}
		end()
		cancel()
		parentCancel()
		sessionCancel()
	}
}
