package commandcontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"assistente/internal/commandjson"
	"github.com/google/uuid"
)

// CommandChainMaxDepth é o limite versionado do protocolo v1. Uma história
// exatamente desse tamanho é válida como estado terminal; quem quiser anexar
// outra entrada deve rejeitá-la antes do append.
const CommandChainMaxDepth = 16

var commandChainCommandIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

var ErrInvalidCommandChainHistory = errors.New("invalid command chain history")

// CommandChainEntry é a parte estrutural, sem payload, de um salto de comando.
type CommandChainEntry struct {
	CommandID    string   `json:"command_id"`
	InvocationID string   `json:"invocation_id"`
	LayerRefs    []string `json:"layer_refs"`
}

// Validate valida uma entrada sem aceitar dados de payload.
func (e CommandChainEntry) Validate() error {
	if !commandChainCommandIDPattern.MatchString(e.CommandID) || !validCommandChainUUID(e.InvocationID) || e.LayerRefs == nil || len(e.LayerRefs) > 256 {
		return ErrInvalidCommandChainHistory
	}
	seen := make(map[string]struct{}, len(e.LayerRefs))
	for _, ref := range e.LayerRefs {
		if ref == "" || len(ref) > 256 || !utf8.ValidString(ref) || strings.TrimSpace(ref) != ref || strings.ContainsRune(ref, '\x00') {
			return ErrInvalidCommandChainHistory
		}
		if _, ok := seen[ref]; ok {
			return ErrInvalidCommandChainHistory
		}
		seen[ref] = struct{}{}
	}
	return nil
}

// DecodeCommandChainHistory decodifica e valida uma história completa. Permite
// o tamanho terminal 16; callers que fazem append impõem o limite anterior.
func DecodeCommandChainHistory(raw []byte) ([]CommandChainEntry, error) {
	if len(bytes.TrimSpace(raw)) == 0 || !utf8.Valid(raw) {
		return nil, ErrInvalidCommandChainHistory
	}
	canonical, err := commandjson.Canonicalize(raw)
	if err != nil {
		return nil, ErrInvalidCommandChainHistory
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	var history []CommandChainEntry
	if err := decoder.Decode(&history); err != nil || history == nil || len(history) > CommandChainMaxDepth {
		return nil, ErrInvalidCommandChainHistory
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, ErrInvalidCommandChainHistory
	}
	commands := make(map[string]struct{}, len(history))
	invocations := make(map[string]struct{}, len(history))
	for _, entry := range history {
		if err := entry.Validate(); err != nil {
			return nil, err
		}
		if _, ok := commands[entry.CommandID]; ok {
			return nil, ErrInvalidCommandChainHistory
		}
		if _, ok := invocations[entry.InvocationID]; ok {
			return nil, ErrInvalidCommandChainHistory
		}
		commands[entry.CommandID] = struct{}{}
		invocations[entry.InvocationID] = struct{}{}
	}
	return history, nil
}

func validCommandChainUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}
