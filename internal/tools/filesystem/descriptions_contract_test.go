package filesystem

import (
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/tools"
)

type parameterContract struct {
	Type                 string                       `json:"type"`
	Properties           map[string]parameterContract `json:"properties"`
	Items                *parameterContract           `json:"items"`
	Required             []string                     `json:"required"`
	AdditionalProperties *bool                        `json:"additionalProperties"`
	Description          string                       `json:"description"`
}

func filesystemToolsForDescriptionContract(t *testing.T) map[string]tools.Tool {
	t.Helper()
	workDir := t.TempDir()
	return map[string]tools.Tool{
		"read_file":      NewReadFile(workDir),
		"search_files":   NewSearchFiles(workDir),
		"grep_search":    NewGrepSearch(workDir),
		"list_directory": NewListDirectory(workDir),
		"write_file":     NewWriteFile(workDir),
		"edit_file":      NewEditFile(workDir, nil),
		"apply_patch":    NewApplyPatch(workDir, nil),
		"copy_file":      NewCopyFile(workDir),
		"move_file":      NewMoveFile(workDir),
		"delete_file":    NewDeleteFile(workDir),
		"make_directory": NewMakeDirectory(workDir),
		"text_edit":      NewTextEdit(workDir, nil),
	}
}

func TestFilesystemDescriptionsExplainSelectionAndRisk(t *testing.T) {
	allTools := filesystemToolsForDescriptionContract(t)
	expectedReferences := map[string][]string{
		"read_file":      {"search_files", "grep_search", "list_directory"},
		"search_files":   {"grep_search", "read_file", "list_directory"},
		"grep_search":    {"search_files", "read_file"},
		"list_directory": {"read_file", "search_files", "grep_search"},
		"write_file":     {"edit_file", "apply_patch"},
		"edit_file":      {"write_file", "apply_patch"},
		"apply_patch":    {"edit_file", "write_file"},
		"copy_file":      {"move_file"},
		"move_file":      {"copy_file"},
		"delete_file":    {"directories"},
		"make_directory": {"write_file"},
		"text_edit":      {"edit_file"},
	}

	for name, tool := range allTools {
		t.Run(name, func(t *testing.T) {
			description := tool.Description()
			for _, marker := range []string{"Use ", "Do not use", "Risk:"} {
				if !strings.Contains(description, marker) {
					t.Errorf("description must contain %q guidance: %s", marker, description)
				}
			}
			for _, reference := range expectedReferences[name] {
				if !strings.Contains(description, reference) {
					t.Errorf("description must distinguish %s from %s", name, reference)
				}
			}
		})
	}
}

func TestFilesystemParameterSchemasKeepContractAndDescribeEveryField(t *testing.T) {
	allTools := filesystemToolsForDescriptionContract(t)
	expectedProperties := map[string][]string{
		"read_file":      {"document_mode", "limit", "offset", "path"},
		"search_files":   {"max_results", "path", "pattern"},
		"grep_search":    {"case_sensitive", "context_lines", "document_mode", "include", "max_results", "path", "pattern"},
		"list_directory": {"max_depth", "path", "recursive"},
		"write_file":     {"content", "path"},
		"edit_file":      {"new_string", "old_string", "path", "replace_all"},
		"apply_patch":    {"hunks", "path"},
		"copy_file":      {"from", "overwrite", "to"},
		"move_file":      {"from", "overwrite", "to"},
		"delete_file":    {"missing_ok", "path"},
		"make_directory": {"parents", "path"},
		"text_edit":      {"description", "format", "notes", "original", "replacement", "title"},
	}
	expectedRequired := map[string][]string{
		"read_file":      {"path"},
		"search_files":   {"pattern"},
		"grep_search":    {"pattern"},
		"list_directory": nil,
		"write_file":     {"path", "content"},
		"edit_file":      {"path", "old_string", "new_string"},
		"apply_patch":    {"path", "hunks"},
		"copy_file":      {"from", "to"},
		"move_file":      {"from", "to"},
		"delete_file":    {"path"},
		"make_directory": {"path"},
		"text_edit":      {"original", "replacement"},
	}

	for name, tool := range allTools {
		t.Run(name, func(t *testing.T) {
			var schema parameterContract
			if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
				t.Fatalf("invalid parameter schema: %v", err)
			}
			if schema.Type != "object" {
				t.Errorf("root type = %q, want object", schema.Type)
			}
			if schema.AdditionalProperties == nil || *schema.AdditionalProperties {
				t.Error("additionalProperties must remain false")
			}
			if len(schema.Properties) != len(expectedProperties[name]) {
				t.Fatalf("properties = %v, want %v", propertyNames(schema.Properties), expectedProperties[name])
			}
			assertSameStrings(t, "required", schema.Required, expectedRequired[name])
			for _, propertyName := range expectedProperties[name] {
				property, ok := schema.Properties[propertyName]
				if !ok {
					t.Errorf("missing property %q", propertyName)
					continue
				}
				assertParameterDescriptions(t, propertyName, property)
			}
		})
	}
}

func assertParameterDescriptions(t *testing.T, path string, parameter parameterContract) {
	t.Helper()
	if strings.TrimSpace(parameter.Description) == "" {
		t.Errorf("property %q has no description", path)
	}
	for name, child := range parameter.Properties {
		assertParameterDescriptions(t, path+"."+name, child)
	}
	if parameter.Items != nil {
		for name, child := range parameter.Items.Properties {
			assertParameterDescriptions(t, path+"[]."+name, child)
		}
	}
}

func assertSameStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	counts := make(map[string]int, len(want))
	for _, value := range want {
		counts[value]++
	}
	for _, value := range got {
		counts[value]--
	}
	for value, count := range counts {
		if count != 0 {
			t.Errorf("%s differs for %q: got %v, want %v", label, value, got, want)
		}
	}
}

func propertyNames(properties map[string]parameterContract) []string {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	return names
}
