package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
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
	GetConversation(id uint) (*app.Conversation, error)
	GetMessages(conversationID uint, parentID *uint) ([]chat.MessageNode, error)
	DeleteConversation(id uint) error
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
		return runHistoryList(rootApp, os.Stdout, historyListSearch, historyListLimit)
	},
}

func runHistoryList(svc historyBackend, out io.Writer, search string, limit int) error {
	if search != "" {
		results, err := svc.SearchConversationHistory(search, limit)
		if err != nil {
			return fmt.Errorf("erro ao buscar: %w", err)
		}
		if len(results) == 0 {
			fmt.Fprintln(out, "Nenhum resultado encontrado.")
			return nil
		}

		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "CONVERSA\tTÍTULO\tROLE\tTRECHO")
		for _, r := range results {
			snippet := r.Snippet
			if len(snippet) > 60 {
				snippet = snippet[:57] + "..."
			}
			snippet = strings.ReplaceAll(snippet, "\n", " ")
			_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", r.ConversationID, r.ConversationTitle, r.Role, snippet)
		}
		return w.Flush()
	}

	conversations, err := svc.GetConversations()
	if err != nil {
		return fmt.Errorf("erro ao listar conversas: %w", err)
	}
	if len(conversations) == 0 {
		fmt.Fprintln(out, "Nenhuma conversa no histórico.")
		return nil
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
		_, _ = fmt.Fprintf(w, "%d\t%s\t%d\t%s\n", c.ID, c.Title, c.MessageCount, date)
	}
	return w.Flush()
}

// ─── show ───────────────────────────────────────────────────────────────────

var historyShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Exibe mensagens de uma conversa",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("ID inválido: %s", args[0])
		}
		return runHistoryShow(rootApp, os.Stdout, uint(id))
	},
}

func runHistoryShow(svc historyBackend, out io.Writer, id uint) error {
	conv, err := svc.GetConversation(id)
	if err != nil {
		return fmt.Errorf("conversa não encontrada: %w", err)
	}

	fmt.Fprintf(out, "=== %s (ID: %d) ===\n\n", conv.Title, conv.ID)

	messages, err := svc.GetMessages(id, nil)
	if err != nil {
		return fmt.Errorf("erro ao carregar mensagens: %w", err)
	}

	fprintMessageNodes(out, messages)
	return nil
}

func fprintMessageNodes(out io.Writer, nodes []chat.MessageNode) {
	for _, node := range nodes {
		msg := node.Message
		role := strings.ToUpper(msg.Role)
		content := msg.Content
		if len(content) > 500 {
			content = content[:497] + "..."
		}
		fmt.Fprintf(out, "[%s] %s\n\n", role, content)

		if len(node.Children) > 0 {
			fprintMessageNodes(out, node.Children)
		}
	}
}

// ─── delete ─────────────────────────────────────────────────────────────────

var historyDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Remove uma conversa",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("ID inválido: %s", args[0])
		}
		return runHistoryDelete(rootApp, os.Stdout, uint(id))
	},
}

func runHistoryDelete(svc historyBackend, out io.Writer, id uint) error {
	if err := svc.DeleteConversation(id); err != nil {
		return fmt.Errorf("erro ao remover conversa: %w", err)
	}
	fmt.Fprintf(out, "Conversa %d removida.\n", id)
	return nil
}

func init() {
	historyListCmd.Flags().IntVarP(&historyListLimit, "limit", "n", 20, "Número máximo de conversas")
	historyListCmd.Flags().StringVarP(&historyListSearch, "search", "s", "", "Busca full-text nas mensagens")

	historyCmd.AddCommand(historyListCmd)
	historyCmd.AddCommand(historyShowCmd)
	historyCmd.AddCommand(historyDeleteCmd)
}
