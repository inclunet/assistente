package llm

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"assistente/internal/configdir"
)

const defaultDebugDumpMaxFiles = 200

var debugDumpBaseDirOverride string

type DebugDumpHandle struct {
	Dir             string
	ConversationDir string
	MaxFiles        int
}

type debugDumpMeta struct {
	CreatedAt      string `json:"created_at"`
	ProviderID     string `json:"provider_id,omitempty"`
	ProviderName   string `json:"provider_name,omitempty"`
	APIFormat      string `json:"api_format,omitempty"`
	Model          string `json:"model,omitempty"`
	ProfileSlug    string `json:"profile_slug,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	TurnID         string `json:"turn_id,omitempty"`
	RequestSHA256  string `json:"request_sha256,omitempty"`
	RequestBytes   int    `json:"request_bytes,omitempty"`
	RequestError   string `json:"request_error,omitempty"`
}

type debugPrefixDiff struct {
	ComparedWith             string `json:"compared_with,omitempty"`
	CurrentRequestBytes      int    `json:"current_request_bytes"`
	PreviousRequestBytes     int    `json:"previous_request_bytes,omitempty"`
	CommonPrefixBytes        int    `json:"common_prefix_bytes"`
	CurrentSuffixBytes       int    `json:"current_suffix_bytes"`
	PreviousSuffixBytes      int    `json:"previous_suffix_bytes,omitempty"`
	FirstDifferentByte       int    `json:"first_different_byte"`
	FirstDifferentJSONPath   string `json:"first_different_json_path,omitempty"`
	CurrentByteAtDifference  string `json:"current_byte_at_difference,omitempty"`
	PreviousByteAtDifference string `json:"previous_byte_at_difference,omitempty"`
}

func dumpLLMRequest(provider *ProviderConfig, model string, params ChatParams, payload any) *DebugDumpHandle {
	if !params.DebugDump.Enabled || (!params.DebugDump.DumpRequests && !params.DebugDump.DumpResponses) {
		return nil
	}
	baseDir := debugDumpBaseDir()
	if baseDir == "" {
		return nil
	}
	profile := safePathSegment(firstNonEmpty(params.DebugDump.ProfileSlug, "unknown-profile"))
	conversationID := safePathSegment(firstNonEmpty(params.DebugDump.ConversationID, "unknown-conversation"))
	turnID := safePathSegment(firstNonEmpty(params.DebugDump.TurnID, "unknown-turn"))
	runName := uniqueDebugDumpRunName(turnID)
	profileDir := filepath.Join(baseDir, profile)
	conversationDir := filepath.Join(profileDir, conversationID)
	runDir := filepath.Join(conversationDir, runName)
	for _, dir := range []string{profileDir, conversationDir, runDir} {
		if err := ensurePrivateDebugDir(dir); err != nil {
			return nil
		}
	}

	meta := debugDumpMeta{
		CreatedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		Model:          model,
		ProfileSlug:    params.DebugDump.ProfileSlug,
		ConversationID: params.DebugDump.ConversationID,
		TurnID:         params.DebugDump.TurnID,
	}
	if provider != nil {
		meta.ProviderID = provider.ID
		meta.ProviderName = provider.Name
		meta.APIFormat = string(provider.APIFormat)
	}
	if params.DebugDump.DumpRequests {
		requestBytes, err := redactedJSON(payload)
		if err != nil {
			meta.RequestError = err.Error()
			if !params.DebugDump.DumpResponses {
				_ = os.RemoveAll(runDir)
				pruneDebugDumps(conversationDir, params.DebugDump.MaxFiles)
				return nil
			}
		} else if err := os.WriteFile(filepath.Join(runDir, "request.json"), requestBytes, 0600); err != nil {
			meta.RequestError = err.Error()
			if !params.DebugDump.DumpResponses {
				_ = os.RemoveAll(runDir)
				pruneDebugDumps(conversationDir, params.DebugDump.MaxFiles)
				return nil
			}
		} else {
			sha := sha256.Sum256(requestBytes)
			meta.RequestSHA256 = hex.EncodeToString(sha[:])
			meta.RequestBytes = len(requestBytes)
			writePrefixDiff(filepath.Join(runDir, "prefix-diff.json"), previousRequestPath(conversationDir, runDir), requestBytes)
		}
	}
	writeJSON(filepath.Join(runDir, "meta.json"), meta)
	if !params.DebugDump.DumpResponses {
		pruneDebugDumps(conversationDir, params.DebugDump.MaxFiles)
	}
	return &DebugDumpHandle{Dir: runDir, ConversationDir: conversationDir, MaxFiles: params.DebugDump.MaxFiles}
}

func uniqueDebugDumpRunName(turnID string) string {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err == nil {
		return time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + turnID + "-" + hex.EncodeToString(suffix[:])
	}
	return time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + turnID
}

func dumpLLMResponse(handle *DebugDumpHandle, params ChatParams, payload any) {
	if handle == nil || handle.Dir == "" || !params.DebugDump.Enabled || !params.DebugDump.DumpResponses {
		return
	}
	writeJSON(filepath.Join(handle.Dir, "response.json"), payload)
	if handle.ConversationDir != "" {
		pruneDebugDumpHandle(handle)
	}
}

func pruneDebugDumpHandle(handle *DebugDumpHandle) {
	if handle == nil || handle.ConversationDir == "" {
		return
	}
	pruneDebugDumps(handle.ConversationDir, handle.MaxFiles)
}

func debugDumpBaseDir() string {
	if debugDumpBaseDirOverride != "" {
		if err := ensurePrivateDebugDir(debugDumpBaseDirOverride); err != nil {
			return ""
		}
		return debugDumpBaseDirOverride
	}
	resolver := configdir.NewResolver(filepath.Join("debug", "llm-dumps"))
	if err := resolver.EnsureHomeDir(); err != nil {
		return ""
	}
	baseDir := resolver.GetHomeDir()
	if err := ensurePrivateDebugDir(baseDir); err != nil {
		return ""
	}
	return baseDir
}

func ensurePrivateDebugDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

func writeJSON(path string, value any) {
	data, err := redactedJSON(value)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}

func redactedJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	return json.MarshalIndent(redactSensitiveKeys(decoded), "", "  ")
}

func redactSensitiveKeys(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if isSensitiveDumpKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = redactSensitiveKeys(child)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for idx, child := range v {
			out[idx] = redactSensitiveKeys(child)
		}
		return out
	default:
		return value
	}
}

func isSensitiveDumpKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	compact := strings.ReplaceAll(normalized, "_", "")
	switch normalized {
	case "authorization", "proxy_authorization", "api_key", "apikey", "x_api_key", "access_token", "refresh_token", "id_token", "token", "secret", "client_secret", "password", "cookie", "set_cookie":
		return true
	default:
		return strings.HasSuffix(normalized, "_api_key") ||
			strings.HasSuffix(normalized, "_token") ||
			strings.HasSuffix(normalized, "_secret") ||
			strings.HasSuffix(compact, "apikey") ||
			strings.HasSuffix(compact, "token") ||
			strings.HasSuffix(compact, "secret") ||
			strings.HasSuffix(compact, "cookie")
	}
}

func writePrefixDiff(path string, previousPath string, current []byte) {
	diff := debugPrefixDiff{
		CurrentRequestBytes: len(current),
		FirstDifferentByte:  -1,
		CommonPrefixBytes:   len(current),
	}
	if previousPath != "" {
		if previous, err := os.ReadFile(previousPath); err == nil {
			prefix := commonPrefixLen(previous, current)
			diff.ComparedWith = previousRequestID(previousPath)
			diff.PreviousRequestBytes = len(previous)
			diff.CommonPrefixBytes = prefix
			diff.CurrentSuffixBytes = len(current) - prefix
			diff.PreviousSuffixBytes = len(previous) - prefix
			if prefix < len(current) || prefix < len(previous) {
				diff.FirstDifferentByte = prefix
				diff.CurrentByteAtDifference = byteAt(current, prefix)
				diff.PreviousByteAtDifference = byteAt(previous, prefix)
				diff.FirstDifferentJSONPath = firstDifferentJSONPath(previous, current)
			}
		}
	}
	if diff.ComparedWith == "" {
		diff.CurrentSuffixBytes = 0
	}
	writeJSON(path, diff)
}

func previousRequestID(previousPath string) string {
	return filepath.Base(filepath.Dir(previousPath))
}

func previousRequestPath(conversationDir, currentRunDir string) string {
	entries, err := os.ReadDir(conversationDir)
	if err != nil {
		return ""
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(conversationDir, entry.Name())
		if filepath.Clean(dir) == filepath.Clean(currentRunDir) {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "request.json")); err == nil {
			dirs = append(dirs, dir)
		}
	}
	sort.Strings(dirs)
	if len(dirs) == 0 {
		return ""
	}
	return filepath.Join(dirs[len(dirs)-1], "request.json")
}

func pruneDebugDumps(conversationDir string, maxFiles int) {
	if maxFiles <= 0 {
		maxFiles = defaultDebugDumpMaxFiles
	}
	entries, err := os.ReadDir(conversationDir)
	if err != nil {
		return
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(conversationDir, entry.Name()))
		}
	}
	sort.Strings(dirs)
	for len(dirs) > maxFiles {
		_ = os.RemoveAll(dirs[0])
		dirs = dirs[1:]
	}
}

func commonPrefixLen(a, b []byte) int {
	max := len(a)
	if len(b) < max {
		max = len(b)
	}
	for i := 0; i < max; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return max
}

func byteAt(data []byte, idx int) string {
	if idx < 0 || idx >= len(data) {
		return ""
	}
	return fmt.Sprintf("0x%02x", data[idx])
}

func firstDifferentJSONPath(previous, current []byte) string {
	var prevValue any
	var currValue any
	if json.Unmarshal(previous, &prevValue) != nil || json.Unmarshal(current, &currValue) != nil {
		return ""
	}
	return compareJSONValue(prevValue, currValue, "$")
}

func compareJSONValue(a, b any, path string) string {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return path
		}
		keys := make([]string, 0, len(av)+len(bv))
		seen := map[string]struct{}{}
		for key := range av {
			keys = append(keys, key)
			seen[key] = struct{}{}
		}
		for key := range bv {
			if _, ok := seen[key]; !ok {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPath := path + "." + key
			_, aOK := av[key]
			_, bOK := bv[key]
			if !aOK || !bOK {
				return childPath
			}
			if child := compareJSONValue(av[key], bv[key], childPath); child != "" {
				return child
			}
		}
		return ""
	case []any:
		bv, ok := b.([]any)
		if !ok {
			return path
		}
		max := len(av)
		if len(bv) < max {
			max = len(bv)
		}
		for idx := 0; idx < max; idx++ {
			if child := compareJSONValue(av[idx], bv[idx], fmt.Sprintf("%s[%d]", path, idx)); child != "" {
				return child
			}
		}
		if len(av) != len(bv) {
			return fmt.Sprintf("%s[%d]", path, max)
		}
		return ""
	default:
		if !jsonScalarEqual(a, b) {
			return path
		}
		return ""
	}
}

func jsonScalarEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(ab, bb)
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var sb strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			sb.WriteRune(r)
			continue
		}
		sb.WriteByte('_')
	}
	out := strings.Trim(sb.String(), "._")
	if out == "" {
		return "unknown"
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
