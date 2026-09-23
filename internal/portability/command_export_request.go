package portability

import (
	"assistente/internal/commandportability"
	"github.com/google/uuid"
)

const maxCommandLayerExportIDs = 64

// ValidateCommandExportRequest valida a entrada da rota dedicada de
// resources.commandLayers. A resolução de posse e de acesso a workspaces é
// responsabilidade do host autenticado que executa o export.
func ValidateCommandExportRequest(req ExportRequest) error {
	if req.OutputFormat != "" && req.OutputFormat != FormatJSON {
		return commandportability.ErrUnsupported
	}
	if !req.IncludeCommandLayers {
		return commandportability.ErrInvalid
	}
	if !req.ExplicitSelection {
		return commandportability.ErrInvalid
	}
	if req.All {
		return commandportability.ErrUnsupported
	}

	if hasCommandExportOtherResourceSelection(req) || req.IncludeContacts || req.IncludeAudio ||
		req.IncludeCredentials || req.CredentialExportPassword != "" ||
		req.IncludeTimestamps != nil || req.IncludeReasoning != nil || req.IncludeMetadata != nil {
		return commandportability.ErrUnsupported
	}

	if len(req.CommandLayerIDs) > maxCommandLayerExportIDs {
		return commandportability.ErrInvalid
	}
	seen := make(map[string]struct{}, len(req.CommandLayerIDs))
	for _, id := range req.CommandLayerIDs {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed.Version() != uuid.Version(7) || parsed.Variant() != uuid.RFC4122 || parsed.String() != id {
			return commandportability.ErrInvalid
		}
		if _, ok := seen[id]; ok {
			return commandportability.ErrInvalid
		}
		seen[id] = struct{}{}
	}
	return nil
}

func hasCommandExportOtherResourceSelection(req ExportRequest) bool {
	return req.All || len(req.ConversationIDs) != 0 || len(req.ProviderIDs) != 0 ||
		len(req.ProfileSlugs) != 0 || len(req.SkillSlugs) != 0 || len(req.AllowlistSlugs) != 0 ||
		len(req.MCPServerSlugs) != 0 || len(req.JobIDs) != 0 || len(req.TaskListIDs) != 0 ||
		len(req.MemoryRecordIDs) != 0 || len(req.ChannelNames) != 0
}
