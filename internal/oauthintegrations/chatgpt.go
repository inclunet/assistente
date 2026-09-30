// Package oauthintegrations declares service-specific OAuth extensions.
package oauthintegrations

import (
	"assistente/internal/oauthflow"
	"net/url"
)

func ChatGPT() oauthflow.Integration {
	return oauthflow.Integration{
		Routes: []oauthflow.Route{{Method: "GET", Path: "/models"}, {Method: "POST", Path: "/responses"}},
		ID:     "chatgpt", Issuer: "https://auth.openai.com", Resource: "https://api.openai.com/v1",
		Endpoints:       oauthflow.Endpoints{Authorization: "https://auth.openai.com/api/accounts/authorize", Token: "https://auth.openai.com/api/accounts/oauth/token", Revocation: "https://auth.openai.com/api/accounts/oauth/revoke", JWKS: "https://auth.openai.com/.well-known/jwks.json"},
		Scopes:          []string{"openid", "profile", "email", "offline_access", "resource.invoke", "chatgpt.tokens.use.direct"},
		RequiredScopes:  []string{"openid", "resource.invoke", "chatgpt.tokens.use.direct"},
		Callback:        oauthflow.CallbackConfig{Host: "127.0.0.1", Path: "/auth/callback", PortPolicy: "ephemeral"},
		InitialClientID: "dynamic_agent_client", RegistrationMethod: "extension",
		AuthorizationParameters: func(r oauthflow.Record, hostID string) url.Values {
			v := url.Values{"ext_agent_host_id": {hostID}}
			if r.Client.ID == "" {
				v.Set("agent_name_hint", "Assistente")
			} else if r.Tokens.ID != "" {
				v.Set("id_token_hint", r.Tokens.ID)
				if r.Email != "" {
					v.Set("login_hint", r.Email)
				}
			}
			return v
		},
		CallbackClientID: func(r oauthflow.Record, v url.Values) (string, error) {
			id := v.Get("client_id")
			if r.Client.ID != "" {
				if id != "" && id != r.Client.ID {
					return "", oauthflow.ErrConflict
				}
				return r.Client.ID, nil
			}
			if id == "" || id == "dynamic_agent_client" {
				return "", oauthflow.ErrReauthorize
			}
			return id, nil
		},
	}
}
