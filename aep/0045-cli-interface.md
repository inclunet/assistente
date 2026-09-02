# 0045 — Interface CLI como alternativa ao Wails

Status: In Progress — CLI entregue e testada; validação manual de `wails dev` permanece pendente

Autor: Leonardo Gleison Ferreira (Leo) / Assistente
Data: 2026-04-18

## Resumo executivo

Adicionar um entrypoint CLI (`cmd/asst/main.go`) que permite usar o assistente via terminal, sem dependência do Wails ou interface gráfica. Para isso, foi necessário reestruturar o projeto: mover a lógica do `App` (antes em `package main` na raiz) para `internal/app/`, tornando-a importável tanto pelo entrypoint desktop (raiz) quanto pelo CLI (`cmd/asst/`).

**Nota:** O `main.go` desktop permanece na raiz do projeto (não em `cmd/desktop/`) porque `//go:embed` do Go não permite caminhos com `..` — e `frontend/dist` está na raiz.

A arquitetura de ports/adapters já abstrai ~85% do acoplamento com Wails. O CLI reutiliza integralmente controllers, services e packages internos, substituindo apenas os 3 adapters de UI (Emitter, Window, Dialog) por implementações de terminal.

Casos de uso: automação via scripts, uso em servidores headless, acessibilidade alternativa, integração com pipelines CI/CD, uso SSH remoto.

## Motivação / Problema atual

- O assistente hoje é exclusivamente desktop (Wails + React). Não há como usá-lo sem interface gráfica.
- Usuários que preferem terminal, automação via scripts ou acesso remoto (SSH) não têm opção.
- A arquitetura de ports/adapters (`internal/core/ports/`, `adapters/wails/`, `adapters/noop/`) já foi projetada para desacoplamento, mas só existe um adapter de produção (Wails) e um de teste (noop).
- Servidores headless e containers não podem rodar Wails (requer WebKit/CGO).
- Todo o código do `App` está em `package main` na raiz, o que impede importação por múltiplos entrypoints — restrição do Go.

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
| `app.go` + ~50 `app_*.go` | `package main` na raiz | Não pode ser importado por `cmd/asst/`; precisa mover para `internal/app/` |
| `startup()` | Hardcode `wails.NewEmitterAdapter(ctx)` | Refatorado para `StartupWithAdapters()` que recebe adapters por DI |
| Imports Wails | 3 arquivos: `main.go`, `adapters/wails/*.go` | Isolados, permanecem fora de `internal/app/` |

## Decisões

### Reestruturação: `internal/app/` + `cmd/`

Em Go, `package main` não pode ser importado por outro package. Para manter os
dois entrypoints (desktop em `main.go` na raiz e CLI em `cmd/asst/`), a lógica
do App precisa ficar em um package importável.

**Estrutura resultante:**

```
main.go                    ← wails.Run() + embed frontend (permanece na raiz por //go:embed)
cmd/
  asst/main.go             ← cobra + adapters CLI (zero Wails)
  asst/chat.go             ← subcomando chat (streaming + REPL + pipe)
  asst/profiles.go         ← subcomando profiles (list, show, activate)
  asst/config.go           ← subcomando config (show, providers, model)
internal/
  app/                     ← App struct + StartupWithAdapters() + toda orquestração
    app.go                 ← App struct, NewApp(), StartupWithAdapters(), Shutdown()
    app_chat.go            ← métodos de chat
    app_profiles.go        ← métodos de perfis
    ...                    ← demais ~50 arquivos (mesma lógica, package app)
    db.go, llm.go, etc.   ← lógica auxiliar
  ...                      ← packages existentes (chat, llm, speech, etc.)
adapters/
  wails/                   ← adapters Wails (importado apenas pelo main.go da raiz)
  cli/                     ← adapters CLI (importado apenas por cmd/asst/)
  noop/                    ← adapters noop (testes)
controllers/               ← inalterado
```

**O que moveu para `internal/app/`:**
- ~51 arquivos `.go` da raiz (exceto `main.go`)
- Mudança mecânica: `package main` → `package app`
- Zero alteração de lógica — apenas rename de package
- `internal/app/` **não importa Wails** — usa apenas interfaces de `ports`

**O que NÃO moveu:**
- `main.go` da raiz — permanece na raiz porque `//go:embed all:frontend/dist` requer que o arquivo esteja acima ou no mesmo nível de `frontend/`. Go **proíbe `..`** em caminhos de embed, impedindo `cmd/desktop/main.go` de referenciar `../../frontend/dist`.
- `adapters/` — já está separado
- `controllers/` — já está separado
- `internal/` (sub-packages) — já estão separados
- `frontend/` — não é Go

