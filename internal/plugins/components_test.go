package plugins

import (
	"strconv"
	"strings"
	"testing"

	"assistente/internal/pluginpackage"
)

func TestSkillDescriptionLimitCountsCodePoints(t *testing.T) {
	for _, tc := range []struct {
		points int
		kind   string
	}{
		{1024, "skill"},
		{1025, "invalidSkill"},
	} {
		t.Run(strconv.Itoa(tc.points), func(t *testing.T) {
			description := strings.Repeat("📝", tc.points)
			files := map[string][]byte{
				"skills/note/SKILL.md": []byte("---\nname: note\ndescription: " + description + "\n---\nNote"),
			}
			got := DiscoverComponents(&pluginpackage.LocalSnapshot{Files: files})
			if len(got) != 1 || got[0].Kind != tc.kind {
				t.Fatal(got)
			}
		})
	}
}

func TestWrongComponentEntryTypeKeepsIndependentComponents(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string][]byte
		want  map[string]string
	}{
		{
			name: "skills is a file",
			files: map[string][]byte{
				"skills":   {},
				"mcp.json": []byte(`{"$schema":"https://agent-plugins.org/schemas/1.0.0/mcp.schema.json","mcpServers":{"example":{"type":"stdio","command":"node"}}}`),
			},
			want: map[string]string{"skills": "invalidSkill", "mcp/example": "mcp"},
		},
		{
			name: "mcp.json is a directory with multiple files",
			files: map[string][]byte{
				"mcp.json/a": {}, "mcp.json/b/c": {},
				"skills/note/SKILL.md": []byte("---\nname: note\ndescription: Note\n---\nHello"),
			},
			want: map[string]string{"skills/note": "skill", "mcp.json": "invalidMcp"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := DiscoverComponents(&pluginpackage.LocalSnapshot{Files: tc.files})
			if len(got) != len(tc.want) {
				t.Fatal(got)
			}
			for _, c := range got {
				if tc.want[c.Key] != c.Kind {
					t.Fatal(got)
				}
				delete(tc.want, c.Key)
			}
		})
	}
}

func TestPluginSkillFrontmatterTypes(t *testing.T) {
	for _, tc := range []struct {
		name, fields, want string
	}{
		{"note", "name: note\ndescription: Note\nmetadata: text", "invalidSkill"},
		{"note", "name: note\ndescription: Note\nmetadata: {retries: 1}", "invalidSkill"},
		{"note", "name: note\ndescription: 42", "invalidSkill"},
		{"42", "name: 42\ndescription: Note", "invalidSkill"},
		{"note", "name: note\ndescription: Note\nlicense: false", "invalidSkill"},
		{"note", "name: note\ndescription: Note\nmetadata: {author: author}", "skill"},
	} {
		key := "skills/" + tc.name + "/SKILL.md"
		got := DiscoverComponents(&pluginpackage.LocalSnapshot{Files: map[string][]byte{key: []byte("---\n" + tc.fields + "\n---\nNote")}})
		if len(got) != 1 || got[0].Kind != tc.want || got[0].Reason != "" {
			t.Errorf("%s: %+v", tc.fields, got)
		}
	}
}
