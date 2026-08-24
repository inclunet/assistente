// Package profileaccess centraliza descoberta e autorização de mudanças de
// profile iniciadas por tools (AEP-0101).
package profileaccess

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"assistente/internal/profiles"
	"assistente/internal/questionnaire"
)

const (
	ActionAllow = "allow"
	ActionDeny  = "deny"
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

type Service struct {
	profiles     ProfileStore
	asker        Asker
	surface      SurfaceResolver
	availability Availability
}

func NewService(store ProfileStore, asker Asker, surface SurfaceResolver, availability Availability) *Service {
	return &Service{
		profiles:     store,
		asker:        asker,
		surface:      surface,
		availability: availability,
	}
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
	if targetSlug == currentSlug && !req.PersistentSwitch {
		return true, nil
	}

	target, err := s.profiles.Get(targetSlug)
	if err != nil || target == nil {
		return false, fmt.Errorf("profile alvo não encontrado: %s", targetSlug)
	}
	if s.availability != nil && !s.availability(ctx, target) {
		return false, fmt.Errorf("provider do profile alvo está indisponível: %s", targetSlug)
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
		return false, fmt.Errorf("profile alvo deixou de existir: %s", targetSlug)
	}
	if s.availability != nil && !s.availability(ctx, target) {
		return false, fmt.Errorf("provider do profile alvo ficou indisponível: %s", targetSlug)
	}
	return true, nil
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
			Actions: []questionnaire.DecisionAction{
				{
					ID:      ActionAllow,
					Label:   questionnaire.KeyedWith("app.questionnaire.profileSwitch.allow", params, "Trocar para "+targetName),
					Variant: "primary",
					Primary: true,
				},
				{
					ID:      ActionDeny,
					Label:   questionnaire.KeyedWith("app.questionnaire.profileSwitch.deny", params, "Manter "+currentName),
					Variant: "secondary",
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
		Actions: []questionnaire.DecisionAction{
			{
				ID:      ActionAllow,
				Label:   questionnaire.KeyedWith("app.questionnaire.subagentProfile.allow", params, "Executar com "+targetName),
				Variant: "primary",
				Primary: true,
			},
			{
				ID:      ActionDeny,
				Label:   questionnaire.Keyed("app.questionnaire.subagentProfile.deny", "Não executar"),
				Variant: "secondary",
			},
		},
		AllowCancel: true,
	}
}
