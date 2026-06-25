package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDumpLLMRequestWritesRequestMetaAndRedactsSecrets(t *testing.T) {
	baseDir := t.TempDir()
	debugDumpBaseDirOverride = baseDir
	t.Cleanup(func() { debugDumpBaseDirOverride = "" })

	handle := dumpLLMRequest(
		&ProviderConfig{ID: "openai", Name: "OpenAI", APIFormat: APIFormatOpenAIResponses},
		"gpt-test",
		ChatParams{
			DebugDump: DebugDumpConfig{
				Enabled:        true,
				DumpRequests:   true,
				DumpResponses:  true,
				MaxFiles:       10,
				ProfileSlug:    "dev",
				ConversationID: "conv-1",
				TurnID:         "turn-1",
			},
		},
		map[string]any{
			"model": "gpt-test",
			"input": []any{map[string]any{"role": "user", "content": "oi"}},
			"headers": map[string]any{
				"Authorization": "Bearer secret",
				"x-api-key":     "secret-key",
				"Cookie":        "session=secret-cookie",
			},
			"accessToken":  "access-token-secret",
			"refreshToken": "refresh-token-secret",
			"clientSecret": "client-secret-value",
			"setCookie":    "set-cookie-secret",
		},
	)
	if handle == nil || handle.Dir == "" {
		t.Fatal("dump handle vazio")
	}

	requestBytes, err := os.ReadFile(filepath.Join(handle.Dir, "request.json"))
	if err != nil {
		t.Fatalf("request.json não gravado: %v", err)
	}
	if string(requestBytes) == "" {
		t.Fatal("request.json vazio")
	}
	for _, leaked := range []string{
		"Bearer secret",
		"secret-key",
		"session=secret-cookie",
		"access-token-secret",
		"refresh-token-secret",
		"client-secret-value",
		"set-cookie-secret",
	} {
		if containsString(requestBytes, leaked) {
			t.Fatalf("request.json contém segredo %q: %s", leaked, string(requestBytes))
		}
	}
	if !containsString(requestBytes, "[redacted]") {
		t.Fatalf("request.json não redigiu segredo: %s", string(requestBytes))
	}

	var meta debugDumpMeta
	if err := readJSON(filepath.Join(handle.Dir, "meta.json"), &meta); err != nil {
		t.Fatalf("meta inválido: %v", err)
	}
	if meta.ProviderID != "openai" || meta.Model != "gpt-test" || meta.ProfileSlug != "dev" || meta.ConversationID != "conv-1" || meta.TurnID != "turn-1" {
		t.Fatalf("meta inesperado: %#v", meta)
	}
}

func TestDebugDumpDirectoriesArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX mode bits consistently")
	}
	baseDir := t.TempDir()
	profileDir := filepath.Join(baseDir, "dev")
	conversationDir := filepath.Join(profileDir, "conv-1")
	if err := os.MkdirAll(conversationDir, 0755); err != nil {
		t.Fatalf("precreate permissive dirs: %v", err)
	}
	if err := os.Chmod(profileDir, 0755); err != nil {
		t.Fatalf("chmod profile dir: %v", err)
	}
	if err := os.Chmod(conversationDir, 0755); err != nil {
		t.Fatalf("chmod conversation dir: %v", err)
	}
	debugDumpBaseDirOverride = baseDir
	t.Cleanup(func() { debugDumpBaseDirOverride = "" })

	handle := dumpLLMRequest(nil, "gpt-test", ChatParams{DebugDump: DebugDumpConfig{
		Enabled:        true,
		DumpRequests:   true,
		MaxFiles:       10,
		ProfileSlug:    "dev",
		ConversationID: "conv-1",
		TurnID:         "turn-1",
	}}, map[string]any{"input": "oi"})
	if handle == nil {
		t.Fatal("dump não criado")
	}
	for _, dir := range []string{baseDir, profileDir, conversationDir, handle.Dir} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if got := info.Mode().Perm(); got != 0700 {
			t.Fatalf("mode %s = %o, want 0700", dir, got)
		}
	}
}

