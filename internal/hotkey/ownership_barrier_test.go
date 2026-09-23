package hotkey

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOwnershipBarrierSnapshotIsSafeBootstrap(t *testing.T) {
	barrier := NewOwnershipBarrier(func(OwnershipFrame) {})
	snapshot := barrier.Snapshot()

	if snapshot.Version != 1 || snapshot.Revision != 0 || snapshot.Platform != "windows" {
		t.Fatalf("initial snapshot = %#v", snapshot)
	}
	if snapshot.InstanceID == "" {
		t.Fatal("initial snapshot has no instance ID")
	}
	parsed, err := uuid.Parse(snapshot.InstanceID)
	if err != nil || parsed.Version() != 7 {
		t.Fatalf("instance ID = %q, want UUIDv7: %v", snapshot.InstanceID, err)
	}
	if snapshot.Combinations == nil || len(snapshot.Combinations) != 0 {
		t.Fatalf("initial combinations = %#v, want non-nil empty slice", snapshot.Combinations)
	}

	wire, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(wire); got != `{"version":1,"instanceId":"`+snapshot.InstanceID+`","revision":0,"platform":"windows","combinations":[]}` {
		t.Fatalf("initial JSON = %s", got)
	}
	if barrier.Ack(snapshot.InstanceID, 0) {
		t.Fatal("ACK revision zero was accepted")
	}
}

