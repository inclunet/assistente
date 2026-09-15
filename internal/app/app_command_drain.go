package app

import (
	"context"

	"assistente/internal/commandexecution"
)

// drainCommandExecutors fecha o mesmo core das fábricas do App. A construção
// por sync.Once serializa inclusive uma fábrica concorrente que ainda não
// tinha obtido o core. Depois desta chamada não há registro/admissão nova.
// Não é um binding Wails, não inicializa banco/cofre e não recupera outra
// instância por inferência. Erro impede destruir dependências do executor.
func (a *App) drainCommandExecutors(ctx context.Context) error {
	if a == nil || ctx == nil {
		return commandexecution.ErrInvalidRequest
	}
	core, err := a.commandSecurityService()
	if err != nil {
		return err
	}
	_, err = core.CloseAndDrain(ctx)
	return err
}
