package commandsecurity

import "context"

// AdmitMutation revalida a sessão/epochs sob gate exclusivo sem invalidar
// sessões nem workspaces independentes. A ação publica sua geração de escopo
// atomicamente com os dados; execuções aguardando fila a reconsultam. Segurança
// usa MutateSession/MutateSecurity, não esta porta de configuração.
func (s *EpochService) AdmitMutation(ctx context.Context, snapshot EpochSnapshot, revalidate func(context.Context) error, action func() error) error {
	if !s.valid() || ctx == nil || revalidate == nil || action == nil {
		return ErrInvalidEpochInput
	}
	return s.gate.WithMutation(ctx, func() error {
		if s.disabled || s.transitions != 0 {
			return ErrStaleEpoch
		}
		current, ok := s.sessions[snapshot.SessionID]
		if !ok || current.user != snapshot.UserID || current.generation != snapshot.AuthGeneration || s.security != snapshot.SecurityGeneration {
			return ErrStaleEpoch
		}
		if err := revalidate(ctx); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return action()
	})
}
