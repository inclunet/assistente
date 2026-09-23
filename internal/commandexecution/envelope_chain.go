package commandexecution

import (
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandjson"
	"encoding/json"
	"strings"
)

// CommandMaxChainDepth é o limite versionado do protocolo de comandos v1.
// Não altera _chain_history, cuja interpretação pertence ao runtime de jobs.
const CommandMaxChainDepth = commandcontract.CommandChainMaxDepth

type commandChainEntry = commandcontract.CommandChainEntry

func prepareCommandChain(e commandcontract.Envelope, definition commandcatalog.Definition, layerRefs []string) (commandcontract.Envelope, error) {
	if e.Provenance == nil {
		// Evento/job capaz de ampliar a cadeia precisa ter raiz comprovada.
		if (e.AuthContextType == commandcontract.AuthJobService || (e.SourceType != nil && *e.SourceType == commandcontract.SourceType(commandcatalog.Event))) && (definition.Effect != commandcatalog.Read || definition.MutatesEffectiveCapability) {
			return e, ErrDenied
		}
		if definition.HandlerClassification != commandcatalog.HandlerJob && definition.HandlerClassification != commandcatalog.HandlerTool {
			return e, nil
		}
		// Uma delegação direta inaugura a cadeia antes da reserva. O handler
		// de jobs não deve inventar uma entrada depois da autorização/ledger.
		if e.AuthContextType != commandcontract.AuthLocalSession || e.ActorType != commandcontract.ActorUser || !validID(e.InvocationID) {
			return e, ErrDenied
		}
		seed, err := commandjson.Marshal(map[string]any{"version": 1, "_chain_id": e.InvocationID, "_chain_history": []string{}, "command_chain_history": []commandcontract.CommandChainEntry{}})
		if err != nil {
			return e, ErrDenied
		}
		provenance := json.RawMessage(seed)
		e.Provenance = &provenance
	}
	var doc map[string]json.RawMessage
	canonical, err := commandjson.Canonicalize(*e.Provenance)
	if err != nil || json.Unmarshal(canonical, &doc) != nil || doc == nil {
		return e, ErrDenied
	}
	raw, present := doc["command_chain_history"]
	reactive := present || doc["_chain_id"] != nil || e.AuthContextType == commandcontract.AuthJobService || (e.SourceType != nil && *e.SourceType == commandcontract.SourceType(commandcatalog.Event))
	if !reactive {
		return e, nil
	}
	if definition.Effect != commandcatalog.Read || definition.MutatesEffectiveCapability {
		var chainID string
		var jobs []json.RawMessage
		if json.Unmarshal(doc["_chain_id"], &chainID) != nil || strings.TrimSpace(chainID) == "" || json.Unmarshal(doc["_chain_history"], &jobs) != nil || jobs == nil {
			return e, ErrDenied
		}
	}
	var history []commandcontract.CommandChainEntry
	if present {
		var decodeErr error
		history, decodeErr = commandcontract.DecodeCommandChainHistory(raw)
		if decodeErr != nil {
			return e, ErrDenied
		}
	}
	if len(history) >= CommandMaxChainDepth || e.CommandID == nil || *e.CommandID != definition.ID || !validID(e.InvocationID) {
		return e, ErrDenied
	}
	seen := map[string]bool{}
	invocations := map[string]bool{}
	for _, entry := range history {
		seen[entry.CommandID] = true
		invocations[entry.InvocationID] = true
	}
	entry := commandcontract.CommandChainEntry{CommandID: definition.ID, InvocationID: e.InvocationID, LayerRefs: append([]string{}, layerRefs...)}
	if seen[definition.ID] || invocations[e.InvocationID] || entry.Validate() != nil || (e.TriggerType != nil && len(layerRefs) == 0) {
		return e, ErrDenied
	}
	history = append(history, entry)
	encoded, err := commandjson.Marshal(history)
	if err != nil {
		return e, ErrDenied
	}
	doc["command_chain_history"] = encoded
	encoded, err = commandjson.Marshal(doc)
	if err != nil {
		return e, ErrDenied
	}
	value := json.RawMessage(encoded)
	e.Provenance = &value
	return e, nil
}
