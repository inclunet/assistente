// Package ports define as interfaces (Outbound Ports) que isolam o núcleo de negócio
// das dependências de infraestrutura (Wails, CLI, HTTP, etc.).
package ports

// WindowPort abstrai operações de controle de janela.
// Implementações concretas: adapters/wails.WindowAdapter (desktop),
// adapters/noop.WindowAdapter (testes/CLI).
type WindowPort interface {
	// Show torna a janela visível e a traz para o primeiro plano.
	Show()
}
