package commandui

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/commandbridge"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"

	"github.com/google/uuid"
)

func externalConnectionFixture() (*ExternalUIConnections, commandbridge.Owner, ExternalUIPrincipal, ExternalUIDestination) {
	store := NewExternalUIConnections()
	store.now = func() time.Time { return time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC) }
	owner := commandbridge.Owner{UserID: "user-01", SessionID: "session-01", WorkspaceID: "workspace-01"}
	principal := ExternalUIPrincipal{Issuer: "https://idp.example", Subject: "subject-1", UserID: owner.UserID, AuthContextID: "[\"issuer\",\"subject\",\"token-fingerprint\"]"}
	destination := ExternalUIDestination{WorkspaceID: owner.WorkspaceID, TabID: "tab-1", Surface: ExternalUISurfaceContext{
		SurfaceType: "editor", SurfaceID: "surface-1", SnapshotVersion: "v1", Selection: json.RawMessage(`{"text":"selected"}`),
	}}
	return store, owner, principal, destination
}

func TestExternalUIConnectionsClaimIsSingleUseAndOwnerMatched(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	invite, err := store.Begin(owner, target)
	if err != nil {
		t.Fatal(err)
	}
	wrong := principal
	wrong.UserID = "other-user"
	if _, err := store.Claim(invite.Invitation, wrong); !errors.Is(err, ErrExternalConnectionDenied) {
		t.Fatalf("owner mismatch claim = %v", err)
	}
	status, err := store.Claim(invite.Invitation, principal)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "connected" || status.ConnectionID == "" || status.Generation == "" || status.TargetSnapshotID == "" || status.ContextVersion == "" {
		t.Fatalf("claim returned incomplete status: %+v", status)
	}
	if _, err := store.Claim(invite.Invitation, principal); !errors.Is(err, ErrExternalConnectionDenied) {
		t.Fatalf("invite replay = %v", err)
	}
	if _, err := store.ReadForPrincipal(principal, status.ConnectionID, status.Generation); err != nil {
		t.Fatalf("explicit connection read = %v", err)
	}
	if _, err := store.ReadForPrincipal(principal, "", status.Generation); !errors.Is(err, ErrExternalConnectionInvalid) {
		t.Fatalf("implicit/empty connection read = %v", err)
	}
}

func TestExternalUIConnectionsPublishUsesCASAndHeartbeatKeepsStamps(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	invite, _ := store.Begin(owner, target)
	status, err := store.Claim(invite.Invitation, principal)
	if err != nil {
		t.Fatal(err)
	}
	oldSnapshot, oldContext, generation := status.TargetSnapshotID, status.ContextVersion, status.Generation
	lease := ExternalUILease{Owner: owner, ConnectionID: status.ConnectionID, Generation: generation}
	beat, err := store.Heartbeat(lease)
	if err != nil || beat.TargetSnapshotID != oldSnapshot || beat.ContextVersion != oldContext || beat.Generation != generation {
		t.Fatalf("heartbeat changed binding/context: status=%+v err=%v", beat, err)
	}
	publication := ExternalUIContextPublication{Owner: owner, ConnectionID: status.ConnectionID, Generation: generation,
		ExpectedTargetSnapshot: oldSnapshot, ExpectedContextVersion: oldContext, Target: target}
	publication.Target.Surface.SnapshotVersion = "v2"
	updated, err := store.PublishContext(publication)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Generation != generation || updated.TargetSnapshotID == oldSnapshot || updated.ContextVersion == oldContext {
		t.Fatalf("publication did not rotate context only: before=%+v after=%+v", status, updated)
	}
	if err := store.Validate(status.ConnectionID, generation, principal, oldSnapshot, oldContext); !errors.Is(err, ErrExternalConnectionStale) {
		t.Fatalf("old invocation context validate = %v", err)
	}
	if err := store.Validate(status.ConnectionID, generation, principal, updated.TargetSnapshotID, updated.ContextVersion); err != nil {
		t.Fatalf("new invocation context validate = %v", err)
	}
	publication.ExpectedContextVersion = oldContext
	if _, err := store.PublishContext(publication); !errors.Is(err, ErrExternalConnectionStale) {
		t.Fatalf("stale CAS publication = %v", err)
	}
}

