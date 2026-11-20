package slashskill

import (
	"context"
	"strings"

	"assistente/internal/contextprovider"
)

const defaultPromptBudget = 50000
const slashSkillTruncationNotice = "... Additional slash skill content omitted due to context budget."

type ContextProvider struct{}

func NewContextProvider() *ContextProvider {
	return &ContextProvider{}
}

func (p *ContextProvider) Name() string { return "slash_skill" }

func (p *ContextProvider) Metadata() contextprovider.ProviderMetadata {
	return contextprovider.ProviderMetadata{
		Name:             p.Name(),
		DisplayName:      "Slash Skill",
		Description:      "Instructions for the skill explicitly invoked in the current turn.",
		DefaultEnabled:   true,
		DefaultBudget:    defaultPromptBudget,
		SupportsSettings: false,
	}
}

func (p *ContextProvider) Build(_ context.Context, req contextprovider.BuildRequest) ([]contextprovider.Block, error) {
	content := strings.TrimSpace(req.SlashSkillContent)
	if content == "" {
		return nil, nil
	}
	if tag := outerTag(content); tag != "" {
		content = contextprovider.TrimTaggedBlockToBudget(content, tag, slashSkillTruncationNotice, req.Budget(p.Name(), defaultPromptBudget))
	} else if contextprovider.RuneLen(content) > req.Budget(p.Name(), defaultPromptBudget) {
		return nil, nil
	}
	if content == "" {
		return nil, nil
	}
	return []contextprovider.Block{{
		Provider:   p.Name(),
		Name:       "slash_skill",
		Volatility: contextprovider.VolatilityTurnDynamic,
		Priority:   200,
		Content:    content,
	}}, nil
}

func outerTag(content string) string {
	if !strings.HasPrefix(content, "<") {
		return ""
	}
	end := strings.Index(content, ">")
	if end <= 1 {
		return ""
	}
	tag := content[1:end]
	if strings.ContainsAny(tag, " \t\r\n/") {
		return ""
	}
	if !strings.HasSuffix(content, "</"+tag+">") {
		return ""
	}
	return tag
}
