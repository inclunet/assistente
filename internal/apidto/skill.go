package apidto

import "assistente/internal/skills"

// SkillCreateRequest é o payload da borda Wails para criar/atualizar skill (AEP-0088).
type SkillCreateRequest struct {
	skills.SkillMetadata `json:",inline"`
	Content              string `json:"content"`
}
