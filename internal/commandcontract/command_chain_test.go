package commandcontract

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func commandChainTestUUID(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func TestDecodeCommandChainHistoryValidatesProtocolBoundary(t *testing.T) {
	history := make([]CommandChainEntry, CommandChainMaxDepth)
	for i := range history {
		history[i] = CommandChainEntry{CommandID: "job.step" + string(rune('a'+i)), InvocationID: commandChainTestUUID(t), LayerRefs: []string{"layer.a"}}
	}
	raw, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeCommandChainHistory(raw); err != nil || len(got) != CommandChainMaxDepth {
		t.Fatalf("terminal history rejected: len=%d err=%v", len(got), err)
	}
	history = append(history, CommandChainEntry{CommandID: "job.overflow", InvocationID: commandChainTestUUID(t), LayerRefs: []string{"layer.a"}})
	raw, _ = json.Marshal(history)
	if _, err := DecodeCommandChainHistory(raw); err == nil {
		t.Fatal("seventeenth entry accepted")
	}
}

func TestDecodeCommandChainHistoryRejectsStructuralDivergence(t *testing.T) {
	cases := map[string][]byte{
		"unnamespaced command": []byte(`[{"command_id":"step","invocation_id":"00000000-0000-7000-8000-000000000001","layer_refs":["layer.a"]}]`),
		"bad uuid":             []byte(`[{"command_id":"job.step","invocation_id":"bad","layer_refs":["layer.a"]}]`),
		"duplicate layer":      []byte(`[{"command_id":"job.step","invocation_id":"00000000-0000-7000-8000-000000000001","layer_refs":["layer.a","layer.a"]}]`),
		"unknown field":        []byte(`[{"command_id":"job.step","invocation_id":"00000000-0000-7000-8000-000000000001","layer_refs":["layer.a"],"extra":true}]`),
		"duplicate json key":   []byte(`[{"command_id":"job.step","command_id":"job.other","invocation_id":"00000000-0000-7000-8000-000000000001","layer_refs":["layer.a"]}]`),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeCommandChainHistory(raw); err == nil {
				t.Fatal("invalid history accepted")
			}
		})
	}
	if _, err := DecodeCommandChainHistory([]byte{'[', '{', '"', 'x', '"', ':', 0xff, '}', ']'}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

func TestCommandChainRejectsDuplicateCommandsAndInvocations(t *testing.T) {
	for _, duplicateCommand := range []bool{false, true} {
		t.Run(fmt.Sprint(duplicateCommand), func(t *testing.T) {
			first := CommandChainEntry{CommandID: "job.first", InvocationID: commandChainTestUUID(t), LayerRefs: []string{}}
			second := CommandChainEntry{CommandID: "job.second", InvocationID: commandChainTestUUID(t), LayerRefs: []string{}}
			if duplicateCommand {
				second.CommandID = first.CommandID
			} else {
				second.InvocationID = first.InvocationID
			}
			raw, err := json.Marshal([]CommandChainEntry{first, second})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecodeCommandChainHistory(raw); err == nil {
				t.Fatal("histórico com identidade duplicada aceito")
			}
		})
	}
}

func TestCommandChainEntryRejectsInvalidLayerReferences(t *testing.T) {
	tooMany := make([]string, 257)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("layer.%d", i)
	}
	for name, refs := range map[string][]string{
		"ausente": nil, "vazia": {""}, "espaço": {" layer.a"},
		"NUL": {"layer.\x00a"}, "UTF8": {string([]byte{0xff})},
		"longa": {strings.Repeat("a", 257)}, "quantidade": tooMany,
	} {
		t.Run(name, func(t *testing.T) {
			entry := CommandChainEntry{CommandID: "job.first", InvocationID: commandChainTestUUID(t), LayerRefs: refs}
			if entry.Validate() == nil {
				t.Fatal("referência inválida aceita")
			}
		})
	}
	valid := `[{"command_id":"job.first","invocation_id":"00000000-0000-7000-8000-000000000001","layer_refs":["PLACEHOLDER"]}]`
	invalidUTF8 := []byte(strings.Replace(valid, "PLACEHOLDER", string([]byte{0xff}), 1))
	if _, err := DecodeCommandChainHistory(invalidUTF8); err == nil {
		t.Fatal("JSON com UTF-8 inválido dentro de string aceito")
	}
}
