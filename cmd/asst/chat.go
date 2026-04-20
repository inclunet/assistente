package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"assistente/internal/app"

	"github.com/spf13/cobra"
)

// chatBackend abstracts the app methods used by chat commands (enables testing).
type chatBackend interface {
	SendMessage(conversationID uint, userContent, userMedia string, params app.ChatParams) (uint, error)
	EnsureConversation(title string) (*app.Conversation, error)
	GetConversation(id uint) (*app.Conversation, error)
	CancelStreamingForConversation(conversationID uint)
	Context() context.Context
}

// waitDoner abstracts the WaitDone method from the emitter.
type waitDoner interface {
	WaitDone() <-chan struct{}
}

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
  asst chat "Qual a capital da França?"
  asst chat --model gpt-4o "Explique recursão"
  echo "Resuma este texto" | asst chat
  asst chat  (modo interativo)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		message := strings.Join(args, " ")

		// Se não há args, tenta ler do stdin (pipe mode)
		if message == "" {
			if !isTerminal(os.Stdin) {
				const maxInputSize = 512 * 1024 // 512KB — alinhado com limite do backend
				data, err := io.ReadAll(io.LimitReader(os.Stdin, maxInputSize+1))
				if err != nil {
					return fmt.Errorf("erro ao ler stdin: %w", err)
				}
				if len(data) > maxInputSize {
					return fmt.Errorf("entrada excede limite de %d KB", maxInputSize/1024)
				}
				message = strings.TrimSpace(string(data))
			}
		}

		// Se ainda não tem mensagem, entra no modo interativo (REPL)
		if message == "" {
			return runREPL(rootApp, cliEmitter)
		}

		return sendAndWait(rootApp, cliEmitter, message)
	},
}

func init() {
	chatCmd.Flags().UintVar(&chatConversationID, "conversation", 0, "ID da conversa (0 = nova conversa)")
	chatCmd.Flags().StringVar(&chatModel, "model", "", "Modelo LLM a usar (ex: gpt-4o)")
	chatCmd.Flags().StringVar(&chatProfile, "profile", "", "Slug do perfil a usar")
}

// sendAndWait envia uma mensagem e bloqueia até o streaming terminar.
// Ctrl+C durante a geração cancela o streaming (barge-in) sem encerrar o processo.
func sendAndWait(svc chatBackend, emitter waitDoner, message string) error {
	conv, err := ensureConversation(svc)
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
	done := emitter.WaitDone()

	// Intercepta Ctrl+C para cancelar a geração (barge-in) sem matar o processo.
	// Registrado ANTES de SendMessage para cobrir latência de rede.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	defer signal.Stop(sigCh)

	_, err = svc.SendMessage(conv.ID, message, "", params)
	if err != nil {
		return fmt.Errorf("erro ao enviar mensagem: %w", err)
	}

	// Aguarda fim do streaming ou cancelamento
	select {
	case <-done:
		return nil
	case <-sigCh:
		svc.CancelStreamingForConversation(conv.ID)
		_, _ = fmt.Fprintln(os.Stderr, "\n(geração cancelada)")
		// Aguarda o done do emitter para garantir que o streaming encerrou
		<-done
		return nil
	case <-svc.Context().Done():
		return fmt.Errorf("cancelado")
	}
}

// ensureConversation obtém ou cria uma conversa para o chat.
func ensureConversation(svc chatBackend) (*app.Conversation, error) {
	if chatConversationID > 0 {
		conv, err := svc.GetConversation(chatConversationID)
		if err != nil {
			return nil, fmt.Errorf("conversa %d não encontrada: %w", chatConversationID, err)
		}
		return conv, nil
	}
	conv, err := svc.EnsureConversation("CLI")
	if err != nil {
		return nil, fmt.Errorf("erro ao criar conversa: %w", err)
	}
	// Armazena para reutilizar em modo REPL
	chatConversationID = conv.ID
	return conv, nil
}

// runREPL inicia o modo interativo de chat.
func runREPL(svc chatBackend, emitter waitDoner) error {
	_, _ = fmt.Fprintln(os.Stderr, "Modo interativo. Ctrl+C durante a geração cancela a resposta; Ctrl+D para sair.")
	_, _ = fmt.Fprintln(os.Stderr, "---")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024) // 1MB para mensagens grandes
	for {
		_, _ = fmt.Fprint(os.Stderr, "> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := sendAndWait(svc, emitter, line); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Erro: %v\n", err)
		}
		_, _ = fmt.Fprintln(os.Stderr, "---")
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
