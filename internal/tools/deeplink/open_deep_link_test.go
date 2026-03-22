package deeplink

import (
	"context"
	"encoding/json"
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