func TestDumpLLMRequestWritesPrefixDiffAgainstPreviousDump(t *testing.T) {
	baseDir := t.TempDir()
	debugDumpBaseDirOverride = baseDir
	t.Cleanup(func() { debugDumpBaseDirOverride = "" })

	params := ChatParams{DebugDump: DebugDumpConfig{
		Enabled:        true,
		DumpRequests:   true,
		MaxFiles:       10,
		ProfileSlug:    "dev",
		ConversationID: "conv-1",
		TurnID:         "turn-1",
	}}
	first := dumpLLMRequest(nil, "gpt-test", params, map[string]any{"input": []any{"abc"}})
	if first == nil {
		t.Fatal("primeiro dump não criado")
	}
	params.DebugDump.TurnID = "turn-2"
	second := dumpLLMRequest(nil, "gpt-test", params, map[string]any{"input": []any{"abd"}})
	if second == nil {
		t.Fatal("segundo dump não criado")
	}

	var diff debugPrefixDiff
	if err := readJSON(filepath.Join(second.Dir, "prefix-diff.json"), &diff); err != nil {
		t.Fatalf("prefix-diff inválido: %v", err)
	}
	if diff.ComparedWith == "" || diff.FirstDifferentByte < 0 || diff.CommonPrefixBytes == 0 {
		t.Fatalf("prefix-diff incompleto: %#v", diff)
	}
	if strings.Contains(diff.ComparedWith, baseDir) || filepath.IsAbs(diff.ComparedWith) {
		t.Fatalf("ComparedWith vazou caminho absoluto: %q", diff.ComparedWith)
	}
	if diff.FirstDifferentJSONPath == "" {
		t.Fatalf("prefix-diff sem path JSON aproximado: %#v", diff)
	}
}

func TestDumpLLMResponseWorksWhenRequestsDisabled(t *testing.T) {
	baseDir := t.TempDir()
	debugDumpBaseDirOverride = baseDir
	t.Cleanup(func() { debugDumpBaseDirOverride = "" })

	params := ChatParams{DebugDump: DebugDumpConfig{
		Enabled:        true,
		DumpRequests:   false,
		DumpResponses:  true,
		MaxFiles:       10,
		ProfileSlug:    "dev",
		ConversationID: "conv-1",
		TurnID:         "turn-1",
	}}
	handle := dumpLLMRequest(nil, "gpt-test", params, map[string]any{"input": "not saved"})
	if handle == nil {
		t.Fatal("handle não criado para response-only")
	}
	if _, err := os.Stat(filepath.Join(handle.Dir, "request.json")); !os.IsNotExist(err) {
		t.Fatalf("request.json err = %v, want not exist", err)
	}

	dumpLLMResponse(handle, params, map[string]any{"content": "ok"})
	var response map[string]any
	if err := readJSON(filepath.Join(handle.Dir, "response.json"), &response); err != nil {
		t.Fatalf("response inválida: %v", err)
	}
	if response["content"] != "ok" {
		t.Fatalf("response = %#v, want content ok", response)
	}
}

func TestDumpLLMResponseWorksWhenRequestSerializationFails(t *testing.T) {
	baseDir := t.TempDir()
	debugDumpBaseDirOverride = baseDir
	t.Cleanup(func() { debugDumpBaseDirOverride = "" })

	params := ChatParams{DebugDump: DebugDumpConfig{
		Enabled:        true,
		DumpRequests:   true,
		DumpResponses:  true,
		MaxFiles:       10,
		ProfileSlug:    "dev",
		ConversationID: "conv-1",
		TurnID:         "turn-1",
	}}
	handle := dumpLLMRequest(nil, "gpt-test", params, map[string]any{"bad": make(chan int)})
	if handle == nil {
		t.Fatal("handle não criado quando response está habilitada")
	}
	var meta debugDumpMeta
	if err := readJSON(filepath.Join(handle.Dir, "meta.json"), &meta); err != nil {
		t.Fatalf("meta inválido: %v", err)
	}
	if meta.RequestError == "" {
		t.Fatalf("RequestError vazio: %#v", meta)
	}

	dumpLLMResponse(handle, params, map[string]any{"content": "ok"})
	var response map[string]any
	if err := readJSON(filepath.Join(handle.Dir, "response.json"), &response); err != nil {
		t.Fatalf("response inválida: %v", err)
	}
}

