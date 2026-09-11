// Package profileaccess centraliza descoberta e autorização de mudanças de
// profile iniciadas por tools (AEP-0101).
package profileaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"assistente/internal/eventctx"
	"assistente/internal/jobprofilegrant"
	"assistente/internal/profiles"
	"assistente/internal/questionnaire"
)

const (
	ActionAllow = "allow"
	ActionDeny  = "deny"
)

var (
	ErrTargetNotFound          = errors.New("profile alvo não encontrado")
	ErrTargetUnavailable       = errors.New("provider do profile alvo indisponível")
	ErrAuthorizationNotGranted = jobprofilegrant.ErrAuthorizationNotGranted
)

// ProfileStore é a leitura mínima do catálogo persistido de profiles.
type ProfileStore interface {
	List() ([]profiles.ProfileInfo, error)
	Get(slug string) (*profiles.Profile, error)
}

// Asker apresenta uma decisão na superfície de origem.
type Asker interface {
	Ask(context.Context, questionnaire.Surface, questionnaire.RequestPayload) (questionnaire.Response, error)
}

// SurfaceResolver descobre onde a decisão deve ser apresentada.
type SurfaceResolver func(context.Context, string, string) questionnaire.Surface

// Availability informa se o provider configurado para o profile está
// disponível sem fazer uma chamada de rede.
type Availability func(context.Context, *profiles.Profile) bool

type JobGrantStore interface {
	CurrentDelegation(context.Context, string) (jobprofilegrant.DelegationConfig, error)
	AuthorizationSnapshot(context.Context, string, string) (jobprofilegrant.AuthorizationSnapshot, error)
	HasValid(context.Context, string, string, string) (bool, error)
	ListValid(context.Context, string) ([]jobprofilegrant.Grant, jobprofilegrant.DelegationConfig, error)
	Grant(context.Context, string, string, string, string, uint64) error
	Revoke(context.Context, string, string, string) error
	RevokeProfileGlobal(context.Context, string, string) error
}

type Service struct {
	profiles     ProfileStore
	asker        Asker
	surface      SurfaceResolver
	availability Availability
	grants       JobGrantStore
	profileMu    sync.Mutex
	profileEpoch map[string]uint64
}

func NewService(store ProfileStore, asker Asker, surface SurfaceResolver, availability Availability) *Service {
	return &Service{
		profiles:     store,
		asker:        asker,
		surface:      surface,
		availability: availability,
	}
}

// WithJobGrants habilita o contrato persistente específico para origens job.
func (s *Service) WithJobGrants(store JobGrantStore) *Service {
	if s != nil {
		s.grants = store
	}
	return s
}

type ProfileSummary struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Current     bool   `json:"current"`
	Available   bool   `json:"available"`
}

func (s *Service) List(ctx context.Context, currentSlug string) ([]ProfileSummary, error) {
	if s == nil || s.profiles == nil {
		return nil, errors.New("catálogo de profiles indisponível")
	}
	infos, err := s.profiles.List()
	if err != nil {
		return nil, err
	}
	currentSlug = strings.TrimSpace(currentSlug)
	result := make([]ProfileSummary, 0, len(infos))
	for _, info := range infos {
		slug := strings.TrimSpace(info.Slug)
		if slug == "" {
			continue
		}
		available := true
		if s.availability != nil {
			profile, getErr := s.profiles.Get(slug)
			available = getErr == nil && profile != nil && s.availability(ctx, profile)
		}
		result = append(result, ProfileSummary{
			Slug:        slug,
			Name:        strings.TrimSpace(info.Name),
			Description: strings.TrimSpace(info.Description),
			Current:     slug == currentSlug,
			Available:   available,
		})
	}
	return result, nil
}

func (s *Service) ValidateTarget(ctx context.Context, targetSlug string) error {
	if s == nil || s.profiles == nil {
		return errors.New("serviço de profiles indisponível")
	}
	targetSlug = strings.TrimSpace(targetSlug)
	if targetSlug == "" {
		return errors.New("profile alvo é obrigatório")
	}
	target, err := s.profiles.Get(targetSlug)
	if err != nil || target == nil {
		return fmt.Errorf("%w: %s", ErrTargetNotFound, targetSlug)
	}
	if s.availability != nil && !s.availability(ctx, target) {
		return fmt.Errorf("%w: %s", ErrTargetUnavailable, targetSlug)
	}
	return nil
}

type AuthorizationRequest struct {
	Source           string
	ConversationID   string
	CurrentSlug      string
	TargetSlug       string
	TaskTitle        string
	Background       bool
	PersistentSwitch bool
}

