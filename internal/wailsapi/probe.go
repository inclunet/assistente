// Package wailsapi agrupa structs bindados no Wails além do *App (AEP-0088).
//
// A migração Strangler Fig tira domínios do monólito App e registra um bind
// por domínio. Este pacote começa pelo spike Probe (Fase 1) e cresce com
// Tokens, Skills, etc.
package wailsapi

// Probe é o spike de multi-bind da AEP-0088 Fase 1: um struct bindado ao lado
// do App, com um único método cujo nome não colide com nenhum método do App.
// Não é superfície de produto — só prova que o Wails gera módulo separado e
// que o CI de bindings aceita mais de um Bind.
type Probe struct{}

// NewProbe cria o spike de multi-bind.
func NewProbe() *Probe {
	return &Probe{}
}

// StranglerFigProbe devolve um marcador fixo para testes e para confirmar que
// o frontend/bindings enxergam o segundo bind. Não tem efeito colateral.
func (p *Probe) StranglerFigProbe() string {
	return "aep-0088-probe-ok"
}