func TestDumpLLMRequestRemovesRunDirWhenRequestSerializationFailsWithoutResponses(t *testing.T) {
	baseDir := t.TempDir()
	debugDumpBaseDirOverride = baseDir
	t.Cleanup(func() { debugDumpBaseDirOverride = "" })

	params := ChatParams{DebugDump: DebugDumpConfig{
		Enabled:        true,
		DumpRequests:   true,
		DumpResponses:  false,
		MaxFiles:       10,
		ProfileSlug:    "dev",
		ConversationID: "conv-1",
		TurnID:         "turn-1",
	}}
	if handle := dumpLLMRequest(nil, "gpt-test", params, map[string]any{"bad": make(chan int)}); handle != nil {
		t.Fatalf("handle = %#v, want nil", handle)
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "dev", "conv-1"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("falha lendo diretório: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("diretórios residuais = %d, want 0", len(entries))
	}
}

func TestDumpLLMRequestPrunesOldDumps(t *testing.T) {
	baseDir := t.TempDir()
	debugDumpBaseDirOverride = baseDir
	t.Cleanup(func() { debugDumpBaseDirOverride = "" })

	params := ChatParams{DebugDump: DebugDumpConfig{
		Enabled:        true,
		DumpRequests:   true,
		MaxFiles:       2,
		ProfileSlug:    "dev",
		ConversationID: "conv-1",
		TurnID:         "turn",
	}}
	for i := 0; i < 4; i++ {
		if dumpLLMRequest(nil, "gpt-test", params, map[string]any{"n": i}) == nil {
			t.Fatalf("dump %d não criado", i)
		}
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "dev", "conv-1"))
	if err != nil {
		t.Fatalf("falha lendo diretório de dumps: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("dumps retidos = %d, want 2", len(entries))
	}
}

func TestDumpLLMRequestDefersPruneUntilResponseWhenResponsesEnabled(t *testing.T) {
	baseDir := t.TempDir()
	debugDumpBaseDirOverride = baseDir
	t.Cleanup(func() { debugDumpBaseDirOverride = "" })

	params := ChatParams{DebugDump: DebugDumpConfig{
		Enabled:        true,
		DumpRequests:   true,
		DumpResponses:  true,
		MaxFiles:       1,
		ProfileSlug:    "dev",
		ConversationID: "conv-1",
		TurnID:         "turn",
	}}
	first := dumpLLMRequest(nil, "gpt-test", params, map[string]any{"n": 1})
	second := dumpLLMRequest(nil, "gpt-test", params, map[string]any{"n": 2})
	if first == nil || second == nil {
		t.Fatal("dumps não criados")
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "dev", "conv-1"))
	if err != nil {
		t.Fatalf("falha lendo diretório de dumps: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("prune antes da response reteve %d dumps, want 2", len(entries))
	}

	dumpLLMResponse(second, params, map[string]any{"content": "ok"})
	pruneDebugDumpHandle(second)
	entries, err = os.ReadDir(filepath.Join(baseDir, "dev", "conv-1"))
	if err != nil {
		t.Fatalf("falha lendo diretório de dumps após prune: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("prune centralizado reteve %d dumps, want 1", len(entries))
	}
}

func readJSON(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func containsString(data []byte, needle string) bool {
	return strings.Contains(string(data), needle)
}