// Authorize valida o profile alvo e pede uma autorização por invocação.
// O caller é responsável por revalidar seu recurso mutável antes de executar.
func (s *Service) Authorize(ctx context.Context, req AuthorizationRequest) (bool, error) {
	if s == nil || s.profiles == nil {
		return false, errors.New("serviço de profiles indisponível")
	}
	currentSlug := strings.TrimSpace(req.CurrentSlug)
	targetSlug := strings.TrimSpace(req.TargetSlug)
	if targetSlug == "" {
		return false, errors.New("profile alvo é obrigatório")
	}
	if provenance, ok := eventctx.From(ctx); ok && provenance.Source == "job" {
		if s.grants == nil || strings.TrimSpace(provenance.SourceJobID) == "" {
			return false, ErrAuthorizationNotGranted
		}
		config, err := s.grants.CurrentDelegation(ctx, provenance.SourceJobID)
		if err != nil {
			return false, fmt.Errorf("%w: %v", ErrAuthorizationNotGranted, err)
		}
		if err := s.ValidateTarget(ctx, targetSlug); err != nil {
			return false, err
		}
		allowed, err := s.grants.HasValid(ctx, config.JobID, targetSlug, config.Fingerprint)
		if err != nil {
			return false, fmt.Errorf("%w: %v", ErrAuthorizationNotGranted, err)
		}
		if !allowed {
			return false, ErrAuthorizationNotGranted
		}
		return true, nil
	}
	if targetSlug == currentSlug && !req.PersistentSwitch {
		return true, nil
	}

	target, err := s.profiles.Get(targetSlug)
	if err != nil || target == nil {
		return false, fmt.Errorf("%w: %s", ErrTargetNotFound, targetSlug)
	}
	if s.availability != nil && !s.availability(ctx, target) {
		return false, fmt.Errorf("%w: %s", ErrTargetUnavailable, targetSlug)
	}
	currentName := currentSlug
	if current, getErr := s.profiles.Get(currentSlug); getErr == nil && current != nil && strings.TrimSpace(current.Name) != "" {
		currentName = strings.TrimSpace(current.Name)
	}
	targetName := strings.TrimSpace(target.Name)
	if targetName == "" {
		targetName = targetSlug
	}

	if s.asker == nil || s.surface == nil {
		return false, questionnaire.ErrAskerUnavailable
	}
	surface := s.surface(ctx, strings.TrimSpace(req.Source), strings.TrimSpace(req.ConversationID))
	if !surface.HasInterlocutor() {
		return false, questionnaire.ErrNoInterlocutor
	}
	payload := authorizationPayload(req, currentName, targetName)
	response, err := s.asker.Ask(ctx, surface, payload)
	if err != nil {
		return false, err
	}
	actionID, ok := questionnaire.DecisionActionID(response)
	if !ok || actionID != ActionAllow {
		return false, nil
	}
	// A pessoa pode manter o diálogo aberto enquanto o profile é editado ou
	// removido. Revalida imediatamente antes de liberar o caller.
	target, err = s.profiles.Get(targetSlug)
	if err != nil || target == nil {
		return false, fmt.Errorf("%w após autorização: %s", ErrTargetNotFound, targetSlug)
	}
	if s.availability != nil && !s.availability(ctx, target) {
		return false, fmt.Errorf("%w após autorização: %s", ErrTargetUnavailable, targetSlug)
	}
	return true, nil
}

type JobGrantState struct {
	JobID             string                  `json:"jobId"`
	JobSlug           string                  `json:"jobSlug"`
	JobName           string                  `json:"jobName"`
	ProfileExpression string                  `json:"profileExpression"`
	Fingerprint       string                  `json:"fingerprint"`
	Dynamic           bool                    `json:"dynamic"`
	Grants            []jobprofilegrant.Grant `json:"grants"`
}

func (s *Service) JobGrantState(ctx context.Context, jobID string) (JobGrantState, error) {
	if s == nil || s.grants == nil {
		return JobGrantState{}, errors.New("store de grants indisponível")
	}
	grants, config, err := s.grants.ListValid(ctx, jobID)
	if err != nil {
		return JobGrantState{}, err
	}
	return JobGrantState{
		JobID:             config.JobID,
		JobSlug:           config.JobSlug,
		JobName:           config.JobName,
		ProfileExpression: config.ProfileExpression,
		Fingerprint:       config.Fingerprint,
		Dynamic:           strings.Contains(config.ProfileExpression, "{{"),
		Grants:            grants,
	}, nil
}

