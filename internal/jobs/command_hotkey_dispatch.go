package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"assistente/internal/commandjson"
	"assistente/internal/database"
	"assistente/internal/hotkey"
	"assistente/internal/logging"
)

const commandHotkeyBindingFingerprintDomain = "assistente.command-hotkey-binding.v1"

// CommandHotkeyBinding é a projeção autoritativa de um trigger hotkey de job.
// A projeção não contém o proprietário: ele é obtido do contexto autenticado.
type CommandHotkeyBinding struct {
	JobDatabaseID         string `json:"job_database_id"`
	JobSlug               string `json:"job_slug"`
	Keys                  string `json:"keys"`
	When                  string `json:"when,omitempty"`
	DefinitionFingerprint string `json:"definition_fingerprint"`
	BindingFingerprint    string `json:"binding_fingerprint"`
}

// CommandHotkeyOccurrence é uma prova opaca produzida pelo callback nativo.
// Seus campos deliberadamente não são exportados: a integração do App só pode
// obter a projeção e preparar o contexto através dos métodos abaixo.
type CommandHotkeyOccurrence struct {
	manager  *Manager
	binding  preparedHotkeyBinding
	lifetime *hotkeyRegistrationLifetime
}

// Binding devolve uma cópia da identidade capturada no registro.
func (o CommandHotkeyOccurrence) Binding() CommandHotkeyBinding {
	return CommandHotkeyBinding{
		JobDatabaseID:         o.binding.jobDatabaseID,
		JobSlug:               o.binding.jobSlug,
		Keys:                  o.binding.keys,
		When:                  o.binding.when,
		DefinitionFingerprint: o.binding.definitionFingerprint,
		BindingFingerprint:    o.binding.bindingFingerprint,
	}
}

// Current informa apenas se o lifetime em memória ainda está ativo. Não faz
// leitura no banco; Validate é o método que revalida a autoridade persistida.
func (o CommandHotkeyOccurrence) Current() bool {
	return o.manager != nil && o.lifetime != nil &&
		o.lifetime.active.Load() && o.lifetime.ctx != nil && o.lifetime.ctx.Err() == nil
}

// Validate revalida proprietário, definição e trigger contra a fonte
// persistida atual, além de rejeitar ocorrências zeradas ou aposentadas.
func (o CommandHotkeyOccurrence) Validate(ctx context.Context) error {
	if !o.Current() || ctx == nil || ctx.Err() != nil || o.manager == nil {
		return errPreparedHotkeyDenied
	}
	if _, err := o.manager.revalidateHotkeyBinding(ctx, o.binding); err != nil {
		return err
	}
	return nil
}

// Context prepara o contexto privado que o handler global exige. A derivação
// preserva valores e cancelamento do chamador e também cancela quando o
// registro nativo deixa de existir.
func (o CommandHotkeyOccurrence) Context(ctx context.Context) (context.Context, error) {
	if err := o.Validate(ctx); err != nil {
		return nil, err
	}
	prepared, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-o.lifetime.ctx.Done():
			cancel()
		case <-prepared.Done():
		}
	}()
	return withPreparedHotkeyDispatch(prepared, o.binding), nil
}

type commandHotkeyBindingFingerprintProjection struct {
	DefinitionFingerprint string `json:"definition_fingerprint"`
	Keys                  string `json:"keys"`
	When                  string `json:"when"`
}

func commandHotkeyBindingFingerprint(definitionFingerprint, keys, when string) (string, error) {
	definitionFingerprint = strings.TrimSpace(definitionFingerprint)
	keys = strings.TrimSpace(keys)
	when = strings.TrimSpace(when)
	if definitionFingerprint == "" || keys == "" {
		return "", errPreparedHotkeyDenied
	}
	payload, err := commandjson.Marshal(commandHotkeyBindingFingerprintProjection{
		DefinitionFingerprint: definitionFingerprint,
		Keys:                  keys,
		When:                  when,
	})
	if err != nil {
		return "", err
	}
	framed := make([]byte, 0, len(commandHotkeyBindingFingerprintDomain)+1+len(payload))
	framed = append(framed, commandHotkeyBindingFingerprintDomain...)
	framed = append(framed, 0)
	framed = append(framed, payload...)
	digest := sha256.Sum256(framed)
	return hex.EncodeToString(digest[:]), nil
}

// CommandHotkeyBindings lê a projeção diretamente do repositório do mesmo
// proprietário. Não depende de Start nem do registry em memória, pois é uma
// leitura sem efeitos usada pela projeção de produto.
func (m *Manager) CommandHotkeyBindings(ctx context.Context) ([]CommandHotkeyBinding, error) {
	if m == nil || m.cfg.Repository == nil || ctx == nil || ctx.Err() != nil {
		return nil, errPreparedHotkeyDenied
	}
	owner, err := database.RequireUserID(ctx)
	if err != nil || owner == "" || !m.hotkeyOwnerMatches(ctx, owner) {
		return nil, errPreparedHotkeyDenied
	}
	enabled := true
	jobs, err := m.cfg.Repository.ListJobs(ctx, JobFilter{Enabled: &enabled})
	if err != nil {
		return nil, fmt.Errorf("list command hotkey bindings: %w", err)
	}

	bindings := make([]CommandHotkeyBinding, 0)
	for i := range jobs {
		job := &jobs[i]
		if !m.effectiveJobEnabled(job) {
			continue
		}
		definitionFingerprint, err := DefinitionFingerprint(job)
		if err != nil {
			return nil, fmt.Errorf("fingerprint job %s: %w", job.ID, err)
		}
		for _, trigger := range job.Triggers {
			if trigger.Type != TriggerHotkey {
				continue
			}
			keys := strings.TrimSpace(trigger.Keys)
			when := strings.TrimSpace(trigger.When)
			if keys == "" {
				logging.Warnf(ctx, "jobs.manager", "ignorando hotkey vazia persistida do job %s; o registro nativo também a recusa", job.ID)
				continue
			}
			if _, _, err := hotkey.ParseCombination(keys); err != nil {
				logging.Warnf(ctx, "jobs.manager", "ignorando hotkey persistida inválida do job %s (%q); o registro nativo também a recusa: %v", job.ID, keys, err)
				continue
			}
			bindingFingerprint, err := commandHotkeyBindingFingerprint(definitionFingerprint, keys, when)
			if err != nil {
				return nil, fmt.Errorf("fingerprint hotkey %s/%s: %w", job.ID, keys, err)
			}
			bindings = append(bindings, CommandHotkeyBinding{
				JobDatabaseID:         job.DatabaseID,
				JobSlug:               job.ID,
				Keys:                  keys,
				When:                  when,
				DefinitionFingerprint: definitionFingerprint,
				BindingFingerprint:    bindingFingerprint,
			})
		}
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].JobSlug != bindings[j].JobSlug {
			return bindings[i].JobSlug < bindings[j].JobSlug
		}
		if bindings[i].Keys != bindings[j].Keys {
			return bindings[i].Keys < bindings[j].Keys
		}
		return bindings[i].When < bindings[j].When
	})
	return bindings, nil
}