func TestOwnershipBarrierPublishWaitsForDeterministicAck(t *testing.T) {
	published := make(chan OwnershipFrame, 1)
	barrier := NewOwnershipBarrier(func(frame OwnershipFrame) { published <- frame })
	combinations := []OwnershipCombination{{Key: 0x41, Modifiers: 0x06}}
	result := make(chan error, 1)
	go func() { result <- barrier.Publish(context.Background(), combinations) }()

	frame := receiveOwnershipFrame(t, published)
	if frame.Revision != 1 || frame.Version != 1 || frame.Platform != "windows" {
		t.Fatalf("published frame = %#v", frame)
	}
	if got := barrier.Snapshot(); got.Revision != 1 || len(got.Combinations) != 1 {
		t.Fatalf("pending snapshot = %#v", got)
	}
	select {
	case err := <-result:
		t.Fatalf("Publish returned before ACK: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	if !barrier.Ack(frame.InstanceID, frame.Revision) {
		t.Fatal("matching ACK was rejected")
	}
	if err := receiveOwnershipResult(t, result); err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
	if barrier.Ack(frame.InstanceID, frame.Revision) {
		t.Fatal("ACK replay was accepted")
	}
}

func TestOwnershipBarrierCopiesInputCallbackAndSnapshotSlices(t *testing.T) {
	published := make(chan OwnershipFrame, 1)
	barrier := NewOwnershipBarrier(func(frame OwnershipFrame) {
		frame.Combinations[0].Key = 0x99
		published <- frame
	})
	input := []OwnershipCombination{{Key: 0x41, Modifiers: 0x06}}
	result := make(chan error, 1)
	go func() { result <- barrier.Publish(context.Background(), input) }()

	frame := receiveOwnershipFrame(t, published)
	input[0].Key = 0x55
	snapshot := barrier.Snapshot()
	if snapshot.Combinations[0].Key != 0x41 {
		t.Fatalf("input/callback alias changed pending snapshot: %#v", snapshot)
	}
	if !barrier.Ack(frame.InstanceID, frame.Revision) {
		t.Fatal("matching ACK was rejected")
	}
	if err := receiveOwnershipResult(t, result); err != nil {
		t.Fatal(err)
	}

	snapshot = barrier.Snapshot()
	snapshot.Combinations[0].Key = 0x77
	if got := barrier.Snapshot().Combinations[0].Key; got != 0x41 {
		t.Fatalf("snapshot returned an aliased slice: %#x", got)
	}
}

func TestOwnershipBarrierSerializesTransitions(t *testing.T) {
	published := make(chan OwnershipFrame, 2)
	barrier := NewOwnershipBarrier(func(frame OwnershipFrame) { published <- frame })
	firstResult := make(chan error, 1)
	secondResult := make(chan error, 1)
	go func() {
		firstResult <- barrier.Publish(context.Background(), []OwnershipCombination{{Key: 1}})
	}()
	first := receiveOwnershipFrame(t, published)
	go func() {
		secondResult <- barrier.Publish(context.Background(), []OwnershipCombination{{Key: 2}})
	}()
	select {
	case frame := <-published:
		t.Fatalf("second transition published before first ACK: %#v", frame)
	case <-time.After(20 * time.Millisecond):
	}
	if !barrier.Ack(first.InstanceID, first.Revision) {
		t.Fatal("first ACK was rejected")
	}
	if err := receiveOwnershipResult(t, firstResult); err != nil {
		t.Fatal(err)
	}
	second := receiveOwnershipFrame(t, published)
	if second.Revision != first.Revision+1 {
		t.Fatalf("second revision = %d, want %d", second.Revision, first.Revision+1)
	}
	if !barrier.Ack(second.InstanceID, second.Revision) {
		t.Fatal("second ACK was rejected")
	}
	if err := receiveOwnershipResult(t, secondResult); err != nil {
		t.Fatal(err)
	}
}

func TestOwnershipBarrierRejectsReplayCrossInstanceAndFutureAck(t *testing.T) {
	published := make(chan OwnershipFrame, 2)
	barrier := NewOwnershipBarrier(func(frame OwnershipFrame) { published <- frame })
	result := make(chan error, 1)
	go func() { result <- barrier.Publish(context.Background(), nil) }()
	first := receiveOwnershipFrame(t, published)
	if barrier.Ack("foreign-instance", first.Revision) || barrier.Ack(first.InstanceID, 0) || barrier.Ack(first.InstanceID, first.Revision+1) {
		t.Fatal("stale, cross-instance, or future ACK was accepted")
	}
	if !barrier.Ack(first.InstanceID, first.Revision) {
		t.Fatal("matching ACK was rejected")
	}
	if err := receiveOwnershipResult(t, result); err != nil {
		t.Fatal(err)
	}

	go func() { result <- barrier.Publish(context.Background(), nil) }()
	second := receiveOwnershipFrame(t, published)
	if barrier.Ack(first.InstanceID, first.Revision) || barrier.Ack(second.InstanceID, second.Revision-1) || barrier.Ack("foreign-instance", second.Revision) {
		t.Fatal("replayed or stale ACK was accepted for a new frame")
	}
	if !barrier.Ack(second.InstanceID, second.Revision) {
		t.Fatal("second matching ACK was rejected")
	}
	if err := receiveOwnershipResult(t, result); err != nil {
		t.Fatal(err)
	}
}

func TestOwnershipBarrierCloseCancelsWaitAndRejectsFutureWork(t *testing.T) {
	published := make(chan OwnershipFrame, 1)
	barrier := NewOwnershipBarrier(func(frame OwnershipFrame) { published <- frame })
	result := make(chan error, 1)
	go func() { result <- barrier.Publish(context.Background(), nil) }()
	frame := receiveOwnershipFrame(t, published)
	barrier.Close()
	if err := receiveOwnershipResult(t, result); !errors.Is(err, ErrOwnershipBarrierClosed) {
		t.Fatalf("close result = %v, want ErrOwnershipBarrierClosed", err)
	}
	if barrier.Ack(frame.InstanceID, frame.Revision) {
		t.Fatal("ACK after Close was accepted")
	}
	if err := barrier.Publish(context.Background(), nil); !errors.Is(err, ErrOwnershipBarrierClosed) {
		t.Fatalf("publish after Close = %v", err)
	}
}

func TestOwnershipBarrierContextCancelMakesAckStale(t *testing.T) {
	published := make(chan OwnershipFrame, 2)
	barrier := NewOwnershipBarrier(func(frame OwnershipFrame) { published <- frame })
	ctx, cancel := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() { firstResult <- barrier.Publish(ctx, nil) }()
	first := receiveOwnershipFrame(t, published)
	cancel()
	if err := receiveOwnershipResult(t, firstResult); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled result = %v", err)
	}
	if barrier.Ack(first.InstanceID, first.Revision) {
		t.Fatal("stale ACK after cancellation was accepted")
	}

	secondResult := make(chan error, 1)
	go func() { secondResult <- barrier.Publish(context.Background(), nil) }()
	second := receiveOwnershipFrame(t, published)
	if second.Revision != first.Revision+1 {
		t.Fatalf("revision after cancellation = %d, want %d", second.Revision, first.Revision+1)
	}
	if !barrier.Ack(second.InstanceID, second.Revision) {
		t.Fatal("current ACK was rejected")
	}
	if err := receiveOwnershipResult(t, secondResult); err != nil {
		t.Fatal(err)
	}
}

