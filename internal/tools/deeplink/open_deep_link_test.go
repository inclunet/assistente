package deeplink

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeEmitter struct {
	lastURI string
	calls   int
}

func (f *fakeEmitter) EmitDeepLink(uri string) {
	f.lastURI = uri
	f.calls++
}

func TestOpenDeepLink_DescriptionDocumentsNavigationContract(t *testing.T) {
	description := strings.ToLower(NewOpenDeepLink(nil).Description())
	requiredConcepts := []string{
		"use when:",
		"do not use:",
		"markdown deep link",
		"conversation/new with optional message and title",
		"send?message=... with required message",
		"tasklist/new",
		"editor/new",
		"accept optional title",
		"editor/open?file=... requires file",
		"focus",
		"matching chat tab",
		"live terminal ids",
		"editor/{id} opens a new editor tab",
		"editor tabs are not matched by that id",
		"tab open:",
		"caller-aware",
		"no caller context",
		"returns to the profiles list",
		"tab=voice",
		"interrupt",
		"terminal/new",
		"frontend terminal flow",
		"rather than the run_command tool",
		"user explicitly asks",
		"does not itself grant content access",
		"non-empty uri",
		"after trimming",
		"assistente:// prefix",
		"unsupported navigate routes",
		"invalid required or validated parameter combinations",
		"tool_catalog",
		"empty route",
		"assistente://navigate/",
		`{"uri":`,
	}

	for _, concept := range requiredConcepts {
		assert.Contains(t, description, strings.ToLower(concept), "Description() deve documentar %q", concept)
	}

	validatedRoutes := []string{
		"settings",
		"settings/providers",
		"settings/mcp",
		"settings/skills",
		"settings/channels",
		"settings/contacts",
		"settings/credentials",
		"settings/allowlists",
		"settings/network-allowlist",
		"settings/path-allowlist",
		"settings/appearance",
		"settings/data",
		"settings/restore-defaults",
		"profiles",
		"history",
		"memories",
		"tasklists",
		"help",
		"about",
		"update",
	}
	for _, route := range validatedRoutes {
		assert.Contains(t, description, route, "Description() deve listar a rota validada %q", route)
	}
}

func TestOpenDeepLink_ParametersDescribeSafeURIConstruction(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
		Required             []string `json:"required"`
		AdditionalProperties *bool    `json:"additionalProperties"`
	}
	assert.NoError(t, json.Unmarshal(NewOpenDeepLink(nil).Parameters(), &schema))

	uriDescription := strings.ToLower(schema.Properties["uri"].Description)
	for _, concept := range []string{
		"exact assistente:// uri",
		"link=",
		"do not invent",
		"url-encode",
		"unsupported navigate routes",
		"invalid required or validated parameter combinations",
		"frontend parser",
	} {
		assert.Contains(t, uriDescription, concept)
	}
	assert.Equal(t, []string{"uri"}, schema.Required)
	if assert.NotNil(t, schema.AdditionalProperties, "schema deve declarar additionalProperties") {
		assert.False(t, *schema.AdditionalProperties)
	}
}

func TestOpenDeepLink_Execute(t *testing.T) {
	emitter := &fakeEmitter{}
	tool := NewOpenDeepLink(emitter)

	assert.Equal(t, "open_deep_link", tool.Name())

	t.Run("valid URI", func(t *testing.T) {
		args, _ := json.Marshal(openDeepLinkArgs{URI: "assistente://conversation/42"})
		result, err := tool.Execute(context.Background(), args)
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content, "assistente://conversation/42")
		assert.Equal(t, "assistente://conversation/42", emitter.lastURI)
		assert.Equal(t, 1, emitter.calls)
	})

	t.Run("empty URI", func(t *testing.T) {
		args, _ := json.Marshal(openDeepLinkArgs{URI: ""})
		result, err := tool.Execute(context.Background(), args)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content, "empty")
	})

	t.Run("invalid protocol", func(t *testing.T) {
		args, _ := json.Marshal(openDeepLinkArgs{URI: "http://example.com"})
		result, err := tool.Execute(context.Background(), args)
		assert.NoError(t, err)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content, "assistente://")
	})

	t.Run("bad JSON", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), []byte(`{invalid`))
		assert.NoError(t, err)
		assert.True(t, result.IsError)
	})

	t.Run("tasklist URI", func(t *testing.T) {
		emitter.calls = 0
		args, _ := json.Marshal(openDeepLinkArgs{URI: "assistente://tasklist/5"})
		result, err := tool.Execute(context.Background(), args)
		assert.NoError(t, err)
		assert.False(t, result.IsError)
		assert.Equal(t, "assistente://tasklist/5", emitter.lastURI)
	})
}
