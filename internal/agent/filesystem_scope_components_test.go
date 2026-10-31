package agent

import (
	"assistente/internal/tools"
	"reflect"
	"testing"
)

func TestMergeFilesystemScopePreservesDeniedComponents(t *testing.T) {
	original := &tools.FilesystemScope{DenyComponents: []string{".git"}}
	merged := mergeFilesystemScope(original, &tools.FilesystemScope{Read: []string{"**"}, DenyComponents: []string{".git", "private"}})
	if !reflect.DeepEqual(merged.DenyComponents, []string{".git", "private"}) {
		t.Fatalf("componentes negados: %v", merged.DenyComponents)
	}
	initial := mergeFilesystemScope(nil, original)
	initial.DenyComponents[0] = "modificado"
	if original.DenyComponents[0] != ".git" {
		t.Fatal("escopo inicial compartilha slice mutável")
	}
}
