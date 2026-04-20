package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"assistente/internal/app"

	"github.com/spf13/cobra"
)

var (
	chatConversationID uint
	chatModel          string
	chatProfile        string
)

var chatCmd = &cobra.Command{
	Use:   "chat [mensagem]",
	Short: "Envia uma mensagem e recebe resposta via streaming",
	Long: `Envia uma mensagem ao assistente e imprime a resposta via streaming no stdout.

Exemplos:
  assistente chat "Qual a capital da França?"
  assistente chat --model gpt-4o "Explique recursão"
  echo "Resuma este texto" | assistente chat
  assistente chat  (modo interativo)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		message := strings.Join(args, " ")

		// Se não há args, tenta ler do stdin (pipe mode)
		if message == "" {
			if !isTerminal(os.Stdin) {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("erro ao ler stdin: %w", err)
				}
				message = strings.TrimSpace(string(data))
			}
		}

		// Se ainda não tem mensagem, entra no modo interativo (REPL)
		if message == "" {
			return runREPL()
		}

		return sendAndWait(message)
	},
}

func init() {
	chatCmd.Flags().UintVar(&chatConversationID, "conversation", 0, "ID da conversa (0 = nova conversa)")
	chatCmd.Flags().StringVar(&chatModel, "model", "", "Modelo LLM a usar (ex: gpt-4o)")
	chatCmd.Flags().StringVar(&chatProfile, "profile", "", "Slug do perfil a usar")
}

// sendAndWait envia uma mensagem e bloqueia até o streaming terminar.
// Ctrl+C durante a geração cancela o streaming (barge-in) sem encerrar o processo.
func sendAndWait(message string) error {
	conv, err := ensureConversation()
	if err != nil {
		return err
	}

	params := app.ChatParams{}
	if chatModel != "" {
		params.Model = chatModel
	}
	if chatProfile != "" {
		params.ProfileSlug = chatProfile
	}

	// Prepara o canal de sincronização ANTES de enviar
	done := cliEmitter.WaitDone()

	_, err = rootApp.SendMessage(conv.ID, message, "", params)
	if err != nil {
		return fmt.Errorf("erro ao enviar mensagem: %w", err)
	}

	// Intercepta Ctrl+C para cancelar a geração (barge-in) sem matar o processo
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)

	// Aguarda fim do streaming ou cancelamento
	select {
	case <-done:
		return nil
	case <-sigCh:
		rootApp.CancelStreamingForConversation(conv.ID)
		fmt.Fprintln(os.Stderr, "\n(geração cancelada)")
		// Aguarda o done do emitter para garantir que o streaming encerrou
		<-done
		return nil
	case <-rootApp.Context().Done():
		return fmt.Errorf("cancelado")
	}
}

// ensureConversation obtém ou cria uma conversa para o chat.
func ensureConversation() (*app.Conversation, error) {
	if chatConversationID > 0 {
		conv, err := rootApp.GetConversation(chatConversationID)
		if err != nil {
			return nil, fmt.Errorf("conversa %d não encontrada: %w", chatConversationID, err)
		}
		return conv, nil
	}
	conv, err := rootApp.EnsureConversation("CLI")
	if err != nil {
		return nil, fmt.Errorf("erro ao criar conversa: %w", err)
	}
	// Armazena para reutilizar em modo REPL
	chatConversationID = conv.ID
	return conv, nil
}

// runREPL inicia o modo interativo de chat.
func runREPL() error {
	fmt.Fprintln(os.Stderr, "Modo interativo. Digite sua mensagem (Ctrl+C para sair).")
	fmt.Fprintln(os.Stderr, "---")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB para mensagens grandes
	for {
		fmt.Fprint(os.Stderr, "> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := sendAndWait(line); err != nil {
			fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
		}
		fmt.Fprintln(os.Stderr, "---")
	}
	return scanner.Err()
}

// isTerminal verifica se o file descriptor é um terminal (não é pipe).
func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
