package commandsecurity

import (
	"context"
	"sync"

	"assistente/internal/commandbindings"
)

type executionWatch struct {
	user                   string
	session                string
	authGeneration         string
	securityGeneration     string
	cancel                 context.CancelFunc
	configurationSensitive bool
	projectionProof        func(*commandbindings.Configuration) bool
}

// WatchEpoch acompanha preparação/decisão fora do gate. Não admite execução
// nem substitui autenticação: só entrega um contexto ligado ao epoch já
// capturado pelo host. A inscrição revalida esse epoch sob gate para não perder
// uma invalidação entre a captura e a espera. release é obrigatório/idempotente.
func (s *EpochService) WatchEpoch(ctx context.Context, snapshot EpochSnapshot) (context.Context, func(), error) {
	var watched context.Context
	release, err := s.AdmitExecution(ctx, snapshot, func(context.Context) error { return nil }, func(runCtx context.Context) error {
		watched = runCtx
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return watched, release, nil
}

// WatchSecurityEpoch acompanha a vida da fonte (por exemplo, um run de job),
// não uma resolução de binding. Publicar configuração não encerra a fonte;
// logout, lock, shutdown, invalidação do epoch e cancelamento do pai continuam
// cancelando a inscrição. Não autoriza camada: grants/condições são revalidados
// pelo consumidor a cada uso da fonte. release é obrigatório e idempotente.
func (s *EpochService) WatchSecurityEpoch(ctx context.Context, snapshot EpochSnapshot) (context.Context, func(), error) {
	var watched context.Context
	release, err := s.admitExecution(ctx, snapshot, func(context.Context) error { return nil }, func(runCtx context.Context) error {
		watched = runCtx
		return nil
	}, false, nil)
	if err != nil {
		return nil, nil, err
	}
	return watched, release, nil
}

// AdmitExecution faz a mesma revalidação de Admit e associa ao handoff um
// contexto cancelado por invalidação do epoch. A inscrição ocorre sob o gate,
// sem janela antes de Start. O chamador deve deferir release após o retorno;
// release é idempotente e nunca readquire o gate. Handoff não pode bloquear.
// Invalidação cancela SOMENTE contextos internos: nunca chama Cancel do handler
// nem aguarda efeitos sob o gate. Um efeito já iniciado não é desfeito.
func (s *EpochService) AdmitExecution(ctx context.Context, snapshot EpochSnapshot, revalidate func(context.Context) error, handoff func(context.Context) error) (release func(), err error) {
	return s.admitExecution(ctx, snapshot, revalidate, handoff, true, nil)
}

// AdmitExecutionWithProjectionProof admite uma execução com testemunho
// privado de sua resolução. O testemunho só é consultado durante uma
// publicação explícita de projeção; deve ser função pura de memória, sem I/O,
// gates ou mutexes externos. Ausência, pânico ou resposta negativa cancela a
// execução como qualquer outra publicação de configuração.
func (s *EpochService) AdmitExecutionWithProjectionProof(ctx context.Context, snapshot EpochSnapshot, proof func(*commandbindings.Configuration) bool, revalidate func(context.Context) error, handoff func(context.Context) error) (release func(), err error) {
	if proof == nil {
		return nil, ErrInvalidEpochInput
	}
	return s.admitExecution(ctx, snapshot, revalidate, handoff, true, proof)
}

func (s *EpochService) admitExecution(ctx context.Context, snapshot EpochSnapshot, revalidate func(context.Context) error, handoff func(context.Context) error, configurationSensitive bool, proof func(*commandbindings.Configuration) bool) (release func(), err error) {
	if handoff == nil {
		return nil, ErrInvalidEpochInput
	}
	err = s.Admit(ctx, snapshot, revalidate, func() error {
		runCtx, cancel := context.WithCancel(ctx)
		watch := &executionWatch{user: snapshot.UserID, session: snapshot.SessionID, authGeneration: snapshot.AuthGeneration, securityGeneration: snapshot.SecurityGeneration, cancel: cancel, configurationSensitive: configurationSensitive, projectionProof: proof}
		s.watchesMu.Lock()
		if s.watches == nil {
			s.watches = make(map[*executionWatch]struct{})
		}
		s.watches[watch] = struct{}{}
		s.watchesMu.Unlock()
		var once sync.Once
		release = func() {
			once.Do(func() {
				s.watchesMu.Lock()
				delete(s.watches, watch)
				s.watchesMu.Unlock()
				cancel()
			})
		}
		accepted := false
		defer func() {
			if !accepted {
				release()
			}
		}()
		if err := handoff(runCtx); err != nil {
			return err
		}
		accepted = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return release, nil
}

// Exige o gate exclusivo do chamador. Não executa callbacks de usuário.
func (s *EpochService) cancelExecutions(session string, all bool) {
	s.watchesMu.Lock()
	defer s.watchesMu.Unlock()
	for watch := range s.watches {
		if all || watch.session == session {
			watch.cancel()
			delete(s.watches, watch)
		}
	}
}
