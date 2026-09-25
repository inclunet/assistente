package commandconfig

// validatePreviewActiveUserLayerIDs valida e canoniza a prova dinâmica da
// porta confiável sem inferir o conjunto efetivo a partir de claims. D8 usa
// união aditiva: uma claim manual expirada não invalida uma ativação
// contextual independente. Nenhum efeito de expiração, rebind ou reconcile é
// executado aqui; isso permanece no serviço de ativação e no hook transacional.
func validatePreviewActiveUserLayerIDs(snapshot Snapshot, ids []string) ([]string, error) {
	canonical, err := canonicalActiveUserLayerIDs(ids)
	if err != nil {
		return nil, err
	}
	layers := make(map[string]Layer, len(snapshot.Layers))
	for _, layer := range snapshot.Layers {
		if layer.UserID != snapshot.Scope.UserID || !inScope(layer.WorkspaceID, snapshot.Scope) {
			return nil, ErrInvalid
		}
		if _, exists := layers[layer.ID]; exists {
			return nil, ErrInvalid
		}
		layers[layer.ID] = layer
	}

	result := make([]string, 0, len(canonical))
	for _, id := range canonical {
		if layer, exists := layers[id]; !exists || !layer.Enabled {
			return nil, ErrInvalid
		}
		result = append(result, id)
	}
	return result, nil
}