// AuthorizeJobTarget cria um grant somente após decisão explícita no desktop.
// Job, fingerprint e profile são revalidados depois da resposta.
func (s *Service) AuthorizeJobTarget(ctx context.Context, surface questionnaire.Surface, jobID, targetSlug string) (bool, error) {
	if s == nil || s.grants == nil || s.asker == nil {
		return false, questionnaire.ErrAskerUnavailable
	}
	if !surface.AllowsPersistentAuthorization() {
		return false, questionnaire.ErrNoInterlocutor
	}
	targetSlug = strings.TrimSpace(targetSlug)
	snapshot, err := s.grants.AuthorizationSnapshot(ctx, jobID, targetSlug)
	if err != nil {
		return false, err
	}
	before := snapshot.Config
	if !strings.Contains(before.ProfileExpression, "{{") && targetSlug != before.ProfileExpression {
		return false, fmt.Errorf("profile alvo não corresponde à configuração literal do job")
	}
	s.profileMu.Lock()
	if err := s.ValidateTarget(ctx, targetSlug); err != nil {
		s.profileMu.Unlock()
		return false, err
	}
	target, _ := s.profiles.Get(targetSlug)
	targetIdentity := profileIdentity(target)
	targetEpoch := s.profileEpoch[targetSlug]
	s.profileMu.Unlock()
	targetName := targetSlug
	if target != nil && strings.TrimSpace(target.Name) != "" {
		targetName = strings.TrimSpace(target.Name)
	}
	response, err := s.asker.Ask(ctx, surface, jobAuthorizationPayload(before.JobName, targetName))
	if err != nil {
		return false, err
	}
	actionID, ok := questionnaire.DecisionActionID(response)
	if !ok || actionID != ActionAllow {
		return false, nil
	}
	s.profileMu.Lock()
	defer s.profileMu.Unlock()
	if s.profileEpoch[targetSlug] != targetEpoch {
		return false, errors.New("profile foi removido durante a autorização")
	}
	after, err := s.grants.CurrentDelegation(ctx, jobID)
	if err != nil {
		return false, err
	}
	if after.JobID != before.JobID || after.Fingerprint != before.Fingerprint {
		return false, errors.New("job ou configuração mudou durante a autorização")
	}
	if err := s.ValidateTarget(ctx, targetSlug); err != nil {
		return false, err
	}
	currentTarget, _ := s.profiles.Get(targetSlug)
	if profileIdentity(currentTarget) != targetIdentity {
		return false, errors.New("profile mudou durante a autorização")
	}
	if err := s.grants.Grant(ctx, after.JobID, targetSlug, after.Fingerprint, "desktop", snapshot.Generation); err != nil {
		return false, err
	}
	return true, nil
}

// DeleteProfile serializa a remoção do arquivo com a concessão de grants.
// A revogação global só é confirmada após a exclusão bem-sucedida; o mutex
// impede que uma autorização pendente seja persistida entre as duas etapas.
func (s *Service) DeleteProfile(ctx context.Context, targetSlug string, deleteProfile func() error) error {
	if s == nil || s.grants == nil {
		return errors.New("store de grants indisponível")
	}
	if deleteProfile == nil {
		return errors.New("operação de exclusão de profile indisponível")
	}
	s.profileMu.Lock()
	defer s.profileMu.Unlock()
	if err := deleteProfile(); err != nil {
		return err
	}
	if s.profileEpoch == nil {
		s.profileEpoch = make(map[string]uint64)
	}
	s.profileEpoch[strings.TrimSpace(targetSlug)]++
	return s.grants.RevokeProfileGlobal(ctx, targetSlug, "profile excluído")
}

func (s *Service) RevokeJobTarget(ctx context.Context, jobID, targetSlug string) error {
	if s == nil || s.grants == nil {
		return errors.New("store de grants indisponível")
	}
	return s.grants.Revoke(ctx, jobID, targetSlug, "revogação no desktop")
}

