package wailsapi

import (
	"assistente/internal/oauthflow"
	"context"
)

func (p *LLMProviders) CreateChatGPTConnection(name string) (oauthflow.Summary, error) {
	session, ctrl, _, err := p.deps()
	if err != nil {
		return oauthflow.Summary{}, err
	}
	return WithUser(session, func(ctx context.Context) (oauthflow.Summary, error) { return ctrl.CreateChatGPTConnection(ctx, name) })
}
func (p *LLMProviders) ChatGPTConnection(id string) (oauthflow.Summary, error) {
	session, ctrl, _, err := p.deps()
	if err != nil {
		return oauthflow.Summary{}, err
	}
	return WithUser(session, func(ctx context.Context) (oauthflow.Summary, error) { return ctrl.ChatGPTConnection(ctx, id) })
}
func (p *LLMProviders) AuthorizeChatGPT(id, completionText string) (oauthflow.Summary, error) {
	session, ctrl, _, err := p.deps()
	if err != nil {
		return oauthflow.Summary{}, err
	}
	return WithUser(session, func(ctx context.Context) (oauthflow.Summary, error) {
		return ctrl.AuthorizeChatGPT(ctx, id, completionText)
	})
}
func (p *LLMProviders) DisconnectChatGPT(id string) (bool, error) {
	session, ctrl, _, err := p.deps()
	if err != nil {
		return false, err
	}
	return WithUser(session, func(ctx context.Context) (bool, error) { return ctrl.DisconnectChatGPT(ctx, id) })
}
func (p *LLMProviders) CancelChatGPT(id string) error {
	session, ctrl, _, err := p.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) { return struct{}{}, ctrl.CancelChatGPT(ctx, id) })
	return err
}
