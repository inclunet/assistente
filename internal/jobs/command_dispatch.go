package jobs

import (
	"context"
	"fmt"

	"assistente/internal/database"
)

// PrepareCommandJob lê uma definição de job pela identidade física exata e
// devolve um snapshot isolado para a ponte de comandos. A preparação não
// habilita, registra trigger, cria grant nem executa o job.
//
// O contexto precisa carregar explicitamente o mesmo usuário do Manager. Não
// usamos contextFrom aqui: aceitar o usuário do Manager no lugar do contexto
// do caller esconderia uma tentativa de cruzar proprietário.
func (m *Manager) PrepareCommandJob(ctx context.Context, jobDatabaseID string) (*Job, error) {
	if m == nil || m.cfg.Repository == nil || ctx == nil {
		return nil, ErrCommandJobDenied
	}

	userID, err := database.RequireUserID(ctx)
	if err != nil || userID == "" {
		return nil, ErrCommandJobDenied
	}
	ownerID, ok := database.UserIDFromContext(m.context())
	if !ok || ownerID == "" || ownerID != userID {
		return nil, ErrCommandJobDenied
	}
	if !validJobDefinitionDatabaseID(jobDatabaseID) {
		return nil, ErrCommandJobDenied
	}

	// GetJobByID é a porta de identidade física; buscar por slug permitiria
	// que uma definição com o mesmo nome fosse confundida com o job solicitado.
	job, err := m.cfg.Repository.GetJobByID(ctx, jobDatabaseID)
	if err != nil {
		return nil, err
	}
	if job == nil || job.DatabaseID != jobDatabaseID {
		return nil, fmt.Errorf("%w: identidade física do job não confere", ErrCommandJobDenied)
	}

	snapshot, err := cloneJob(job)
	if err != nil {
		return nil, err
	}
	// DatabaseID é deliberadamente json:"-" em Job; cloneJob preserva o
	// isolamento de mapas/slices, mas exige esta restauração explícita.
	snapshot.DatabaseID = job.DatabaseID
	return snapshot, nil
}
