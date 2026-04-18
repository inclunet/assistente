# 0045 — Interface CLI como alternativa ao Wails

Autor: Leonardo Gleison Ferreira (Leo) / Assistente
Data: 2026-04-18
Status: rascunho

## Resumo executivo

Adicionar um entrypoint CLI (`cmd/cli/main.go`) que permite usar o assistente via terminal, sem dependência do Wails ou interface gráfica. A arquitetura atual de ports/adapters já abstrai ~85% do acoplamento com Wails — o CLI reutiliza integralmente controllers, services e packages internos, substituindo apenas os 3 adapters de UI (Emitter, Window, Dialog) por implementações de terminal.

Casos de uso: automação via scripts, uso em servidores headless, acessibilidade alternativa, integração com pipelines CI/CD, uso SSH remoto.

## Motivação / Problema atual

- O assistente hoje é exclusivamente desktop (Wails + React). Não há como usá-lo sem interface gráfica.
- Usuários que preferem terminal, automação via scripts ou acesso remoto (SSH) não têm opção.
- A arquitetura de ports/adapters (`internal/core/ports/`, `adapters/wails/`, `adapters/noop/`) já foi projetada para desacoplamento, mas só existe um adapter de produção (Wails) e um de teste (noop).
- Servidores headless e containers não podem rodar Wails (requer WebKit/CGO).

## Análise da arquitetura atual

### O que já está abstraído ✅

| Componente | Abstração | Localização |
|---|---|---|
| Emissão de eventos | `ports.Emitter` interface | `internal/core/ports/emitter.go` |
| Diálogos de arquivo | `ports.SystemDialogPort` interface | `internal/core/ports/dialog.go` |
| Controle de janela | `ports.WindowPort` interface | `internal/core/ports/window.go` |
| Business logic | 30+ packages em `internal/` | Zero imports de Wails |
| Controllers | 20 controllers com DI | `controllers/` — sem ref ao App struct |
| Database | SQLite via `internal/database` | Framework-agnostic |
| LLM/TTS/STT | Services puros | `internal/llm/`, `internal/speech/`, `internal/agent/` |

### O que está acoplado ❌

| Componente | Acoplamento | Impacto |
|---|---|---|
| `main.go` | 100% Wails (`wails.Run`) | Precisa de novo entrypoint |
| `app.go` startup() | Hardcode `wails.NewEmitterAdapter(ctx)` | Refatorar para DI |
| Imports Wails | 3 arquivos: `main.go`, `adapters/wails/*.go` | Isolados, não afetam CLI |

## Decisões

### CLI library: cobra
- Subcomandos naturais (`assistente chat`, `assistente profiles list`)
- Autocompletion de shell
- Amplamente adotado no ecossistema Go
- Alternativa descartada: `flag` stdlib (sem suporte ergonômico a subcomandos)

### Binário separado: `assistente-cli`
- Evita carregar dependências de Wails/WebKit/CGO
- Build independente: `go build -o assistente-cli ./cmd/cli`
- Build tags para excluir `sapi5_windows.go` e outras deps gráficas

### Escopo incluído
- Chat interativo (REPL) e não-interativo (pipe/args)
- Listagem e ativação de perfis
- Leitura e escrita de configurações
- Streaming de resposta LLM no terminal

### Escopo excluído (non-goals)
- TTS/STT via CLI (complexidade de áudio no terminal)
- Workspace/editor features (requer UI)
- Diálogos nativos de arquivo (substituídos por flags)
- HTTP/REST API server (pode ser AEP futura separada)

## Arquitetura proposta

```
cmd/cli/main.go           ← novo entrypoint (cobra root command)
cmd/cli/chat.go            ← subcomando chat (REPL + pipe)
cmd/cli/profiles.go        ← subcomando profiles
cmd/cli/config.go          ← subcomando config

adapters/cli/emitter.go    ← Emitter que imprime no stdout
adapters/cli/window.go     ← WindowPort noop
adapters/cli/dialog.go     ← DialogPort via args/flags

app.go                     ← startup() refatorado para aceitar adapters por DI
```

