package hotkey

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

const (
	ownershipFrameVersion = 1
	ownershipPlatform     = "windows"
	// JavaScript Number.MAX_SAFE_INTEGER. Revisions cross the Wails/JSON
	// boundary and must remain exactly representable there.
	maxOwnershipRevision = uint64(9007199254740991)
)

var (
	// ErrOwnershipBarrierClosed is returned after the barrier has been closed.
	ErrOwnershipBarrierClosed = errors.New("ownership barrier is closed")
	// ErrOwnershipRevisionExhausted means that the JSON-safe revision space is
	// exhausted and no further transition can be proposed.
	ErrOwnershipRevisionExhausted = errors.New("ownership barrier revision space exhausted")
)

// OwnershipCombination is a native key/modifier pair exposed to the UI as
// ownership metadata. It does not describe a command and cannot authorize one.
type OwnershipCombination struct {
	Key       uint32 `json:"key"`
	Modifiers uint32 `json:"modifiers"`
}

// OwnershipFrame is the versioned ownership publication sent to the UI.
type OwnershipFrame struct {
	Version      int                    `json:"version"`
	InstanceID   string                 `json:"instanceId"`
	Revision     uint64                 `json:"revision"`
	Platform     string                 `json:"platform"`
	Combinations []OwnershipCombination `json:"combinations"`
}

type ownershipPublication struct {
	frame OwnershipFrame
	ctx   context.Context
	ack   chan struct{}
	acked bool
}

// OwnershipBarrier coordinates publication of native ownership with the UI.
// It deliberately contains no native registration logic and does not make
// ACKs usable as command authorization.
type OwnershipBarrier struct {
	publish func(OwnershipFrame)

	// transitionMu serializes the complete propose/publish/ack transition. The
	// state mutex remains available to Ack, Snapshot and Close while a callback
	// or a caller's context is waiting.
	transitionMu sync.Mutex
	stateMu      sync.Mutex
	closed       bool
	closeCh      chan struct{}
	instanceID   string
	nextRevision uint64
	current      OwnershipFrame
	pending      *ownershipPublication
}

// NewOwnershipBarrier creates a barrier with one UUIDv7 identity for its
// lifetime. publish is invoked outside the barrier's state mutex.
func NewOwnershipBarrier(publish func(OwnershipFrame)) *OwnershipBarrier {
	instanceID, err := uuid.NewV7()
	if err != nil {
		// google/uuid currently obtains entropy from crypto/rand. There is no
		// error return in this constructor's contract, so an inability to create
		// the required identity is a construction failure rather than a usable
		// barrier with an invalid instance ID.
		panic(fmt.Sprintf("create ownership barrier UUIDv7: %v", err))
	}

	empty := []OwnershipCombination{}
	return &OwnershipBarrier{
		publish:    publish,
		closeCh:    make(chan struct{}),
		instanceID: instanceID.String(),
		current: OwnershipFrame{
			Version:      ownershipFrameVersion,
			InstanceID:   instanceID.String(),
			Platform:     ownershipPlatform,
			Combinations: empty,
		},
	}
}

// Publish proposes one ownership transition and waits for its matching ACK.
// The caller owns the timeout policy through ctx.
func (b *OwnershipBarrier) Publish(ctx context.Context, combinations []OwnershipCombination) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if b == nil {
		return ErrOwnershipBarrierClosed
	}

	b.transitionMu.Lock()
	defer b.transitionMu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	if b.publish == nil {
		return errors.New("ownership barrier publish callback is nil")
	}

	b.stateMu.Lock()
	if b.closed {
		b.stateMu.Unlock()
		return ErrOwnershipBarrierClosed
	}
	if b.nextRevision >= maxOwnershipRevision {
		b.stateMu.Unlock()
		return ErrOwnershipRevisionExhausted
	}
	b.nextRevision++
	frame := OwnershipFrame{
		Version:      ownershipFrameVersion,
		InstanceID:   b.instanceID,
		Revision:     b.nextRevision,
		Platform:     ownershipPlatform,
		Combinations: cloneOwnershipCombinations(combinations),
	}
	p := &ownershipPublication{
		frame: cloneOwnershipFrame(frame),
		ctx:   ctx,
		ack:   make(chan struct{}),
	}
	b.pending = p
	callbackFrame := cloneOwnershipFrame(frame)
	b.stateMu.Unlock()

	if err := invokeOwnershipPublisher(b.publish, callbackFrame); err != nil {
		b.clearPending(p)
		return err
	}
	if err := ctx.Err(); err != nil {
		b.clearPending(p)
		return err
	}

	select {
	case <-p.ack:
		if b.commitPending(p) {
			return nil
		}
		if err := ctx.Err(); err != nil {
			b.clearPending(p)
			return err
		}
		return b.pendingResult(p)
	case <-ctx.Done():
		b.clearPending(p)
		return ctx.Err()
	case <-b.closeCh:
		if b.commitPending(p) {
			return nil
		}
		b.clearPending(p)
		return ErrOwnershipBarrierClosed
	}
}

// Ack acknowledges exactly the currently pending frame. It rejects stale,
// unknown, cross-instance and future acknowledgements, including replays.
func (b *OwnershipBarrier) Ack(instance string, revision uint64) bool {
	if b == nil {
		return false
	}
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.closed || b.pending == nil || b.pending.acked || b.pending.ctx.Err() != nil {
		return false
	}
	if instance != b.instanceID || revision != b.pending.frame.Revision {
		return false
	}
	b.pending.acked = true
	close(b.pending.ack)
	return true
}

// Snapshot returns the pending frame while a publication is awaiting ACK, and
// otherwise the last acknowledged frame. Every returned slice is independent.
func (b *OwnershipBarrier) Snapshot() OwnershipFrame {
	if b == nil {
		return OwnershipFrame{Version: ownershipFrameVersion, Platform: ownershipPlatform, Combinations: []OwnershipCombination{}}
	}
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.pending != nil {
		return cloneOwnershipFrame(b.pending.frame)
	}
	return cloneOwnershipFrame(b.current)
}

// Close rejects future publications and cancels the current wait, if any.
func (b *OwnershipBarrier) Close() {
	if b == nil {
		return
	}
	b.stateMu.Lock()
	if !b.closed {
		b.closed = true
		b.pending = nil
		close(b.closeCh)
	}
	b.stateMu.Unlock()
}

func (b *OwnershipBarrier) commitPending(p *ownershipPublication) bool {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.closed || b.pending != p || !p.acked || p.ctx.Err() != nil {
		return false
	}
	b.current = cloneOwnershipFrame(p.frame)
	b.pending = nil
	return true
}

func (b *OwnershipBarrier) clearPending(p *ownershipPublication) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.pending != p {
		return
	}
	b.pending = nil
}

func (b *OwnershipBarrier) pendingResult(p *ownershipPublication) error {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	if b.closed {
		return ErrOwnershipBarrierClosed
	}
	if p.acked {
		return nil
	}
	return errors.New("ownership barrier publication was superseded")
}

func invokeOwnershipPublisher(publish func(OwnershipFrame), frame OwnershipFrame) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("ownership barrier publish panicked: %v", recovered)
		}
	}()
	publish(frame)
	return nil
}

func cloneOwnershipCombinations(combinations []OwnershipCombination) []OwnershipCombination {
	clone := make([]OwnershipCombination, len(combinations))
	copy(clone, combinations)
	return clone
}

func cloneOwnershipFrame(frame OwnershipFrame) OwnershipFrame {
	frame.Combinations = cloneOwnershipCombinations(frame.Combinations)
	return frame
}
