package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestProductionCodeDoesNotUseUnsafeDatabaseAPIs(t *testing.T) {
	root := repoRoot(t)
	unsafe := regexp.MustCompile(`database\.(CreateConversation|RecycleOrCreateConversation|FindOrCreateChannelConversation|GetConversations|GetConversation|GetConversationInfo|UpdateConversation|UpdateConversationChannel|DeleteConversation|ClearAllConversations|CreateMessage|AddMessage|AddChildMessage|AddAssistantMessage|AddSystemMessage|AddToolResultMessage|AddAssistantToolMessage|UpdateMessageContent|DeleteMessage|DeleteAllMessages|GetMessages|GetRecentRootMessages|GetRootMessagesBefore|GetMessageWindow|CountChildren|GetAllTokenStats|GetTurnTokenStats|GetConversationDetailedTokenStats|GetDetailedTokenStats|GetContextWindowUsage|GetRecentMessagesTokenCount|GetConversationSummary|UpdateConversationSummary|SetSummarizingInProgress|IsSummarizingInProgress|SearchConversations|SearchMessageContent|SaveLLMProvider|GetLLMProviders|GetLLMProvider|DeleteLLMProvider|CountLLMProviders|SetDefaultProvider|GetDefaultProvider|CreateTaskList|GetTaskList|GetAllTaskLists|UpdateTaskList|SetTaskListViewMode|CloneTaskList|ClearTaskList|DeleteTaskList|GetWorkflow|UpdateWorkflow|UpdateWorkflowFull|GetTaskCountsByStatus|UpdateTaskListFull|ReorderWorkflowStatuses|ValidateStatusTransition|CreateTask|CreateTaskFull|FindTaskByCode|GetTask|GetTasksByTaskListID|GetTasksByStatus|FindTaskNoteByExternalRef|UpsertTaskNoteByExternal)\(`)

	var offenders []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "frontend", "build", "dist", "wailsjs":
				return filepath.SkipDir
			}
			if rel := slashRel(root, path); rel == "internal/database" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if unsafe.Match(data) {
			offenders = append(offenders, slashRel(root, path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("produção não deve chamar APIs database sem context.Context; use variantes WithContext ou repositórios explícitos:\n%s", strings.Join(offenders, "\n"))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("go.mod não encontrado")
		}
		wd = parent
	}
}

func slashRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