func TestExternalUIConnectionsDisconnectAndIdentityRevocationInvalidate(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	invite, _ := store.Begin(owner, target)
	status, err := store.Claim(invite.Invitation, principal)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Validate(status.ConnectionID, status.Generation, principal, status.TargetSnapshotID, status.ContextVersion); err != nil {
		t.Fatal(err)
	}
	store.RevokePrincipal(principal.Issuer, principal.Subject)
	if err := store.Validate(status.ConnectionID, status.Generation, principal, status.TargetSnapshotID, status.ContextVersion); !errors.Is(err, ErrExternalConnectionDenied) {
		t.Fatalf("revoked principal still had connection: %v", err)
	}
	invite, _ = store.Begin(owner, target)
	status, err = store.Claim(invite.Invitation, principal)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Disconnect(ExternalUILease{Owner: owner, ConnectionID: status.ConnectionID, Generation: status.Generation}); err != nil {
		t.Fatal(err)
	}
	if err := store.Validate(status.ConnectionID, status.Generation, principal, status.TargetSnapshotID, status.ContextVersion); !errors.Is(err, ErrExternalConnectionDenied) {
		t.Fatalf("disconnected session still had connection: %v", err)
	}
}

func TestExternalUIConnectionsInvitationAndConnectionExpire(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	invite, _ := store.Begin(owner, target)
	store.now = func() time.Time { return invite.ExpiresAt.Add(time.Nanosecond) }
	if _, err := store.Claim(invite.Invitation, principal); !errors.Is(err, ErrExternalConnectionDenied) {
		t.Fatalf("expired invite claim = %v", err)
	}
	store.now = func() time.Time { return time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC) }
	invite, _ = store.Begin(owner, target)
	status, err := store.Claim(invite.Invitation, principal)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return status.ExpiresAt.Add(time.Nanosecond) }
	if err := store.Validate(status.ConnectionID, status.Generation, principal, status.TargetSnapshotID, status.ContextVersion); !errors.Is(err, ErrExternalConnectionDenied) {
		t.Fatalf("expired connection validate = %v", err)
	}
}

func TestExternalUIConnectionsDetachSurfacePayload(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	invite, err := store.Begin(owner, target)
	if err != nil {
		t.Fatal(err)
	}
	target.Surface.Selection[8] = 'X'
	status, err := store.Claim(invite.Invitation, principal)
	if err != nil {
		t.Fatal(err)
	}
	if string(status.Target.Surface.Selection) != `{"text":"selected"}` {
		t.Fatalf("surface payload aliased caller memory: %s", status.Target.Surface.Selection)
	}
}

func TestExternalUIConnectionsOwnerReadLifecycleAndBoundedHistory(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	status, err := store.ReadForOwner(owner)
	if err != nil || status.State != "disconnected" {
		t.Fatalf("empty owner read = %+v, %v", status, err)
	}
	invite, err := store.Begin(owner, target)
	if err != nil {
		t.Fatal(err)
	}
	status, err = store.ReadForOwner(owner)
	if err != nil || status.State != "waiting_claim" {
		t.Fatalf("pending owner read = %+v, %v", status, err)
	}
	status, err = store.Claim(invite.Invitation, principal)
	if err != nil {
		t.Fatal(err)
	}
	status, err = store.ReadForOwner(owner)
	if err != nil || status.State != "connected" || status.ConnectionID == "" {
		t.Fatalf("connected owner read = %+v, %v", status, err)
	}
	if err := store.Disconnect(ExternalUILease{Owner: owner, ConnectionID: status.ConnectionID, Generation: status.Generation}); err != nil {
		t.Fatal(err)
	}
	status, err = store.ReadForOwner(owner)
	if err != nil || status.State != "disconnected" {
		t.Fatalf("disconnected owner read = %+v, %v", status, err)
	}

	for i := 0; i < externalOwnerStateMax+8; i++ {
		other := owner
		other.SessionID = "session-" + uuid.NewString()
		otherTarget := target
		otherTarget.Surface = cloneExternalDestination(target).Surface
		otherInvite, beginErr := store.Begin(other, otherTarget)
		if beginErr != nil {
			t.Fatal(beginErr)
		}
		otherPrincipal := principal
		otherPrincipal.UserID = other.UserID
		otherStatus, claimErr := store.Claim(otherInvite.Invitation, otherPrincipal)
		if claimErr != nil {
			t.Fatal(claimErr)
		}
		if disconnectErr := store.Disconnect(ExternalUILease{Owner: other, ConnectionID: otherStatus.ConnectionID, Generation: otherStatus.Generation}); disconnectErr != nil {
			t.Fatal(disconnectErr)
		}
	}
	store.mu.Lock()
	states := len(store.ownerStates)
	store.mu.Unlock()
	if states > externalOwnerStateMax {
		t.Fatalf("ownerStates grew beyond bound: %d > %d", states, externalOwnerStateMax)
	}
}