func TestOwnershipBarrierTimeoutRestoresPreviousSafeSnapshotAndRemovalPublishesEmpty(t *testing.T) {
	published := make(chan OwnershipFrame, 3)
	barrier := NewOwnershipBarrier(func(frame OwnershipFrame) { published <- frame })

	initialResult := make(chan error, 1)
	go func() {
		initialResult <- barrier.Publish(context.Background(), []OwnershipCombination{{Key: 0x41}})
	}()
	initial := receiveOwnershipFrame(t, published)
	if !barrier.Ack(initial.InstanceID, initial.Revision) {
		t.Fatal("initial ACK was rejected")
	}
	if err := receiveOwnershipResult(t, initialResult); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	timedOutResult := make(chan error, 1)
	go func() {
		timedOutResult <- barrier.Publish(ctx, []OwnershipCombination{{Key: 0x42}})
	}()
	timedOut := receiveOwnershipFrame(t, published)
	if got := barrier.Snapshot().Combinations[0].Key; got != 0x42 {
		t.Fatalf("pending snapshot key = %#x, want proposed transition", got)
	}
	cancel()
	if err := receiveOwnershipResult(t, timedOutResult); !errors.Is(err, context.Canceled) {
		t.Fatalf("timed out publication = %v", err)
	}
	if got := barrier.Snapshot(); got.Revision != initial.Revision || got.Combinations[0].Key != 0x41 {
		t.Fatalf("snapshot after timeout = %#v, want previous safe frame", got)
	}
	if barrier.Ack(timedOut.InstanceID, timedOut.Revision) {
		t.Fatal("timed-out ACK was accepted")
	}

	removalResult := make(chan error, 1)
	go func() { removalResult <- barrier.Publish(context.Background(), nil) }()
	removal := receiveOwnershipFrame(t, published)
	if removal.Revision <= initial.Revision || len(removal.Combinations) != 0 {
		t.Fatalf("removal frame = %#v", removal)
	}
	if got := barrier.Snapshot(); got.Revision != removal.Revision || got.Combinations == nil || len(got.Combinations) != 0 {
		t.Fatalf("pending removal snapshot = %#v, want empty", got)
	}
	if !barrier.Ack(removal.InstanceID, removal.Revision) {
		t.Fatal("removal ACK was rejected")
	}
	if err := receiveOwnershipResult(t, removalResult); err != nil {
		t.Fatal(err)
	}
	if got := barrier.Snapshot(); got.Revision != removal.Revision || got.Combinations == nil || len(got.Combinations) != 0 {
		t.Fatalf("committed removal snapshot = %#v, want empty", got)
	}
}

func TestOwnershipBarrierConvertsPublishPanicToError(t *testing.T) {
	barrier := NewOwnershipBarrier(func(OwnershipFrame) { panic("boom") })
	if err := barrier.Publish(context.Background(), nil); err == nil {
		t.Fatal("publish panic did not become an error")
	}
	if got := barrier.Snapshot().Revision; got != 0 {
		t.Fatalf("panic changed committed revision to %d", got)
	}
}

func TestOwnershipBarrierRejectsAckAfterPublisherCancelsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var barrier *OwnershipBarrier
	ackResult := make(chan bool, 1)
	barrier = NewOwnershipBarrier(func(frame OwnershipFrame) {
		cancel()
		ackResult <- barrier.Ack(frame.InstanceID, frame.Revision)
	})

	if err := barrier.Publish(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Publish after callback cancellation = %v, want context.Canceled", err)
	}
	if accepted := <-ackResult; accepted {
		t.Fatal("ACK from a canceled publisher was accepted")
	}
	if got := barrier.Snapshot(); got.Revision != 0 || len(got.Combinations) != 0 {
		t.Fatalf("canceled publication changed snapshot = %#v", got)
	}
}

func receiveOwnershipFrame(t *testing.T, published <-chan OwnershipFrame) OwnershipFrame {
	t.Helper()
	select {
	case frame := <-published:
		return frame
	case <-time.After(time.Second):
		t.Fatal("ownership frame was not published")
		return OwnershipFrame{}
	}
}

func receiveOwnershipResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("ownership publication did not finish")
		return nil
	}
}
