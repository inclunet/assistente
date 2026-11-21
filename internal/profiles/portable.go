package profiles

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

// PortableDefinition carries conversational preferences across accounts.
// Provider credentials, command grants and local resource IDs have no representation here.
type PortableDefinition struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"maxTokens,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
}

func ReadPortableDefinition(raw []byte) (*Profile, error) {
	if len(raw) > 64<<10 || !utf8.Valid(raw) {
		return nil, errors.New("invalid portable profile encoding or size")
	}
	var def PortableDefinition
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&def); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF || strings.TrimSpace(def.Name) == "" {
		return nil, errors.New("invalid portable profile")
	}
	p := DefaultProfile()
	p.Name = def.Name
	p.Description = def.Description
	p.Chat.Temperature = def.Temperature
	p.Chat.MaxTokens = def.MaxTokens
	p.Chat.TopP = def.TopP
	p.Chat.DisableTools = true
	p.Chat.DisableSkills = true
	p.Chat.ToolPolicy = nil
	p.Chat.ToolPolicyDefault = "disabled"
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}
