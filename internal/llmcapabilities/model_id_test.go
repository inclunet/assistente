package llmcapabilities

import (
	"strings"
	"testing"
)

func TestRemoteModelIDPreservesDomainWithoutNormalization(t *testing.T) {
	for _, id := range []string{"model", " model ", "\tmodel", strings.Repeat("界", 512)} {
		if !ValidRemoteModelID(id) {
			t.Fatalf("valid ID rejected: %q", id)
		}
	}
	for _, id := range []string{"", " ", "\t", "a\x00b", strings.Repeat("界", 513), string([]byte{0xff})} {
		if ValidRemoteModelID(id) {
			t.Fatalf("invalid ID accepted: %q", id)
		}
	}
}
