// Package commandexecution implementa o executor interno restrito a leituras
// local_session diretas, sem argumentos, workspace, providers ou decisões.
// Não registra rotas Wails/CLI nem inicializa banco ou segredos reais.
package commandexecution

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
)

var (
	ErrInvalidConfiguration = errors.New("configuração de executor inválida")
	ErrInvalidRequest       = errors.New("solicitação de comando inválida")
	ErrDenied               = errors.New("comando não autorizado ou indisponível")
	ErrStale                = errors.New("contexto de comando obsoleto")
	ErrExecution            = errors.New("falha interna de execução")
)

// Request contém somente candidatos de ingresso. Identidade, origem, relógio,
// versões, fingerprints e políticas são derivados pelo host, nunca pelo cliente.
type Request struct{ InvocationID, CorrelationID, CommandID string }

// Versions é lido pelo host sob o gate. Mudanças devem usar o mesmo EpochService.
// Unlocked inclui TODOS os locks exigidos pela origem (cofre e SO, se aplicável).
type Versions struct {
	Registry, GlobalConfig, ActiveLayers string
	Unlocked                             bool
}

// Invocation é uma cópia sem tokens/segredos entregue exclusivamente ao handler.
type Invocation struct {
	Envelope                     *commandcontract.Envelope
	ID, CorrelationID, CommandID string
	Principal                    auth.LocalSessionPrincipal
	Source                       commandcatalog.Source
}

// Outcome só admite succeeded, failed ou cancelled, confirmados pelo handler.
// Sem resultado explícito, o executor não presume ausência de efeitos.
type Outcome struct {
	Status commandledger.Status
	Result json.RawMessage
}

// ExecutionHandle é retornado por Start sem esperar o trabalho terminar.
// Done entrega um outcome; Cancel deve ser idempotente e não bloqueante.
type ExecutionHandle struct {
	ID     string
	Done   <-chan Outcome
	Cancel func()
}

type Handler struct {
	Contract commandcatalog.HandlerContract
	// Start não pode readquirir o gate, aguardar rede/UI/trabalho nem bloquear.
	// Qualquer erro/panic após entrar em Start é inconclusivo nesta versão.
	Start func(context.Context, Invocation) (ExecutionHandle, error)
}

// Config é exclusivamente de bootstrap confiável. Authorize precisa reconsultar
// política e disponibilidade atuais; nem role no JWT nem metadata bastam.
// Snapshot/Authorize são callbacks curtos sob gate, sem reentrada, UI ou rede.
// Registry e Handlers são um snapshot imutável; trocar rotas exige novo serviço
// e publicar uma nova versão sob o mesmo gate. Source é fixada pelo adapter.
type Config struct {
	Envelope            *EnvelopeConfig
	Sessions            *auth.SessionService
	Epochs              *commandsecurity.EpochService
	Store               *commandledger.Store
	Registry            *commandcatalog.Registry
	RegistryVersion     string
	Handlers            map[string]Handler
	Source              commandcatalog.Source
	Snapshot            func(context.Context, auth.LocalSessionPrincipal) (Versions, error)
	Authorize           func(context.Context, auth.LocalSessionPrincipal, string, commandcatalog.Source) error
	Keys                commandledger.FingerprintKeyProvider
	KeyVersion          string
	Now                 func() time.Time
	Retention           time.Duration
	ExecutionTimeout    time.Duration
	FinalizationTimeout time.Duration
}