### CLI library: cobra
- Subcomandos naturais (`assistente chat`, `assistente profiles list`)
- Autocompletion de shell
- Amplamente adotado no ecossistema Go

### Binário separado: `asst`
- Evita carregar dependências de Wails/WebKit/CGO
- Build independente: `go build -o asst ./cmd/asst`

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
main.go (raiz)             ← wails.Run(), embed, OnStartup wrapper com adapters Wails
cmd/asst/main.go            ← cobra root command
cmd/asst/chat.go            ← subcomando chat (REPL + pipe)
cmd/asst/profiles.go        ← subcomando profiles
cmd/asst/config.go          ← subcomando config

internal/app/              ← App struct + StartupWithAdapters() + toda lógica
  app.go
  app_chat.go
  app_profiles.go
  ...

adapters/cli/emitter.go    ← Emitter que imprime no stdout
adapters/cli/window.go     ← WindowPort noop
adapters/cli/dialog.go     ← DialogPort via args/flags
```

### Fluxo de inicialização

**Desktop (Wails):**
1. `main.go` (raiz) cria `app.NewApp()`
2. `wails.Run()` chama `OnStartup(ctx)` que injeta adapters Wails
3. `OnStartup()` chama `app.StartupWithAdapters(ctx, wailsEmitter, wailsWindow, wailsDialog)`

**CLI:**
1. `cmd/asst/main.go` cria `app.NewApp()`
2. `app.StartupWithAdapters(ctx, cliEmitter, cliWindow, cliDialog)`
3. cobra parsea subcomando e delega ao controller apropriado

### CLI Emitter — tradução de eventos para terminal

O `cli.EmitterAdapter` traduz eventos para output formatado:

- `chat:stream` → imprime token a token no stdout (streaming real)
- `chat:done` → newline + flush
- `chat:error` → stderr
- Outros eventos → ignorados (ou log em modo verbose `-v`)

## Fases de implementação

### Fase 0 — Reestruturação do projeto ✅

1. ✅ Mover ~51 arquivos `.go` da raiz para `internal/app/` (`package main` → `package app`)
2. ✅ `main.go` permanece na raiz (restrição do `//go:embed` impede mover para `cmd/desktop/`)
3. ✅ Exportar tipos e funções: `NewApp()`, `StartupWithAdapters()`, `Shutdown()`, `Context()`
4. N/A — `wails.json` não precisa de alteração (main.go continua na raiz)
5. ✅ `go build .` e `go build ./cmd/asst` compilam; `go test ./...` sem regressões

### Fase 1 — Adapters CLI e entrypoint ✅

6. ✅ Criar package `adapters/cli/` com 3 implementações (Emitter, Window, Dialog)
7. ✅ Criar `cmd/asst/main.go` com cobra root command e inicialização via `app.StartupWithAdapters()`
8. ✅ EmitterAdapter com `WaitDone()` para sincronização de streaming
9. ✅ `go build ./cmd/asst` compila; `go test ./...` sem regressões

### Fase 2 — Comandos básicos ✅

10. ✅ `assistente chat "pergunta"` — envia mensagem e imprime resposta streaming
11. ✅ `assistente chat` sem args — modo REPL interativo
12. ✅ `echo "pergunta" | assistente chat` — modo pipe
13. ✅ `assistente profiles list|show|activate`
14. ✅ `assistente config show|providers|model`
15. Flags: `--model`, `--profile`, `--conversation`, `--verbose`

### Fase 3 — Interatividade e automação ✅

16. ✅ Ctrl+C cancela geração em andamento (barge-in via `CancelStreamingForConversation` + SIGINT handler)
17. Descartado — markdown no terminal adiciona dependência pesada; output plain text é suficiente para CLI
18. ✅ Auto-complete de shell (subcomando `completion` para bash, zsh, fish, powershell)

### Fase 4 — Build e distribuição ✅

19. ✅ Task `Go: build cli` no `.vscode/tasks.json`
20. Documentação detalhada no README permanece follow-up opcional; `asst --help`
    e a ajuda dos subcomandos são a referência operacional da CLI.

### Fase 5 — Setup e gerenciamento de providers ✅

Objetivo: permitir que um usuário CLI-only configure o assistente do zero, sem precisar do desktop.