func TestExternalUIConnectionsClearCancelsAndKeepsGenerationMonotonic(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	invite, err := store.Begin(owner, target)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Claim(invite.Invitation, principal)
	if err != nil {
		t.Fatal(err)
	}
	store.Clear()
	if _, err := store.ReadForPrincipal(principal, first.ConnectionID, first.Generation); !errors.Is(err, ErrExternalConnectionDenied) {
		t.Fatalf("connection survived Clear: %v", err)
	}
	status, err := store.ReadForOwner(owner)
	if err != nil || status.State != "disconnected" {
		t.Fatalf("owner after Clear = %+v, %v", status, err)
	}
	invite, err = store.Begin(owner, target)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Claim(invite.Invitation, principal)
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation == second.Generation {
		t.Fatalf("generation ABA after Clear: %q", first.Generation)
	}
}

func TestExternalUIConnectionsHandoffAllowsAcknowledgedCausalNavigation(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	status := externalConnect(t, store, owner, principal, target)
	handle, event := externalStart(t, store, owner, principal, status)
	if event.CommandID != "ui.navigate" || event.InvocationID == "" {
		t.Fatalf("ready event lacks command correlation: %+v", event)
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(eventJSON), "arguments") || strings.Contains(string(eventJSON), "receiptId") {
		t.Fatalf("ready event leaked handoff data: %s", eventJSON)
	}
	handoff, err := store.TakeExternal(owner, status.ConnectionID, status.Generation, event.InvocationID, status.TargetSnapshotID, status.ContextVersion)
	if err != nil {
		t.Fatal(err)
	}
	if handoff.CommandID != event.CommandID || string(handoff.Arguments) != `{"route":"settings"}` || handoff.TargetSnapshotID != status.TargetSnapshotID {
		t.Fatalf("unexpected handoff: %+v", handoff)
	}
	if err := store.CompleteExternal(owner, status.ConnectionID, status.Generation, handoff.InvocationID, handoff.ReceiptID, handoff.TargetSnapshotID, handoff.ContextVersion, "succeeded"); err != nil {
		t.Fatal(err)
	}
	if outcome := <-handle.Done; outcome.Status != commandledger.Succeeded || string(outcome.Result) != `{}` {
		t.Fatalf("ack outcome=%q result=%s", outcome.Status, outcome.Result)
	}
	newTarget := target
	newTarget.TabID = "tab-opened-by-command"
	newTarget.Surface.SurfaceID = "surface-settings"
	newTarget.Surface.SnapshotVersion = "v2"
	updated, err := store.PublishContext(ExternalUIContextPublication{Owner: owner, ConnectionID: status.ConnectionID, Generation: status.Generation, ExpectedTargetSnapshot: status.TargetSnapshotID, ExpectedContextVersion: status.ContextVersion, Target: newTarget})
	if err != nil {
		t.Fatalf("publish navigation after exact success ack: %v", err)
	}
	if updated.Target.TabID != newTarget.TabID {
		t.Fatalf("navigation was not published: %+v", updated.Target)
	}
}

