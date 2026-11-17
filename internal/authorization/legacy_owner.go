package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"assistente/internal/configdir"
	"assistente/internal/database"
)

// LocalLegacyOwner admite adoção automática somente na instalação local com
// um único usuário identificado. Origens multiusuário exigem atribuição explícita.
func LocalLegacyOwner(ctx context.Context) bool {
	user, err := database.RequireUserID(ctx)
	if err != nil || database.DB() == nil {
		return false
	}
	data, _, err := configdir.NewResolver("").Read("auth.json")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err == nil {
		var cfg struct {
			Mode string `json:"mode"`
		}
		if json.Unmarshal(data, &cfg) != nil || (cfg.Mode != "" && cfg.Mode != "local") {
			return false
		}
	}
	// SECURITY: consulta instance-wide apenas para provar dono único; não
	// devolve dados de outros usuários nem adota para o primeiro login.
	var users []string
	if err := database.DB().WithContext(ctx).Model(&database.User{}).Limit(2).Pluck("id", &users).Error; err != nil {
		return false
	}
	return len(users) == 1 && users[0] == user
}
