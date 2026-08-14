package wailsapi

import (
	"assistente/internal/acptrust"
	"assistente/internal/apidto"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ACPTrust é o bind Wails do domínio acp_trust — autorizações permanentes
// concedidas aos agentes de código (AEP-0088 / AEP-0084 D9).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
//
// Handlers de pedido de permissão em tempo de turno (app_acp_permissions.go)
// permanecem no *App.
type ACPTrust struct {
	mu           sync.RWMutex
	session      Session
	trust        *acptrust.Store
	profileNames func() map[string]string
}

// NewACPTrust cria o bind vazio; AttachACPTrust preenche deps no startup.
func NewACPTrust() *ACPTrust {
	return &ACPTrust{}
}

// AttachACPTrust associa Session, Store e mapa de nomes de perfil após o startup.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachACPTrust(api *ACPTrust, session Session, trust *acptrust.Store, profileNames func() map[string]string) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.trust = trust
	api.profileNames = profileNames
}

func (api *ACPTrust) deps() (Session, *acptrust.Store, func() map[string]string, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.trust == nil {
		return nil, nil, nil, ErrACPTrustNotWired
	}
	return api.session, api.trust, api.profileNames, nil
}

// GetAgentPermissions lista o que os perfis autorizaram aos agentes de código
// sem perguntar de novo.
//
// Lista todos os perfis, e não só o ativo: uma autorização esquecida num perfil
// que não se usa há meses volta a valer no dia em que ele for aberto, e quem
// revoga precisa enxergá-la antes disso.
func (api *ACPTrust) GetAgentPermissions() ([]apidto.AgentPermissionView, error) {
	session, trust, profileNames, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]apidto.AgentPermissionView, error) {
		_ = ctx
		var names map[string]string
		if profileNames != nil {
			names = profileNames()
		}
		// Lista vazia é lista, não ausência: um slice nulo chegaria à interface
		// como null, e o tipo gerado promete um array.
		out := make([]apidto.AgentPermissionView, 0)
		for _, slug := range trust.Profiles() {
			for _, entry := range trust.List(slug) {
				out = append(out, apidto.AgentPermissionView{
					ProfileSlug: slug,
					ProfileName: names[slug],
					// A classe vai como está guardada, e não pelo conjunto que o
					// app conhece hoje: o que ele não reconhecesse viraria "other"
					// na tela e na revogação, que então não casaria com a entrada
					// do arquivo — a linha ficaria lá, impossível de tirar.
					Action:    strings.ToLower(strings.TrimSpace(entry.Kind)),
					GrantedAt: entry.GrantedAt.Format(time.RFC3339),
				})
			}
		}
		// Ordem estável: a lista é lida em sequência por leitor de telas, e uma
		// ordem que muda a cada carregamento faria a pessoa procurar de novo o que
		// acabou de encontrar.
		sort.Slice(out, func(i, j int) bool {
			if out[i].ProfileSlug != out[j].ProfileSlug {
				return out[i].ProfileSlug < out[j].ProfileSlug
			}
			return out[i].Action < out[j].Action
		})
		return out, nil
	})
}

// RevokeAgentPermission tira a autorização permanente: o agente volta a
// perguntar antes de agir daquela classe naquele perfil.
func (api *ACPTrust) RevokeAgentPermission(profileSlug, action string) error {
	session, trust, _, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		_ = ctx
		if err := trust.Revoke(profileSlug, action); err != nil {
			if errors.Is(err, acptrust.ErrEntryNotFound) {
				// Dizer "revogado" sem ter revogado nada faria a pessoa acreditar
				// que fechou uma porta que continua aberta.
				return struct{}{}, fmt.Errorf("essa autorização não existe mais")
			}
			return struct{}{}, err
		}
		return struct{}{}, nil
	})
	return err
}
