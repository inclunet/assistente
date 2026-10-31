package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/llm"
	"assistente/internal/skills"
	"assistente/internal/tools"
	"assistente/internal/tools/filesystem"
)

const maxIterations = 20

type iteration struct {
	text      string
	reasoning string
	calls     []llm.ToolCall
	err       string
	finished  bool
	finish    llm.FinishInfo
}

func (h *iteration) OnChunk(string)                         {}
func (h *iteration) OnThinking(string)                      {}
func (h *iteration) OnThinkingDone(s string)                { h.reasoning = s }
func (h *iteration) OnMCPToolEvent(llm.MCPToolEvent)        {}
func (h *iteration) OnFinishReason(f llm.FinishInfo)        { h.finish = f }
func (h *iteration) OnError(e string)                       { h.err = e }
func (h *iteration) OnDone(s string, _ llm.Usage, _ string) { h.text = s; h.finished = true }
func (h *iteration) OnToolCalls(c []llm.ToolCall, s string, _ llm.Usage, _ string) {
	h.text = s
	h.calls = c
}

// run mantém o histórico somente na execução. Não usa o serviço de chat nem o ledger.
func run(ctx context.Context, streamer llm.Streamer, model, instruction, prompt, dir string) (string, error) {
	return runWithTools(ctx, streamer, model, instruction, prompt, dir, nil)
}

func runWithTools(ctx context.Context, streamer llm.Streamer, model, instruction, prompt, dir string, extra []tools.Tool) (string, error) {
	ctx = tools.WithExecutionContext(ctx, tools.ExecutionContext{Filesystem: &tools.FilesystemScope{Read: []string{filepath.Join(dir, "**")}, Deny: []string{filepath.Join(dir, ".git", "**")}}})
	registry := tools.NewRegistry()
	if err := registry.Register(filesystem.NewReadFile(dir)); err != nil {
		return "", err
	}
	for _, tool := range extra {
		if err := registry.Register(tool); err != nil {
			return "", err
		}
	}
	executor := tools.NewExecutor(registry, tools.DefaultExecutorConfig())
	mcpConfig := tools.DefaultExecutorConfig()
	mcpConfig.RequireCompleteResult = true
	mcpExecutor := tools.NewExecutor(registry, mcpConfig)
	defs := make([]llm.ToolDefinition, 0, 1)
	for _, d := range registry.ToDefinitions() {
		defs = append(defs, llm.ToolDefinition{Type: d.Type, Function: llm.FunctionDefinition{Name: d.Function.Name, Description: d.Function.Description, Parameters: d.Function.Parameters}})
	}
	messages := []llm.Message{{Role: "system", Content: instruction}, {Role: "user", Content: prompt}}
	for i := 0; i < maxIterations; i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		h := &iteration{}
		streamer.StreamChat(ctx, messages, llm.ChatParams{Model: model}, h, defs...)
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if h.err != "" {
			return "", fmt.Errorf("falha do provedor: %s", h.err)
		}
		if h.finish.Reason == llm.FinishReasonMaxTokens || h.finish.Reason == llm.FinishReasonContentFilter || h.finish.Reason == llm.FinishReasonCancelled || h.finish.Reason == llm.FinishReasonOther {
			return "", fmt.Errorf("resposta incompleta: %s", h.finish.Reason)
		}
		if h.finished {
			return h.text, nil
		}
		if len(h.calls) == 0 {
			return "", errors.New("provedor terminou sem resposta nem ferramentas")
		}
		messages = append(messages, llm.Message{Role: "assistant", Content: h.text, ToolCalls: h.calls, ReasoningContent: h.reasoning})
		for _, c := range h.calls {
			if c.ID == "" || c.Function.Name == "" {
				return "", errors.New("chamada de ferramenta inválida")
			}
			selected := executor
			if candidate, ok := registry.Get(c.Function.Name); ok {
				if _, isMCP := tools.MCPIdentityOf(candidate); isMCP {
					selected = mcpExecutor
				}
			}
			result := selected.ExecuteOne(ctx, tools.ToolCall{ID: c.ID, Type: c.Type, Function: tools.FunctionCall{Name: c.Function.Name, Arguments: c.Function.Arguments}})
			content := tools.ContentForModel(result.Result)
			if result.Error != nil {
				content = result.Error.Error()
			}
			messages = append(messages, llm.Message{Role: "tool", ToolCallID: c.ID, Content: content})
		}
	}
	return "", errors.New("limite de iterações atingido")
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "uso: lasst <SKILL.md> <instrução>")
		os.Exit(2)
	}
	if err := execute(os.Args[1], os.Args[2], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func execute(skillPath, prompt string, out io.Writer) error {
	base, model, key := os.Getenv("LASST_BASE_URL"), os.Getenv("LASST_MODEL"), os.Getenv("LASST_API_KEY")
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("LASST_BASE_URL deve ser uma URL HTTPS válida")
	}
	if model == "" || key == "" {
		return errors.New("LASST_MODEL e LASST_API_KEY são obrigatórios")
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	data, err := readSkill(dir, skillPath)
	if err != nil {
		return fmt.Errorf("ler skill: %w", err)
	}
	meta, instruction, err := skills.Parse(string(data))
	if err != nil {
		return fmt.Errorf("skill inválida: %w", err)
	}
	if meta.Filesystem != nil || meta.Network != nil || meta.Tools != nil || meta.AllowedTools != "" || meta.Hooks != nil || meta.MCP != nil || meta.SkillContext != "" || meta.Agent != "" || meta.Behavior != nil || meta.Dependencies != nil || meta.Input != nil || meta.Output != nil || meta.Model != "" {
		return errors.New("skill contém recursos ou políticas ainda não suportados pela PoC")
	}
	if strings.TrimSpace(instruction) == "" || strings.TrimSpace(prompt) == "" {
		return errors.New("skill e instrução não podem estar vazias")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 10*time.Minute)
	defer timeout()
	mode := llm.ReasoningContentMode(strings.TrimSpace(os.Getenv("LASST_REASONING_CONTENT_MODE")))
	if mode != "" && mode != llm.ReasoningContentDisabled && mode != llm.ReasoningContentReplayWithTools {
		return errors.New("LASST_REASONING_CONTENT_MODE deve ser disabled ou replay_with_tools")
	}
	provider := llm.NewOpenAIProviderWithAPIKey(&llm.ProviderConfig{BaseURL: base, ReasoningContentMode: mode}, key)
	mcpTools, closeMCP, err := openGitHubMCP(ctx, dir)
	if err != nil {
		return err
	}
	defer closeMCP()
	answer, err := runWithTools(ctx, provider, model, instruction, prompt, dir, mcpTools)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, answer)
	return err
}
