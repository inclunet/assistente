package commandcontext

import (
	"errors"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
)

func TestValidateFreshnessTemporalBoundaries(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	key := FactKey{"surface", "active"}
	policy := commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{
		Provider: key.Provider, Fact: key.Fact, Mode: commandcatalog.MaxAge, MaxAgeMS: 1000,
	}}}

	for _, tc := range []struct {
		name string
		at   time.Time
		want error
	}{
		{"no limite", now.Add(-time.Second), nil},
		{"vencido", now.Add(-time.Second - time.Nanosecond), ErrSnapshotExpired},
		{"futuro", now.Add(time.Nanosecond), ErrInvalidTimestamp},
		{"zero", time.Time{}, ErrInvalidTimestamp},
	} {
		t.Run(tc.name, func(t *testing.T) {
			captured := Snapshots{key: {Version: "v1", CapturedAt: tc.at}}
			current := Snapshots{key: {Version: "v1"}}
			if err := ValidateFreshness(policy, captured, current, now); !errors.Is(err, tc.want) {
				t.Fatalf("erro = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestValidateFreshnessExactVersion(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	key := FactKey{"workspace", "tab"}
	policy := commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: key.Provider, Fact: key.Fact, Mode: commandcatalog.ExactVersion}}}
	captured := Snapshots{key: {Version: "v1"}}

	if err := ValidateFreshness(policy, captured, Snapshots{key: {Version: "v1"}}, now); err != nil {
		t.Fatal(err)
	}
	for _, current := range []Snapshots{{key: {Version: "v2"}}, {key: {}}, {key: {Version: " "}}} {
		if err := ValidateFreshness(policy, captured, current, now); !errors.Is(err, ErrVersionMismatch) {
			t.Fatalf("erro = %v, want version mismatch", err)
		}
	}
}

func TestValidateFreshnessMissingAndInvalidPolicy(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	key := FactKey{"job", "run"}
	valid := commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: key.Provider, Fact: key.Fact, Mode: commandcatalog.EventSnapshot, MaxAgeMS: 1}}}
	if err := ValidateFreshness(valid, nil, nil, now); !errors.Is(err, ErrMissingSnapshot) {
		t.Fatalf("erro = %v, want missing snapshot", err)
	}
	if err := ValidateFreshness(valid, Snapshots{key: {CapturedAt: now}}, nil, now); !errors.Is(err, ErrMissingSnapshot) {
		t.Fatalf("erro = %v, want current snapshot missing", err)
	}

	invalid := []commandcatalog.ContextPolicy{
		{},
		{None: true, Facts: valid.Facts},
		{Facts: []commandcatalog.ContextFact{{Provider: " ", Fact: key.Fact, Mode: commandcatalog.MaxAge, MaxAgeMS: 1}}},
		{Facts: []commandcatalog.ContextFact{{Provider: key.Provider, Fact: key.Fact, Mode: "unknown"}}},
		{Facts: []commandcatalog.ContextFact{{Provider: key.Provider, Fact: key.Fact, Mode: commandcatalog.ExactVersion, MaxAgeMS: 1}}},
		{Facts: []commandcatalog.ContextFact{{Provider: key.Provider, Fact: key.Fact, Mode: commandcatalog.MaxAge, MaxAgeMS: -1}}},
		{Facts: []commandcatalog.ContextFact{{Provider: key.Provider, Fact: key.Fact, Mode: commandcatalog.MaxAge, MaxAgeMS: 1}, {Provider: key.Provider, Fact: key.Fact, Mode: commandcatalog.MaxAge, MaxAgeMS: 1}}},
	}
	for _, policy := range invalid {
		if err := ValidateFreshness(policy, nil, nil, now); !errors.Is(err, ErrInvalidPolicy) {
			t.Fatalf("erro = %v, want invalid policy", err)
		}
	}
}

func TestValidateFreshnessEventSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	key := FactKey{"foreground", "window"}
	policy := commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: key.Provider, Fact: key.Fact, Mode: commandcatalog.EventSnapshot, MaxAgeMS: 10}}}
	for _, tc := range []struct {
		name string
		at   time.Time
		want error
	}{
		{"válido", now.Add(-10 * time.Millisecond), nil},
		{"expirado", now.Add(-10*time.Millisecond - time.Nanosecond), ErrSnapshotExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateFreshness(policy, Snapshots{key: {CapturedAt: tc.at}}, Snapshots{key: {}}, now); !errors.Is(err, tc.want) {
				t.Fatalf("erro = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestValidateFreshnessNone(t *testing.T) {
	if err := ValidateFreshness(commandcatalog.ContextPolicy{None: true}, nil, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFreshness(commandcatalog.ContextPolicy{None: true}, Snapshots{{"p", "f"}: {}}, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateFreshnessHugeTTL(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	key := FactKey{"session", "epoch"}
	policy := commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: key.Provider, Fact: key.Fact, Mode: commandcatalog.MaxAge, MaxAgeMS: int64(^uint64(0) >> 1)}}}
	captured := Snapshots{key: {CapturedAt: now.Add(-time.Hour)}}
	if err := ValidateFreshness(policy, captured, Snapshots{key: {}}, now); !errors.Is(err, ErrTTLUnrepresentable) {
		t.Fatalf("erro = %v, want unrepresentable TTL", err)
	}
}
