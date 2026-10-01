package controllers

import (
	"assistente/internal/oauthflow"
	"context"
)

func (c *LLMController) CreateChatGPTConnection(ctx context.Context, name string) (oauthflow.Summary, error) {
	return c.providerSvc.CreateChatGPTConnection(ctx, name)
}
func (c *LLMController) ChatGPTConnection(ctx context.Context, id string) (oauthflow.Summary, error) {
	return c.providerSvc.ChatGPTConnection(ctx, id)
}
func (c *LLMController) AuthorizeChatGPT(ctx context.Context, id, completionText string) (oauthflow.Summary, error) {
	return c.providerSvc.AuthorizeChatGPT(ctx, id, completionText)
}
func (c *LLMController) DisconnectChatGPT(ctx context.Context, id string) (bool, error) {
	return c.providerSvc.DisconnectChatGPT(ctx, id)
}
func (c *LLMController) CancelChatGPT(ctx context.Context, id string) error {
	return c.providerSvc.CancelChatGPT(ctx, id)
}
