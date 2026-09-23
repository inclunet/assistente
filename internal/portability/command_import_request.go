package portability

import (
	"bytes"
	"encoding/json"
	"strings"

	"assistente/internal/commandportability"
)

const maxCommandImportEntries = 64

const maxCommandImportResolutions = maxCommandImportEntries*2 + 1

// ParseCommandImportEnvelope expõe o parser estrito do envelope dedicado sem
// duplicar o formato nem o writer de commandportability. O parser rejeita
// recursos misturados, campos desconhecidos, duplicatas e documentos fora do
// limite do commandjson.
func ParseCommandImportEnvelope(raw []byte) ([]commandportability.LayerExport, error) {
	file, err := parseCommandLayersEnvelope(raw)
	if err != nil {
		return nil, err
	}
	return append([]commandportability.LayerExport(nil), file.Resources.CommandLayers...), nil
}

// HasCommandLayers é somente um discriminador de roteamento. A validação
// segura, inclusive de duplicatas, acontece em ParseCommandImportEnvelope e
// no writer; portanto esta função não deve ser usada para aceitar o envelope.
func HasCommandLayers(jsonData string) bool {
	decoder := json.NewDecoder(strings.NewReader(jsonData))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return false
	}
	found := false
	for decoder.More() {
		key, ok := nextJSONString(decoder)
		if !ok {
			return false
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return false
		}
		if strings.EqualFold(key, "resources") && rawResourcesHaveCommandLayers(raw) {
			found = true
			break
		}
	}
	if found {
		return true
	}
	end, err := decoder.Token()
	return err == nil && end == json.Delim('}') && found
}

func nextJSONString(decoder *json.Decoder) (string, bool) {
	token, err := decoder.Token()
	if err != nil {
		return "", false
	}
	value, ok := token.(string)
	return value, ok
}

func rawResourcesHaveCommandLayers(raw []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return false
	}
	found := false
	for decoder.More() {
		key, ok := nextJSONString(decoder)
		if !ok {
			return false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return false
		}
		if strings.EqualFold(key, "commandLayers") {
			found = true
			break
		}
	}
	if found {
		return true
	}
	end, err := decoder.Token()
	return err == nil && end == json.Delim('}') && found
}

// CommandImportOptions converte as resoluções públicas já existentes no
// ImportRequest para as opções do planejador confiável. A conversão é pura:
// não consulta posse, nomes ou workspaces do destino.
func CommandImportOptions(req ImportRequest) (commandportability.PlanOptions, error) {
	if req.CredentialExportPassword != "" || len(req.Resolutions) > maxCommandImportResolutions {
		return commandportability.PlanOptions{}, commandportability.ErrInvalid
	}
	layers, err := ParseCommandImportEnvelope([]byte(req.JSONData))
	if err != nil {
		return commandportability.PlanOptions{}, err
	}
	if len(layers) == 0 || len(layers) > maxCommandImportEntries {
		return commandportability.PlanOptions{}, commandportability.ErrInvalid
	}

	layerIDs := make(map[string]struct{}, len(layers))
	workspaceIDs := make(map[string]struct{})
	for _, layer := range layers {
		if layer.Scope.Kind == commandportability.WorkspaceScope {
			workspaceIDs[layer.Scope.WorkspaceID] = struct{}{}
		}
		if layer.DeltaOnly {
			continue
		}
		if _, duplicate := layerIDs[layer.ID]; duplicate {
			return commandportability.PlanOptions{}, commandportability.ErrInvalid
		}
		layerIDs[layer.ID] = struct{}{}
	}

	options := commandportability.PlanOptions{
		WorkspaceMap:  make(map[string]string),
		RenameByLayer: make(map[string]string),
	}
	seen := make(map[string]struct{}, len(req.Resolutions))
	commandLayersDecisions := 0
	for _, resolution := range req.Resolutions {
		key := resolution.ResourceType + "\x00" + resolution.Identifier
		if _, duplicate := seen[key]; duplicate {
			return commandportability.PlanOptions{}, commandportability.ErrInvalid
		}
		seen[key] = struct{}{}

		switch resolution.ResourceType {
		case "commandLayers":
			if resolution.Identifier != "*" || resolution.RenameValue != "" || commandLayersDecisions != 0 {
				return commandportability.PlanOptions{}, commandportability.ErrInvalid
			}
			commandLayersDecisions++
			switch resolution.Strategy {
			case ConflictResolutionSkip:
				options.Mode = commandportability.KeepMode
			case ConflictResolutionOverwrite:
				options.Mode = commandportability.ReplaceMode
			case ConflictResolutionRename:
				options.Mode = commandportability.CopyMode
			default:
				return commandportability.PlanOptions{}, commandportability.ErrInvalid
			}

		case "commandLayerName":
			if resolution.Strategy != ConflictResolutionRename || resolution.RenameValue == "" || strings.TrimSpace(resolution.RenameValue) != resolution.RenameValue || strings.ContainsRune(resolution.RenameValue, '\x00') {
				return commandportability.PlanOptions{}, commandportability.ErrInvalid
			}
			if _, exists := layerIDs[resolution.Identifier]; !exists || len(options.RenameByLayer) >= maxCommandImportEntries {
				return commandportability.PlanOptions{}, commandportability.ErrInvalid
			}
			options.RenameByLayer[resolution.Identifier] = resolution.RenameValue

		case "commandWorkspace":
			if resolution.Strategy != ConflictResolutionRename || resolution.RenameValue == "" || strings.TrimSpace(resolution.RenameValue) != resolution.RenameValue || strings.ContainsRune(resolution.RenameValue, '\x00') {
				return commandportability.PlanOptions{}, commandportability.ErrInvalid
			}
			if _, exists := workspaceIDs[resolution.Identifier]; !exists || len(options.WorkspaceMap) >= maxCommandImportEntries {
				return commandportability.PlanOptions{}, commandportability.ErrInvalid
			}
			options.WorkspaceMap[resolution.Identifier] = resolution.RenameValue

		default:
			return commandportability.PlanOptions{}, commandportability.ErrInvalid
		}
	}
	if commandLayersDecisions != 1 {
		return commandportability.PlanOptions{}, commandportability.ErrInvalid
	}
	return options, nil
}
