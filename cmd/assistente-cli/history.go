package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"assistente/internal/chat"

	"github.com/spf13/cobra"
)

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
		if historyListSearch != "" {
			results, err := rootApp.SearchConversationHistory(historyListSearch, historyListLimit)
			if err != nil {
				return fmt.Errorf("erro ao buscar: %w", err)
			}
			if len(results) == 0 {
				fmt.Println("Nenhum resultado encontrado.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "CONVERSA\tTÍTULO\tROLE\tTRECHO")
			for _, r := range results {
				snippet := r.Snippet
				if len(snippet) > 60 {
					snippet = snippet[:57] + "..."
				}
				snippet = strings.ReplaceAll(snippet, "\n", " ")
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", r.ConversationID, r.ConversationTitle, r.Role, snippet)
			}
			return w.Flush()
		}

		conversations, err := rootApp.GetConversations()
		if err != nil {
			return fmt.Errorf("erro ao listar conversas: %w", err)
		}
		if len(conversations) == 0 {
			fmt.Println("Nenhuma conversa no histórico.")
			return nil
		}

		limit := historyListLimit
		if limit <= 0 || limit > len(conversations) {
			limit = len(conversations)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tTÍTULO\tMENSAGENS\tDATA")
		for i := 0; i < limit; i++ {
			c := conversations[i]
			date := c.UpdatedAt.Format(time.DateOnly)
			fmt.Fprintf(w, "%d\t%s\t%d\t%s\n", c.ID, c.Title, c.MessageCount, date)
		}
		return w.Flush()
	},
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

		conv, err := rootApp.GetConversation(uint(id))
		if err != nil {
			return fmt.Errorf("conversa não encontrada: %w", err)
		}

		fmt.Printf("=== %s (ID: %d) ===\n\n", conv.Title, conv.ID)

		messages, err := rootApp.GetMessages(uint(id), nil)
		if err != nil {
			return fmt.Errorf("erro ao carregar mensagens: %w", err)
		}

		printMessageNodes(messages)
		return nil
	},
}

func printMessageNodes(nodes []chat.MessageNode) {
	for _, node := range nodes {
		msg := node.Message
		role := strings.ToUpper(msg.Role)
		content := msg.Content
		if len(content) > 500 {
			content = content[:497] + "..."
		}
		fmt.Printf("[%s] %s\n\n", role, content)

		if len(node.Children) > 0 {
			printMessageNodes(node.Children)
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

		if err := rootApp.DeleteConversation(uint(id)); err != nil {
			return fmt.Errorf("erro ao remover conversa: %w", err)
		}
		fmt.Printf("Conversa %d removida.\n", id)
		return nil
	},
}

func init() {
	historyListCmd.Flags().IntVarP(&historyListLimit, "limit", "n", 20, "Número máximo de conversas")
	historyListCmd.Flags().StringVarP(&historyListSearch, "search", "s", "", "Busca full-text nas mensagens")

	historyCmd.AddCommand(historyListCmd)
	historyCmd.AddCommand(historyShowCmd)
	historyCmd.AddCommand(historyDeleteCmd)
}
