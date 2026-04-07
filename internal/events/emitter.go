package events

// Emitter abstrai a emissão de eventos para diferentes interfaces (Wails, CLI, REST, etc.).
// Implementações concretas: WailsEmitter (main package), NoopEmitter (testes).
type Emitter interface {
	Emit(event string, data any)
}

// NoopEmitter descarta todos os eventos. Útil em testes e contextos sem UI.
type NoopEmitter struct{}

func (NoopEmitter) Emit(_ string, _ any) {}
