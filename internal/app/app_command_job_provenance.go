package app

import (
	"bytes"
	"encoding/json"
	"strings"

	"assistente/internal/commandbindings"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjson"
)

// Somente a proveniência das camadas selecionadas entra no envelope. A
// identidade privada do runtime nunca sai da projeção para a auditoria do
// comando. Não escolhemos uma raiz arbitrária quando ciclos divergem.
func commandSelectedJobProvenance(sources []commandbindings.LayerProvenance) (*json.RawMessage, error) {
	var selected json.RawMessage
	for _, source := range sources {
		canonical, err := commandjson.Canonicalize(source.Provenance)
		if err != nil {
			return nil, commandexecution.ErrDenied
		}
		var document map[string]json.RawMessage
		if json.Unmarshal(canonical, &document) != nil || document == nil {
			return nil, commandexecution.ErrDenied
		}
		var chainID string
		var jobs []string
		if json.Unmarshal(document["_chain_id"], &chainID) != nil || strings.TrimSpace(chainID) == "" ||
			strings.TrimSpace(chainID) != chainID || strings.ContainsRune(chainID, '\x00') ||
			json.Unmarshal(document["_chain_history"], &jobs) != nil || jobs == nil {
			return nil, commandexecution.ErrDenied
		}
		for _, job := range jobs {
			if strings.TrimSpace(job) == "" || strings.TrimSpace(job) != job || strings.ContainsRune(job, '\x00') {
				return nil, commandexecution.ErrDenied
			}
		}
		chain := map[string]json.RawMessage{
			"version":   json.RawMessage(`1`),
			"_chain_id": document["_chain_id"], "_chain_history": document["_chain_history"],
		}
		for _, field := range []string{"_source", "_source_job_id"} {
			if raw, exists := document[field]; exists {
				var value string
				if json.Unmarshal(raw, &value) != nil || value == "" || strings.TrimSpace(value) != value || strings.ContainsRune(value, '\x00') {
					return nil, commandexecution.ErrDenied
				}
				chain[field] = raw
			}
		}
		if history, exists := document["command_chain_history"]; exists {
			// O executor é dono da validação e do limite 16, antes da reserva.
			// Inclusive null/malformado é preservado para ser recusado, não apagado.
			chain["command_chain_history"] = history
		}
		raw, err := commandjson.Marshal(chain)
		if err != nil || selected != nil && !bytes.Equal(selected, raw) {
			return nil, commandexecution.ErrDenied
		}
		selected = raw
	}
	if selected == nil {
		return nil, nil
	}
	return &selected, nil
}
