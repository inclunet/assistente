package cli

// WindowAdapter é uma implementação no-op de ports.WindowPort para o modo CLI.
// No terminal, não há janela para mostrar.
type WindowAdapter struct{}

// Show é no-op no CLI — não há janela para exibir.
func (WindowAdapter) Show() {}
