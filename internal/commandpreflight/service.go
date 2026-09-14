// Package commandpreflight conecta o resolvedor, catálogo e contexto para um
// diagnóstico de leitura. Nenhum resultado autoriza despacho ou consumo de tecla.
// Não é CommandExecutionService: ledger, argumentos, gates de autorização e
// auditoria ainda precisam ser implementados antes de ligar handlers reais.
package commandpreflight

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
)

var (
	ErrInvalidHost     = errors.New("host de diagnóstico inválido")
	ErrInvalidRequest  = errors.New("solicitação de diagnóstico inválida")
	ErrUnauthenticated = errors.New("contexto autenticado ausente")
	ErrStale           = errors.New("snapshot de diagnóstico obsoleto")
)

// HostSnapshot só pode ser produzido pelo host autenticado, nunca pelo payload
// Wails/HTTP. Generation cobre catálogo, configuração, camadas e segurança.
// O host deve incrementá-la em toda mudança semântica; reconstruir objetos
// equivalentes não é mudança e não depende da identidade de ponteiros Go.
// Facts/Dialog são cópias estáveis derivadas dos providers, não dados do cliente.
type HostSnapshot struct {
	UserID        string
	AuthContextID string
	Generation    string
	Registry      *commandcatalog.Registry
	Bindings      *commandbindings.Configuration
	Facts         commandbindings.Facts
	Dialog        *commandbindings.DialogScope
}

// SnapshotProvider deve autenticar novamente a cada consulta, retornando erro
// após logout/revogação. É uma porta interna, não um endpoint público.
type SnapshotProvider func(context.Context) (HostSnapshot, error)

type Diagnostic struct {
	Status         string
	CommandID      string
	BindingIDs     []string
	ContextVersion string
}

const (
	ReadChecksPassed = "read_checks_passed"
	WouldSuppress    = "would_suppress"
)

type Service struct {
	snapshot SnapshotProvider
	versions *commandcontext.VersionService
	now      func() time.Time
}

// New restringe este primeiro fluxo ao teclado local; a origem não é escolhida
// pelo payload. O relógio e os providers são fornecidos pelo bootstrap confiável.
func New(snapshot SnapshotProvider, versions *commandcontext.VersionService, now func() time.Time) (*Service, error) {
	if snapshot == nil || versions == nil || now == nil {
		return nil, ErrInvalidHost
	}
	return &Service{snapshot: snapshot, versions: versions, now: now}, nil
}

func validateHost(h HostSnapshot) error {
	if strings.TrimSpace(h.UserID) == "" || strings.TrimSpace(h.AuthContextID) == "" {
		return ErrUnauthenticated
	}
	if strings.TrimSpace(h.Generation) == "" || h.Registry == nil || h.Bindings == nil {
		return ErrInvalidHost
	}
	return nil
}

// Inspect nunca gera invocation/event IDs nem chama handlers. would_suppress
// é apenas simulação: o adapter NÃO deve fazer preventDefault com este resultado.
// Mesmo read_checks_passed exige validação de argumentos, autorização e ledger
// no executor futuro; este resultado não é token reutilizável de admissão.
func (s *Service) Inspect(ctx context.Context, trigger string) (Diagnostic, error) {
	if s == nil || s.snapshot == nil || s.versions == nil || s.now == nil {
		return Diagnostic{}, ErrInvalidHost
	}
	if ctx == nil || strings.TrimSpace(trigger) == "" {
		return Diagnostic{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return Diagnostic{}, err
	}
	h, err := s.snapshot(ctx)
	if err != nil {
		return Diagnostic{}, err
	}
	if err = validateHost(h); err != nil {
		return Diagnostic{}, err
	}
	// Retém cópias para detectar mudança de contexto entre as consultas do host.
	h.Facts = maps.Clone(h.Facts)
	if h.Dialog != nil {
		d := *h.Dialog
		d.AllowedCommandIDs = slices.Clone(d.AllowedCommandIDs)
		d.AllowedTriggers = slices.Clone(d.AllowedTriggers)
		h.Dialog = &d
	}
	selected, err := h.Bindings.Resolve(trigger, h.Facts, h.Dialog)
	if err != nil {
		return Diagnostic{}, err
	}
	diagnostic := Diagnostic{Status: string(selected.Status), BindingIDs: slices.Clone(selected.BindingIDs)}
	var captured commandcontext.Snapshots
	var policy commandcatalog.ContextPolicy
	if selected.Status == commandbindings.Suppressed {
		diagnostic.Status = WouldSuppress
	}
	if selected.Status == commandbindings.Selected {
		definition, checkErr := h.Registry.CheckReadiness(selected.CommandID, commandcatalog.KeyboardLocal)
		if checkErr != nil {
			return Diagnostic{}, checkErr
		}
		var version string
		var captureErr error
		policy = definition.Context
		captured, version, captureErr = s.versions.Capture(ctx, policy)
		if captureErr != nil {
			return Diagnostic{}, captureErr
		}
		if err = s.versions.Revalidate(ctx, definition.Context, captured, s.now()); err != nil {
			return Diagnostic{}, err
		}
		diagnostic.Status = ReadChecksPassed
		diagnostic.CommandID = definition.ID
		diagnostic.ContextVersion = version
	}
	current, err := s.snapshot(ctx)
	if err != nil {
		return Diagnostic{}, err
	}
	if err = validateHost(current); err != nil {
		return Diagnostic{}, err
	}
	if current.UserID != h.UserID || current.AuthContextID != h.AuthContextID || current.Generation != h.Generation ||
		!reflect.DeepEqual(current.Facts, h.Facts) || !reflect.DeepEqual(current.Dialog, h.Dialog) {
		return Diagnostic{}, ErrStale
	}
	if err = ctx.Err(); err != nil {
		return Diagnostic{}, err
	}
	// A consulta final do host/provider pode demorar. Recalcula a idade no fim,
	// sem transformar o timestamp capturado em uma captura nova. A igualdade de
	// versões já foi reconsultada; isto continua sendo diagnóstico não atômico.
	if selected.Status == commandbindings.Selected {
		if err = commandcontext.ValidateFreshness(policy, captured, captured, s.now()); err != nil {
			return Diagnostic{}, err
		}
	}
	return diagnostic, nil
}
