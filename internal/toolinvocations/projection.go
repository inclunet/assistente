package toolinvocations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

func populateInputProjection(invocation *Invocation) {
	if invocation == nil {
		return
	}
	invocation.InputBytes = int64(len(invocation.Input))
	invocation.InputHash = invocationPayloadHash(invocation.Input)
	invocation.InputPreview = invocationStructuralPreview(invocation.Input)
	if invocation.OutputHash == "" {
		invocation.OutputHash = invocationPayloadHash(nil)
	}
}

func populateOutputProjection(invocation *Invocation) {
	if invocation == nil {
		return
	}
	invocation.OutputBytes = int64(len(invocation.Output))
	invocation.OutputHash = invocationPayloadHash(invocation.Output)
	invocation.OutputPreview = invocationStructuralPreview(invocation.Output)
	if len(invocation.Output) == 0 {
		invocation.ResultAvailability = "missing"
	} else {
		invocation.ResultAvailability = "available"
	}
}

func invocationPayloadHash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func invocationStructuralPreview(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	preview := map[string]any{"bytes": len(value)}
	var object map[string]json.RawMessage
	if json.Unmarshal(value, &object) == nil {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		preview["fields"] = keys
	}
	encoded, _ := json.Marshal(preview)
	return string(encoded)
}
