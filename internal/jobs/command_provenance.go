package jobs

import (
	"errors"

	"assistente/internal/commandcontract"
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
	chain, err := commandcontract.DecodeCommandChainHistory(raw)
	if err != nil {
		return nil, errors.New("invalid command chain provenance")
	}
	result["command_chain_history"] = chain
	return result, nil
}
