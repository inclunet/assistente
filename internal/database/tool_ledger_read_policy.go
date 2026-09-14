package database

import (
	"context"
	"strings"
)

const toolLedgerConversationResource = "conversation"

// ToolLedgerLegacyReadPolicy informa quais conversas ainda dependem de L1/L3.
type ToolLedgerLegacyReadPolicy map[string]bool

func (policy ToolLedgerLegacyReadPolicy) Allows(conversationID string) bool {
	return policy[strings.TrimSpace(conversationID)]
}

// LoadToolLedgerLegacyReadPolicyWithUser resolve o gate em lote. Ausência de
// checkpoint significa ledger-only; apenas pending autoriza leitura legada.
func LoadToolLedgerLegacyReadPolicyWithUser(ctx context.Context, userID string, conversationIDs []string) (ToolLedgerLegacyReadPolicy, error) {
	policy := ToolLedgerLegacyReadPolicy{}
	ids := uniqueToolLedgerConversationIDs(conversationIDs)
	if len(ids) == 0 {
		return policy, nil
	}
	if db == nil || !db.Migrator().HasTable(&ToolLedgerMigrationState{}) {
		// Compatibilidade somente com schemas pré-v18 de upgrade/testes.
		for _, id := range ids {
			policy[id] = true
		}
		return policy, nil
	}
	var pendingIDs []string
	if err := db.WithContext(ctx).
		Model(&ToolLedgerMigrationState{}).
		Where(
			"user_id = ? AND resource_type = ? AND state = ? AND resource_id IN ?",
			strings.TrimSpace(userID),
			toolLedgerConversationResource,
			"pending",
			ids,
		).
		Pluck("resource_id", &pendingIDs).Error; err != nil {
		return nil, err
	}
	for _, id := range pendingIDs {
		policy[id] = true
	}
	return policy, nil
}

func uniqueToolLedgerConversationIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
