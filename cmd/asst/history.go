package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"assistente/internal/app"
	"assistente/internal/chat"
	"assistente/internal/database"

	"github.com/spf13/cobra"
)

// historyBackend abstracts the app methods used by history commands.
type historyBackend interface {
	SearchConversationHistory(query string, limit int) ([]database.MessageSearchResult, error)
	GetConversations() ([]app.Conversation, error)
	GetConversation(id string) (*app.Conversation, error)
	ReadTimelineWindow(database.MessageWindowQuery) (*chat.TimelineWindowSnapshot, error)
	DeleteConversation(id string) error
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Gerencia histórico de conversas",
	Long:  "Lista, exibe e remove conversas do histórico.",
}

// ─── list ───────────────────────────────────────────────────────────────────

var historyListLimit int
var historyListSearch string

var historyListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lista conversas recentes",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHistoryList(asCLI(rootApp), os.Stdout, historyListSearch, historyListLimit)
	},
}

func runHistoryList(svc historyBackend, out io.Writer, search string, limit int) error {
	if search != "" {
		results, err := svc.SearchConversationHistory(search, limit)
		if err != nil {
			return fmt.Errorf("erro ao buscar: %w", err)
		}
		if len(results) == 0 {
			_, err = fmt.Fprintln(out, "Nenhum resultado encontrado.")
			return err
		}

		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "CONVERSA\tTÍTULO\tROLE\tTRECHO")
		for _, r := range results {
			snippet := r.Snippet
			snippetRunes := []rune(snippet)
			if len(snippetRunes) > 60 {
				snippet = string(snippetRunes[:57]) + "..."
			}
			snippet = strings.ReplaceAll(snippet, "\n", " ")
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.ConversationID, r.ConversationTitle, r.Role, snippet)
		}
		return w.Flush()
	}

	conversations, err := svc.GetConversations()
	if err != nil {
		return fmt.Errorf("erro ao listar conversas: %w", err)
	}
	if len(conversations) == 0 {
		_, err = fmt.Fprintln(out, "Nenhuma conversa no histórico.")
		return err
	}

	display := limit
	if display <= 0 || display > len(conversations) {
		display = len(conversations)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tTÍTULO\tMENSAGENS\tDATA")
	for i := 0; i < display; i++ {
		c := conversations[i]
		date := c.UpdatedAt.Format(time.DateOnly)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", c.ID, c.Title, c.MessageCount, date)
	}
	return w.Flush()
}

// ─── show ───────────────────────────────────────────────────────────────────

var historyShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Exibe mensagens de uma conversa",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHistoryShow(asCLI(rootApp), os.Stdout, args[0])
	},
}

func runHistoryShow(svc historyBackend, out io.Writer, id string) error {
	conv, err := svc.GetConversation(id)
	if err != nil {
		return fmt.Errorf("conversa não encontrada: %w", err)
	}

	ew := &errWriter{w: out}
	ew.printf("=== %s (ID: %s) ===\n\n", conv.Title, conv.ID)
	var version *database.ChatTimelineVersion
	var printScope func(*string) error
	printScope = func(parent *string) error {
		query := database.MessageWindowQuery{ConversationID: id, ParentID: parent, Anchor: "start", Direction: "after", Limit: database.MaxMessageWindowRows}
		for {
			snapshot, err := svc.ReadTimelineWindow(query)
			if err != nil {
				return fmt.Errorf("erro ao carregar mensagens: %w", err)
			}
			if snapshot == nil {
				return fmt.Errorf("janela do histórico indisponível")
			}
			if version != nil && *version != snapshot.Version {
				return fmt.Errorf("o histórico mudou durante a leitura; execute o comando novamente")
			}
			if snapshot.Window.HasAfter && (snapshot.AfterCursor == "" || snapshot.AfterCursor == query.Cursor) {
				return fmt.Errorf("histórico sem avanço de cursor")
			}
			if version == nil {
				v := snapshot.Version
				version = &v
			}
			for _, item := range snapshot.Items {
				role := "assistant"
				if item.Representative != nil {
					role = item.Representative.Role
				}
				var text strings.Builder
				for _, segment := range item.Segments {
					if segment.Kind == "text" {
						text.WriteString(segment.Text)
					}
				}
				content := []rune(text.String())
				if len(content) > 500 {
					content = append(content[:497], '.', '.', '.')
				}
				ew.printf("[%s] %s\n\n", strings.ToUpper(role), string(content))
				if ew.err != nil {
					return ew.err
				}
				if parent == nil && item.Representative != nil && item.Representative.Presentation != nil && item.Representative.Presentation.ChildCount > 0 {
					if err := printScope(&item.Representative.ID); err != nil {
						return err
					}
				}
			}
			if !snapshot.Window.HasAfter {
				return nil
			}
			query.Anchor, query.Cursor = "", snapshot.AfterCursor
		}
	}
	return printScope(nil)
}

// ─── delete ─────────────────────────────────────────────────────────────────

var historyDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Remove uma conversa",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runHistoryDelete(asCLI(rootApp), os.Stdout, args[0])
	},
}

func runHistoryDelete(svc historyBackend, out io.Writer, id string) error {
	if err := svc.DeleteConversation(id); err != nil {
		return fmt.Errorf("erro ao remover conversa: %w", err)
	}
	_, err := fmt.Fprintf(out, "Conversa %s removida.\n", id)
	return err
}

func init() {
	historyListCmd.Flags().IntVarP(&historyListLimit, "limit", "n", 20, "Número máximo de conversas")
	historyListCmd.Flags().StringVarP(&historyListSearch, "search", "s", "", "Busca full-text nas mensagens")

	historyCmd.AddCommand(historyListCmd)
	historyCmd.AddCommand(historyShowCmd)
	historyCmd.AddCommand(historyDeleteCmd)
}
