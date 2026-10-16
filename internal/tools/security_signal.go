package tools

import (
	"context"
	"sync"
)

const SecuritySignalsMetadataKey = "security_signals"

// SecuritySignal é evidência factual de uma decisão de segurança ocorrida
// durante uma invocação. Não contém o alvo para não ampliar exposição no card.
type SecuritySignal struct {
	Version   int    `json:"version"`
	Domain    string `json:"domain"`
	Outcome   string `json:"outcome"`
	Source    string `json:"source"`
	Scope     string `json:"scope,omitempty"`
	Operation string `json:"operation,omitempty"`
}

type securitySignalCollector struct {
	mu      sync.Mutex
	signals []SecuritySignal
}
type securitySignalContextKey struct{}

func WithSecuritySignalCollector(ctx context.Context) context.Context {
	return withSecuritySignalCollector(ctx, &securitySignalCollector{})
}

func withSecuritySignalCollector(ctx context.Context, collector *securitySignalCollector) context.Context {
	return context.WithValue(ctx, securitySignalContextKey{}, collector)
}

func RecordSecuritySignal(ctx context.Context, signal SecuritySignal) {
	collector, _ := ctx.Value(securitySignalContextKey{}).(*securitySignalCollector)
	if collector == nil || signal.Version != 1 {
		return
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	collector.signals = append(collector.signals, signal)
}

func SecuritySignalsFrom(ctx context.Context) []SecuritySignal {
	collector, _ := ctx.Value(securitySignalContextKey{}).(*securitySignalCollector)
	if collector == nil {
		return nil
	}
	return collector.snapshot()
}

func (collector *securitySignalCollector) snapshot() []SecuritySignal {
	if collector == nil {
		return nil
	}
	collector.mu.Lock()
	defer collector.mu.Unlock()
	return append([]SecuritySignal(nil), collector.signals...)
}