func TestExternalUIConnectionsPublicUnknownMapsToLedgerOutcomeUnknown(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	status := externalConnect(t, store, owner, principal, target)
	handle, event := externalStart(t, store, owner, principal, status)
	handoff, err := store.TakeExternal(owner, status.ConnectionID, status.Generation, event.InvocationID, status.TargetSnapshotID, status.ContextVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteExternal(owner, status.ConnectionID, status.Generation, handoff.InvocationID, handoff.ReceiptID, handoff.TargetSnapshotID, handoff.ContextVersion, "unknown"); err != nil {
		t.Fatalf("public unknown outcome rejected: %v", err)
	}
	if outcome := <-handle.Done; outcome.Status != commandledger.OutcomeUnknown {
		t.Fatalf("public unknown mapped to %q", outcome.Status)
	}
}

func TestExternalUIConnectionsConcurrentContextChangeMakesTakenHandoffUnknown(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	status := externalConnect(t, store, owner, principal, target)
	handle, event := externalStart(t, store, owner, principal, status)
	handoff, err := store.TakeExternal(owner, status.ConnectionID, status.Generation, event.InvocationID, status.TargetSnapshotID, status.ContextVersion)
	if err != nil {
		t.Fatal(err)
	}
	newTarget := target
	newTarget.TabID = "tab-changed-concurrently"
	newTarget.Surface.SnapshotVersion = "v2"
	if _, err := store.PublishContext(ExternalUIContextPublication{Owner: owner, ConnectionID: status.ConnectionID, Generation: status.Generation, ExpectedTargetSnapshot: status.TargetSnapshotID, ExpectedContextVersion: status.ContextVersion, Target: newTarget}); err != nil {
		t.Fatal(err)
	}
	if outcome := <-handle.Done; outcome.Status != commandledger.OutcomeUnknown {
		t.Fatalf("taken handoff after concurrent navigation=%q, want outcome_unknown", outcome.Status)
	}
	if err := store.CompleteExternal(owner, status.ConnectionID, status.Generation, handoff.InvocationID, handoff.ReceiptID, handoff.TargetSnapshotID, handoff.ContextVersion, "succeeded"); !errors.Is(err, ErrExternalConnectionStale) {
		t.Fatalf("late completion after invalidation=%v", err)
	}
}

func TestExternalUIConnectionsCompletePrunesExpiredReceipt(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	status := externalConnect(t, store, owner, principal, target)
	handle, event := externalStart(t, store, owner, principal, status)
	handoff, err := store.TakeExternal(owner, status.ConnectionID, status.Generation, event.InvocationID, status.TargetSnapshotID, status.ContextVersion)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return status.ExpiresAt.Add(time.Second) }
	err = store.CompleteExternal(owner, status.ConnectionID, status.Generation, handoff.InvocationID, handoff.ReceiptID, handoff.TargetSnapshotID, handoff.ContextVersion, "succeeded")
	if !errors.Is(err, ErrExternalConnectionStale) {
		t.Fatalf("expired receipt completion=%v, want stale", err)
	}
	if outcome := <-handle.Done; outcome.Status != commandledger.OutcomeUnknown {
		t.Fatalf("expired taken execution=%q", outcome.Status)
	}
}

func TestExternalUIConnectionsPendingTTLExpiresWithoutRegistryTraffic(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	status := externalConnect(t, store, owner, principal, target)
	handle, event := externalStart(t, store, owner, principal, status)
	store.mu.Lock()
	store.pending[event.InvocationID].timer.Reset(10 * time.Millisecond)
	store.mu.Unlock()
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.Cancelled {
			t.Fatalf("unclaimed pending TTL outcome=%q", outcome.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("pending TTL did not cancel without a follow-up registry operation")
	}
}

func TestExternalUIConnectionsTakenPendingTTLReportsUnknown(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	status := externalConnect(t, store, owner, principal, target)
	handle, event := externalStart(t, store, owner, principal, status)
	if _, err := store.TakeExternal(owner, status.ConnectionID, status.Generation, event.InvocationID, status.TargetSnapshotID, status.ContextVersion); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.pending[event.InvocationID].timer.Reset(10 * time.Millisecond)
	store.mu.Unlock()
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.OutcomeUnknown {
			t.Fatalf("taken pending TTL outcome=%q, want outcome_unknown", outcome.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("taken pending TTL did not finish without another registry operation")
	}
}

func TestExternalUIConnectionsClearDoesNotWaitForBlockedNotifier(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	status := externalConnect(t, store, owner, principal, target)
	entered, release := make(chan struct{}), make(chan struct{})
	store.SetReadyNotifier(func(ExternalUIReadyEvent) {
		close(entered)
		<-release
	})
	t.Cleanup(store.Close)
	invocation, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	handle, err := store.StartExternal(owner, principal, status.ConnectionID, status.Generation, status.TargetSnapshotID, status.ContextVersion, invocation.String(), "ui.navigate", json.RawMessage(`{"route":"settings"}`))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("notifier did not start")
	}
	cleared := make(chan struct{})
	go func() { store.Clear(); close(cleared) }()
	select {
	case <-cleared:
	case <-time.After(250 * time.Millisecond):
		close(release)
		t.Fatal("Clear waited for an external notifier callback")
	}
	if outcome := <-handle.Done; outcome.Status != commandledger.Cancelled {
		t.Fatalf("Clear pending outcome=%q", outcome.Status)
	}
	close(release)
}

func TestExternalUIConnectionsCloseIsTerminalAndWorkerExits(t *testing.T) {
	store, owner, principal, target := externalConnectionFixture()
	invite, err := store.Begin(owner, target)
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Claim(invite.Invitation, principal)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	store.SetReadyNotifier(func(ExternalUIReadyEvent) {
		close(entered)
		<-release
	})
	invocation, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	handle, err := store.StartExternal(owner, principal, status.ConnectionID, status.Generation, status.TargetSnapshotID, status.ContextVersion, invocation.String(), "ui.navigate", json.RawMessage(`{"route":"settings"}`))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("notifier did not start")
	}
	closed := make(chan struct{})
	go func() { store.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(250 * time.Millisecond):
		close(release)
		t.Fatal("Close waited for an in-flight notifier")
	}
	if outcome := <-handle.Done; outcome.Status != commandledger.Cancelled {
		t.Fatalf("Close pending outcome=%q", outcome.Status)
	}
	if _, err := store.Begin(owner, target); !errors.Is(err, ErrExternalConnectionClosed) {
		t.Fatalf("Begin after Close = %v", err)
	}
	if _, err := store.Claim(invite.Invitation, principal); !errors.Is(err, ErrExternalConnectionClosed) {
		t.Fatalf("Claim after Close = %v", err)
	}
	if _, err := store.StartExternal(owner, principal, status.ConnectionID, status.Generation, status.TargetSnapshotID, status.ContextVersion, invocation.String(), "ui.navigate", json.RawMessage(`{"route":"settings"}`)); !errors.Is(err, ErrExternalConnectionClosed) {
		t.Fatalf("Start after Close = %v", err)
	}
	close(release)
	select {
	case <-store.readyDone:
	case <-time.After(time.Second):
		t.Fatal("worker did not exit after notifier returned and queue closed")
	}
}

