package commandconfig

type ActionClass string

const (
	ReadAction         ActionClass = "read"
	CapabilityMutation ActionClass = "capability_mutation"
)

// ClassifyAction fecha o vocabulário de command_config. Classificar não
// anuncia implementação: import/export exigem também readiness do adapter I11.
func ClassifyAction(action string) (ActionClass, error) {
	switch action {
	case "layer_list", "layer_get", "binding_list", "binding_check_conflict", "config_export":
		return ReadAction, nil
	case "layer_create", "layer_update", "layer_delete", "layer_enable", "layer_disable", "layer_restore", "binding_create", "binding_update", "binding_delete", "binding_enable", "binding_disable", "binding_restore", "config_restore", "config_import":
		return CapabilityMutation, nil
	default:
		return "", ErrInvalid
	}
}
