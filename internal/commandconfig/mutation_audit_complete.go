package commandconfig

import (
	"gorm.io/gorm"
	"time"
)

func recordCompleteMutation(tx *gorm.DB, c *ConfirmedMutation, g Generation) error {
	p, r := c.prepared, c.request
	before, after, err := mutationDocuments(p)
	if err != nil {
		return err
	}
	scope := "global"
	if p.before.Scope.WorkspaceID != nil {
		scope = "workspace"
	}
	// A montagem completa exige o projetor antes do preview/commit e recusa
	// presença em paths sensíveis até I11 resolver referências ao cofre.
	// O texto do diálogo e tokens não entram nestes documentos.
	return tx.Table("command_config_mutations").Create(map[string]any{
		"mutation_id": p.diff.MutationID, "schema_version": 2, "user_id": r.UserID, "session_id": r.SessionID,
		"scope": scope, "workspace_id": p.before.Scope.WorkspaceID, "operation": string(p.diff.Operation), "binding_id": nil,
		"decision_id": r.DecisionID, "request_fingerprint": r.Fingerprint, "auth_generation": r.AuthGeneration, "security_generation": r.SecurityGeneration,
		"generation_id": g.ID, "before_generation": g.Generation, "after_generation": g.Generation + 1,
		"before_enabled": nil, "after_enabled": nil, "before_document": before, "after_document": after, "occurred_at": time.Now().UTC(),
	}).Error
}