21. `assistente setup` — Wizard interativo de configuração inicial (provider, URL, API key, modelo). Equivalente ao Welcome Wizard do desktop. Usa `NeedsWelcomeWizard()`, `createWizardProvider()`, `validateWizardConnection()`, `saveWelcomeConfig()`.
22. `assistente providers list` — Lista providers LLM com status de conexão
23. `assistente providers add` — Wizard interativo para criar provider (tipo, URL, API key, testa, salva)
24. `assistente providers test <id>` — Testa conexão com provider existente
25. `assistente providers models <id>` — Lista modelos disponíveis no provider
26. `assistente providers default <id>` — Define provider padrão
27. `assistente providers remove <id>` — Remove provider

### Fase 6 — Credenciais e perfis CRUD ✅

28. `assistente credentials list` — Lista credenciais (sem mostrar secrets)
29. `assistente credentials set <pattern>` — Cria/atualiza credencial (lê secret do stdin ou flag `--value`)
30. `assistente credentials remove <pattern>` — Remove credencial
31. `assistente profiles create` — Cria perfil (flags: `--name`, `--model`, `--provider`, `--system-prompt`)
32. `assistente profiles edit <slug>` — Edita campos do perfil via flags
33. `assistente profiles duplicate <slug>` — Duplica perfil
34. `assistente profiles delete <slug>` — Remove perfil

### Fase 7 — MCP, histórico e tools ✅

35. `assistente mcp list` — Lista servidores MCP e status
36. `assistente mcp add <slug>` — Adiciona servidor (flags: `--command`, `--args`, `--env`)
37. `assistente mcp connect <slug>` / `disconnect <slug>` — Gerencia conexão
38. `assistente mcp tools <slug>` — Lista tools do servidor
39. `assistente mcp remove <slug>` — Remove servidor
40. `assistente history list` — Lista conversas recentes
41. `assistente history show <id>` — Exibe mensagens da conversa
42. `assistente history delete <id>` — Remove conversa
43. `assistente tools list` — Lista ferramentas disponíveis (built-in + MCP)

### Evidências das Fases 5–7

Os comandos estão implementados em `cmd/asst/setup.go`, `providers.go`,
`credentials.go`, `profiles.go`, `mcp.go`, `history.go` e `tools.go`, com testes
focados nos arquivos `*_test.go` correspondentes. A ajuda é registrada em
`cmd/asst/main.go`.

### Non-goals para CLI

- TTS/STT (áudio no terminal não faz sentido)
- Editor Monaco (UI-only)
- Workspace/abas (conceito de UI)
- Kanban/Task Lists (complexidade de UI demais para CLI simples)
- Jobs builder visual (arquivos YAML seriam uma AEP separada)
- Aparência/temas (N/A)

## Riscos e mitigações

| Risco | Mitigação |
|---|---|
| Reestruturação quebra build Wails | Fase 0 é puramente mecânica (rename de package); verificar `wails dev` antes de prosseguir |
| `wails.json` aponta para path errado | Atualizar campo de build no `wails.json` |
| Database lock (CLI + Wails simultâneos) | WAL mode no SQLite; documentar uso simultâneo como experimental |
| go-ole/SAPI5 no build CLI | Build tags para excluir em modo CLI se necessário |
| Eventos assíncronos no terminal | CLI Emitter usa mutex para serializar output |
| Cobra como dependência adicional | Padrão no ecossistema Go; impacto mínimo no go.mod |

## Critérios de aceitação

- [x] `go build .` compila o desktop.
- [ ] `wails dev` funciona normalmente — não há execução manual reproduzível
  registrada nesta auditoria.
- [x] `go build ./cmd/asst` compila sem dependências Wails.
- [x] Chat interativo e pipe são cobertos por `cmd/asst/chat_test.go`.
- [x] Perfis são cobertos por `cmd/asst/profiles_test.go`.
- [x] Setup headless é coberto por `cmd/asst/setup_test.go`.
- [x] Providers são cobertos por `cmd/asst/providers_test.go`.
- [x] Credenciais são cobertas por `cmd/asst/credentials_test.go`.
- [x] Pacotes CLI possuem regressões automatizadas focadas.
- [x] `internal/app/` não importa `github.com/wailsapp`.

## Referências

- `internal/core/ports/` — interfaces Emitter, WindowPort, SystemDialogPort
- `adapters/wails/` — implementação Wails (referência)
- `adapters/cli/` — implementação CLI (nova)
- `adapters/noop/` — implementação noop (testes)
- `controllers/` — camada de controllers
- AEP de backend-driven-messaging — contrato de eventos