### Fluxo de inicialização CLI

1. `cmd/cli/main.go` cria `context.Background()` (sem Wails)
2. Instancia `App` via `NewApp()`
3. Chama `app.StartupWithAdapters(ctx, cliEmitter, cliWindow, cliDialog)`
4. Toda a inicialização padrão roda (DB, services, controllers)
5. cobra parsea subcomando e delega ao controller apropriado

### CLI Emitter — tradução de eventos para terminal

O `cli.EmitterAdapter` escuta eventos e traduz para output formatado:

- `chat:stream` → imprime token a token no stdout (streaming real)
- `chat:done` → newline + flush
- `chat:error` → stderr
- Outros eventos → ignorados (ou log em modo verbose `-v`)

## Fases de implementação

### Fase 1 — Infraestrutura (adapters + startup)

1. Criar package `adapters/cli/` com 3 implementações (Emitter, Window, Dialog)
2. Refatorar `app.go` — extrair `StartupWithAdapters(ctx, emitter, window, dialog)` a partir do `startup()` existente, mantendo `startup()` como wrapper que injeta adapters Wails
3. Criar `cmd/cli/main.go` com cobra root command e inicialização básica
4. **Verificação:** `go build ./cmd/cli` compila; `go test ./...` sem regressões

### Fase 2 — Comandos básicos

5. Implementar `assistente chat "pergunta"` — envia mensagem via `chatController` e imprime resposta streaming no terminal
6. Implementar `assistente profiles list|show|activate` — CRUD via `profilesController`
7. Implementar `assistente config get|set` — via `settingsController`
8. **Verificação:** comandos funcionam end-to-end com provedor LLM configurado

### Fase 3 — Interatividade e automação

9. REPL interativo — `assistente chat` sem args entra em modo conversacional (stdin → mensagem → stdout streaming)
10. Pipe mode — `echo "pergunta" | assistente chat` para automação/scripting; detecta stdin não-interativo
11. Ctrl+C cancela geração em andamento (context cancellation)
12. **Verificação:** REPL e pipe funcionam corretamente

### Fase 4 — Build e distribuição

13. Build tags para excluir deps gráficas (`//go:build !cli` nos arquivos SAPI5/go-ole)
14. Task no `.vscode/tasks.json`: `Go: build cli`
15. Documentação no README

## Riscos e mitigações

| Risco | Mitigação |
|---|---|
| Database lock (CLI + Wails simultâneos) | WAL mode no SQLite; documentar uso simultâneo como experimental |
| go-ole/SAPI5 no build CLI | Build tags `//go:build !cli` nos arquivos que usam SAPI5 |
| Eventos assíncronos no terminal | CLI Emitter usa channel interno para serializar output |
| Cobra como dependência adicional | Padrão no ecossistema Go; impacto mínimo no go.mod |
| Contexto Wails vs Go genérico | `a.ctx` já é `context.Context` padrão; CLI usa `context.Background()` |

## Critérios de aceitação

- `go build ./cmd/cli` compila sem erros e sem deps Wails
- `assistente-cli chat "olá"` retorna resposta streaming no terminal
- `echo "olá" | assistente-cli chat` funciona em modo pipe
- `assistente-cli profiles list` lista perfis corretamente
- `go test ./adapters/cli/...` passa
- `go test ./...` sem regressões — build Wails continua funcionando
- Build Wails separado (`wails build`) não é afetado

## Referências

- `internal/core/ports/emitter.go` — interface Emitter
- `internal/core/ports/window.go` — interface WindowPort
- `internal/core/ports/dialog.go` — interface SystemDialogPort
- `adapters/wails/` — implementação Wails (referência)
- `adapters/noop/ui.go` — implementação noop (referência)
- `controllers/chat_controller.go` — controller de chat
- AEP 0040 — backend-driven messaging (contrato de eventos)
