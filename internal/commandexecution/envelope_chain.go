package commandexecution

import (
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandjson"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// CommandMaxChainDepth é o limite versionado do protocolo de comandos v1.
// Não altera _chain_history, cuja interpretação pertence ao runtime de jobs.
const CommandMaxChainDepth = 16

type commandChainEntry struct {
	CommandID    string   `json:"command_id"`
	InvocationID string   `json:"invocation_id"`
	LayerRefs    []string `json:"layer_refs"`
}

func prepareCommandChain(e commandcontract.Envelope, definition commandcatalog.Definition, layerRefs []string) (commandcontract.Envelope, error) {
	if e.Provenance == nil {
		// Evento/job capaz de ampliar a cadeia precisa ter raiz comprovada.
		if (e.AuthContextType == commandcontract.AuthJobService || (e.SourceType != nil && *e.SourceType == commandcontract.SourceType(commandcatalog.Event))) && (definition.Effect != commandcatalog.Read || definition.MutatesEffectiveCapability) {
			return e, ErrDenied
		}
		return e, nil
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
	var history []commandChainEntry
	if present {
		if len(raw) == 0 || raw[0] != '[' {
			return e, ErrDenied
		}
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&history) != nil {
			return e, ErrDenied
		}
	}
	if len(history) >= CommandMaxChainDepth || e.CommandID == nil || *e.CommandID != definition.ID || !validID(e.InvocationID) {
		return e, ErrDenied
	}
	seen := map[string]bool{}
	invocations := map[string]bool{}
	for _, entry := range history {
		if !commandIDPattern.MatchString(entry.CommandID) || !validID(entry.InvocationID) || entry.LayerRefs == nil || !validChainLayers(entry.LayerRefs) || seen[entry.CommandID] || invocations[entry.InvocationID] {
			return e, ErrDenied
		}
		seen[entry.CommandID] = true
		invocations[entry.InvocationID] = true
	}
	if seen[definition.ID] || invocations[e.InvocationID] || !validChainLayers(layerRefs) || (e.TriggerType != nil && len(layerRefs) == 0) {
		return e, ErrDenied
	}
	history = append(history, commandChainEntry{CommandID: definition.ID, InvocationID: e.InvocationID, LayerRefs: append([]string{}, layerRefs...)})
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

func validChainLayers(refs []string) bool {
	if len(refs) > 256 {
		return false
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		if ref == "" || len(ref) > 256 || !utf8.ValidString(ref) || strings.TrimSpace(ref) != ref || strings.ContainsRune(ref, '\x00') || seen[ref] {
			return false
		}
		seen[ref] = true
	}
	return true
}
