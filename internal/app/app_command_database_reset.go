package app

import (
	"context"
	"fmt"
	"time"
)

// beforeCommandDatabaseReset encerra o domínio de comandos antes que o
// controller de settings possa fechar ou substituir o banco. O App fica
// terminal para comandos nesta instância; o próximo processo fará um novo
// bootstrap, sem tentar recriar um core sobre o banco resetado.
func (a *App) beforeCommandDatabaseReset() error {
	if a == nil {
		return fmt.Errorf("app inválido")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.shutdownCommandLifecycleIfConfigured(ctx); err != nil {
		return fmt.Errorf("encerrar lifecycle de comandos: %w", err)
	}
	if err := a.drainCommandExecutors(ctx); err != nil {
		return fmt.Errorf("drenar executores de comandos: %w", err)
	}
	if err := a.shutdownCommandBridgeIfConfigured(ctx); err != nil {
		return fmt.Errorf("encerrar ponte de comandos: %w", err)
	}
	return nil
}
