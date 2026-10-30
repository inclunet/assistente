package llmcapabilities

import (
	"strings"
	"unicode/utf8"
)

// ValidRemoteModelID matches the persisted identity domain without normalizing it.
func ValidRemoteModelID(id string) bool {
	return utf8.ValidString(id) && strings.TrimSpace(id) != "" && utf8.RuneCountInString(id) <= 512 && !strings.ContainsRune(id, 0)
}