func jobAuthorizationPayload(jobName, targetName string) questionnaire.RequestPayload {
	params := map[string]any{"jobName": jobName, "targetProfile": targetName}
	return questionnaire.RequestPayload{
		Kind:  questionnaire.KindDecision,
		Title: questionnaire.Keyed("app.questionnaire.jobProfileGrant.title", "Autorizar profile para este job?"),
		Description: questionnaire.KeyedWith(
			"app.questionnaire.jobProfileGrant.description",
			params,
			fmt.Sprintf("O job %s poderá executar sozinho usando somente o profile %s enquanto essa configuração não mudar.", jobName, targetName),
		),
		Actions: []questionnaire.DecisionAction{
			{
				ID: ActionAllow, Label: questionnaire.KeyedWith("app.questionnaire.jobProfileGrant.allow", params, "Autorizar "+targetName),
				Variant: "primary", Primary: true, Polarity: questionnaire.DecisionPolarityAffirmative, Scope: questionnaire.DecisionScopePersistent,
			},
			{
				ID: ActionDeny, Label: questionnaire.Keyed("app.questionnaire.jobProfileGrant.deny", "Não autorizar"),
				Variant: "secondary", Polarity: questionnaire.DecisionPolarityNegative, Scope: questionnaire.DecisionScopePersistent,
			},
		},
		AllowCancel: true,
	}
}

func profileIdentity(profile *profiles.Profile) string {
	if profile == nil {
		return ""
	}
	data, err := json.Marshal(profile)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func authorizationPayload(req AuthorizationRequest, currentName, targetName string) questionnaire.RequestPayload {
	params := map[string]any{
		"currentProfile": currentName,
		"targetProfile":  targetName,
	}
	if req.PersistentSwitch {
		return questionnaire.RequestPayload{
			Kind: questionnaire.KindDecision,
			Title: questionnaire.Keyed(
				"app.questionnaire.profileSwitch.title",
				"Trocar o profile desta conversa?",
			),
			Description: questionnaire.KeyedWith(
				"app.questionnaire.profileSwitch.description",
				params,
				fmt.Sprintf("O profile mudará de %s para %s a partir do próximo turno.", currentName, targetName),
			),
			Body: strings.TrimSpace(req.TaskTitle),
			BodyLabel: questionnaire.Keyed(
				"app.questionnaire.profileSwitch.bodyLabel",
				"Tarefa solicitada",
			),
			Actions: []questionnaire.DecisionAction{
				{
					ID:       ActionAllow,
					Label:    questionnaire.KeyedWith("app.questionnaire.profileSwitch.allow", params, "Trocar para "+targetName),
					Variant:  "primary",
					Primary:  true,
					Polarity: questionnaire.DecisionPolarityAffirmative,
					Scope:    questionnaire.DecisionScopeConversation,
				},
				{
					ID:       ActionDeny,
					Label:    questionnaire.KeyedWith("app.questionnaire.profileSwitch.deny", params, "Manter "+currentName),
					Variant:  "secondary",
					Polarity: questionnaire.DecisionPolarityNegative,
					Scope:    questionnaire.DecisionScopeConversation,
				},
			},
			AllowCancel: true,
		}
	}

	descriptionKey := "app.questionnaire.subagentProfile.descriptionInline"
	descriptionFallback := fmt.Sprintf(
		"O subagente usará %s em vez de %s somente nesta execução, e o turno aguardará o resultado.",
		targetName,
		currentName,
	)
	if req.Background {
		descriptionKey = "app.questionnaire.subagentProfile.descriptionBackground"
		descriptionFallback = fmt.Sprintf(
			"O subagente usará %s em vez de %s somente nesta execução em segundo plano; o resultado chegará depois nesta conversa.",
			targetName,
			currentName,
		)
	}
	return questionnaire.RequestPayload{
		Kind: questionnaire.KindDecision,
		Title: questionnaire.Keyed(
			"app.questionnaire.subagentProfile.title",
			"Executar a tarefa com outro profile?",
		),
		Description: questionnaire.KeyedWith(
			descriptionKey,
			params,
			descriptionFallback,
		),
		Body: strings.TrimSpace(req.TaskTitle),
		BodyLabel: questionnaire.Keyed(
			"app.questionnaire.subagentProfile.bodyLabel",
			"Tarefa solicitada",
		),
		Actions: []questionnaire.DecisionAction{
			{
				ID:       ActionAllow,
				Label:    questionnaire.KeyedWith("app.questionnaire.subagentProfile.allow", params, "Executar com "+targetName),
				Variant:  "primary",
				Primary:  true,
				Polarity: questionnaire.DecisionPolarityAffirmative,
				Scope:    questionnaire.DecisionScopeCurrent,
			},
			{
				ID:       ActionDeny,
				Label:    questionnaire.Keyed("app.questionnaire.subagentProfile.deny", "Não executar"),
				Variant:  "secondary",
				Polarity: questionnaire.DecisionPolarityNegative,
				Scope:    questionnaire.DecisionScopeCurrent,
			},
		},
		AllowCancel: true,
	}
}
