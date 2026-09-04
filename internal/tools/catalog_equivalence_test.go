package tools_test

import (
	"testing"

	"assistente/internal/tools"
	"assistente/internal/tools/deeplink"
	"assistente/internal/tools/feed"
	"assistente/internal/tools/filesystem"
	"assistente/internal/tools/history"
	"assistente/internal/tools/job"
	profiletool "assistente/internal/tools/profile"
	"assistente/internal/tools/questionnaire"
	"assistente/internal/tools/shell"
	"assistente/internal/tools/skillloader"
	"assistente/internal/tools/subagent"
	"assistente/internal/tools/tasklist"
	"assistente/internal/tools/web"
)

// goldenBuiltinCatalogMetadata nasceu do antigo mapa central de metadados que
// vivia em internal/tools/catalog.go antes da Fase 1 do AEP-0077 (#122) e é
// estendido quando uma builtin nova nasce. Serve como contrato executável para
// category/class/package/risk de todas as tools registráveis.
var goldenBuiltinCatalogMetadata = map[string]tools.CatalogMetadata{
	"read_file":            {Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"},
	"list_directory":       {Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"},
	"search_files":         {Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"},
	"grep_search":          {Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"},
	"write_file":           {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"},
	"edit_file":            {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"},
	"apply_patch":          {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"},
	"move_file":            {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"},
	"copy_file":            {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"},
	"delete_file":          {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "destructive"},
	"make_directory":       {Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"},
	"run_command":          {Category: "shell", Class: "run_commands", Package: "coding_edit", Risk: "shell"},
	"web_search":           {Category: "web", Class: "web_lookup", Package: "web", Risk: "network"},
	"web_fetch":            {Category: "web", Class: "web_lookup", Package: "web", Risk: "network"},
	"http_request":         {Category: "http", Class: "http_api", Package: "web", Risk: "network"},
	"feed_read":            {Category: "web", Class: "web_lookup", Package: "web", Risk: "network"},
	"search_conversations": {Category: "history", Class: "read_context", Package: "history", Risk: "read"},
	"get_messages":         {Category: "history", Class: "read_context", Package: "history", Risk: "read"},
	"collect_responses":    {Category: "questionnaire", Class: "app_tool", Package: "basic", Risk: "read"},
	"update_plan":          {Category: "tasklist", Class: "task_management", Package: "tasks", Risk: "write"},
	"task_list":            {Category: "tasklist", Class: "task_management", Package: "tasks", Risk: "write"},
	"task":                 {Category: "tasklist", Class: "task_management", Package: "tasks", Risk: "write"},
	"task_note":            {Category: "tasklist", Class: "task_management", Package: "tasks", Risk: "write"},
	"job":                  {Category: "jobs", Class: "automation_management", Package: "jobs", Risk: "write"},
	"job_pipeline":         {Category: "jobs", Class: "automation_management", Package: "jobs", Risk: "write"},
	"open_deep_link":       {Category: "app", Class: "app_tool", Package: "basic", Risk: "read"},
	"load_skill":           {Category: "skills", Class: "runtime_control", Package: "skills", Risk: "read"},
	"subagent":             {Category: "agents", Class: "agent_delegation", Package: "agents", Risk: "write"},
	"profile":              {Category: "agents", Class: "profile_control", Package: "agents", Risk: "write"},
}

// builtinsUnderTest instancia todas as builtins com metadados de catálogo. Os
// construtores recebem dependências nil/mínimas — o teste só consulta
// Name/Description/Parameters/CatalogMetadata, nunca Execute.
func builtinsUnderTest() []tools.Tool {
	return []tools.Tool{
		filesystem.NewReadFile("."),
		filesystem.NewListDirectory("."),
		filesystem.NewSearchFiles("."),
		filesystem.NewGrepSearch("."),
		filesystem.NewWriteFile("."),
		filesystem.NewEditFile(".", nil),
		filesystem.NewApplyPatch(".", nil),
		filesystem.NewMoveFile("."),
		filesystem.NewCopyFile("."),
		filesystem.NewDeleteFile("."),
		filesystem.NewMakeDirectory("."),
		shell.NewRunCommand(nil, nil, nil, "."),
		web.NewWebSearch(nil),
		web.NewWebFetch(nil),
		web.NewHTTPRequest(nil),
		feed.NewFeedRead(nil),
		history.NewSearchConversationsForTest(nil),
		history.NewGetMessages(),
		questionnaire.NewCollectResponses(nil),
		tasklist.NewUpdatePlan(nil),
		tasklist.NewTaskList(nil),
		tasklist.NewTask(nil),
		tasklist.NewTaskNote(nil),
		job.NewJobWithProvider(nil),
		job.NewPipelineWithProvider(nil),
		deeplink.NewOpenDeepLink(nil),
		skillloader.New(nil, nil),
		subagent.NewWithProvider(nil),
		profiletool.New(nil, nil),
	}
}

func TestBuiltinCatalogEntriesMatchGolden(t *testing.T) {
	seen := make(map[string]bool, len(goldenBuiltinCatalogMetadata))

	for _, tool := range builtinsUnderTest() {
		entry := tools.CatalogEntryFromTool(tool)
		name := tool.Name()
		seen[name] = true

		want, ok := goldenBuiltinCatalogMetadata[name]
		if !ok {
			t.Fatalf("builtin %q não está no golden de equivalência", name)
		}

		if entry.Name != name {
			t.Errorf("%s: Name esperado %q, obtido %q", name, name, entry.Name)
		}
		if entry.DisplayName != name {
			t.Errorf("%s: DisplayName esperado %q, obtido %q", name, name, entry.DisplayName)
		}
		if entry.Origin != tools.ToolOriginBuiltin {
			t.Errorf("%s: Origin esperado %q, obtido %q", name, tools.ToolOriginBuiltin, entry.Origin)
		}
		if entry.Category != want.Category || entry.Class != want.Class || entry.Package != want.Package || entry.Risk != want.Risk {
			t.Errorf("%s: metadados divergentes\n  esperado: %+v\n  obtido:   {Category:%s Class:%s Package:%s Risk:%s}",
				name, want, entry.Category, entry.Class, entry.Package, entry.Risk)
		}
		if len(entry.Tags) != 0 {
			t.Errorf("%s: nenhuma builtin tinha tags no mapa central, obtido %#v", name, entry.Tags)
		}
		if want := tools.SchemaHash(tool.Parameters()); entry.SchemaHash != want {
			t.Errorf("%s: SchemaHash divergente, esperado %q, obtido %q", name, want, entry.SchemaHash)
		}
		if entry.SchemaBytes != len(tool.Parameters()) {
			t.Errorf("%s: SchemaBytes divergente, esperado %d, obtido %d", name, len(tool.Parameters()), entry.SchemaBytes)
		}
		if entry.AvailabilityStatus != tools.ToolAvailabilityAvailable {
			t.Errorf("%s: AvailabilityStatus esperado %q, obtido %q", name, tools.ToolAvailabilityAvailable, entry.AvailabilityStatus)
		}
	}

	for name := range goldenBuiltinCatalogMetadata {
		if !seen[name] {
			t.Errorf("builtin %q do golden não foi coberta pelo teste de equivalência", name)
		}
	}
}