func TestExternalUIConnectionsRevokeOwnerAndPrincipalCancelPending(t *testing.T) {
	t.Run("owner antes do take", func(t *testing.T) {
		store, owner, principal, target := externalConnectionFixture()
		status := externalConnect(t, store, owner, principal, target)
		handle, _ := externalStart(t, store, owner, principal, status)
		store.RevokeOwner(owner)
		if outcome := <-handle.Done; outcome.Status != commandledger.Cancelled {
			t.Fatalf("unclaimed owner revoke outcome=%q", outcome.Status)
		}
		read, err := store.ReadForOwner(owner)
		if err != nil || read.State != "disconnected" {
			t.Fatalf("owner state after revoke=%+v err=%v", read, err)
		}
	})
	t.Run("principal após take", func(t *testing.T) {
		store, owner, principal, target := externalConnectionFixture()
		status := externalConnect(t, store, owner, principal, target)
		handle, event := externalStart(t, store, owner, principal, status)
		if _, err := store.TakeExternal(owner, status.ConnectionID, status.Generation, event.InvocationID, status.TargetSnapshotID, status.ContextVersion); err != nil {
			t.Fatal(err)
		}
		store.RevokePrincipal(principal.Issuer, principal.Subject)
		if outcome := <-handle.Done; outcome.Status != commandledger.OutcomeUnknown {
			t.Fatalf("taken principal revoke outcome=%q", outcome.Status)
		}
	})
}

func externalConnect(t *testing.T, store *ExternalUIConnections, owner commandbridge.Owner, principal ExternalUIPrincipal, target ExternalUIDestination) ExternalUIConnectionStatus {
	t.Helper()
	invite, err := store.Begin(owner, target)
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Claim(invite.Invitation, principal)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func externalStart(t *testing.T, store *ExternalUIConnections, owner commandbridge.Owner, principal ExternalUIPrincipal, status ExternalUIConnectionStatus) (commandexecution.ExecutionHandle, ExternalUIReadyEvent) {
	t.Helper()
	t.Cleanup(store.Close)
	events := make(chan ExternalUIReadyEvent, 1)
	store.SetReadyNotifier(func(ready ExternalUIReadyEvent) {
		select {
		case events <- ready:
		default:
		}
	})
	invocation, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	handle, err := store.StartExternal(owner, principal, status.ConnectionID, status.Generation, status.TargetSnapshotID, status.ContextVersion, invocation.String(), "ui.navigate", json.RawMessage(`{"route":"settings"}`))
	if err != nil {
		t.Fatal(err)
	}
	var event ExternalUIReadyEvent
	select {
	case event = <-events:
	case <-time.After(time.Second):
		t.Fatal("ready event was not delivered by bounded worker")
	}
	if event.InvocationID != invocation.String() {
		t.Fatalf("ready event not enqueued: %+v", event)
	}
	return handle, event
}
