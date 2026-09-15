package jobs

import (
	"encoding/json"
	"errors"
	"strings"

	"assistente/internal/commandjson"
)

// commandRunProvenance persiste apenas a origem estrutural do runtime, nunca
// o payload arbitrário do trigger. Somente jobs acrescentam seu slug à cadeia
// de jobs; a cadeia de comandos é preservada separadamente, sem ampliação.
func (e *JobExecutor) commandRunProvenance(job *Job, trigger *TriggerContext, run *RunLog) (map[string]any, error) {
	p := e.runProvenance(job, trigger, run)
	result := map[string]any{"_source": p.Source, "_source_job_id": p.SourceJobID, "_chain_id": p.ChainID, "_chain_history": append([]string{}, p.ChainHistory...)}
	if trigger == nil || trigger.Provenance == nil {
		return result, nil
	}
	value, ok := trigger.Provenance["command_chain_history"]
	if !ok {
		return result, nil
	}
	raw, err := commandjson.Marshal(value)
	if err != nil {
		return nil, err
	}
	var chain []struct {
		CommandID    string   `json:"command_id"`
		InvocationID string   `json:"invocation_id"`
		LayerRefs    []string `json:"layer_refs"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&chain); err != nil || chain == nil || len(chain) > 16 {
		return nil, errors.New("invalid command chain provenance")
	}
	seen := map[string]bool{}
	for _, item := range chain {
		if item.CommandID == "" || strings.TrimSpace(item.CommandID) != item.CommandID || strings.ContainsRune(item.CommandID, '\x00') || !isUUIDv7(item.InvocationID) || item.LayerRefs == nil || seen[item.CommandID] {
			return nil, errors.New("invalid command chain provenance")
		}
		for _, ref := range item.LayerRefs {
			if strings.TrimSpace(ref) != ref || ref == "" || strings.ContainsRune(ref, '\x00') {
				return nil, errors.New("invalid command chain provenance")
			}
		}
		seen[item.CommandID] = true
	}
	result["command_chain_history"] = chain
	return result, nil
}
