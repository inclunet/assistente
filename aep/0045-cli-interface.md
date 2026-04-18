# 0045 — Interface CLI como alternativa ao Wails

Autor: Leonardo Gleison Ferreira (Leo) / Assistente
Data: 2026-04-18
Status: em implementação

## Resumo executivo

Adicionar um entrypoint CLI (`cmd/cli/main.go`) que permite usar o assistente via terminal, sem dependência do Wails ou interface gráfica. Para isso, foi necessário reestruturar o projeto: mover a lógica do `App` (antes em `package main` na raiz) para `internal/app/`, tornando-a importável tanto pelo entrypoint desktop (raiz) quanto pelo CLI (`cmd/cli/`).

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
| `app.go` + ~50 `app_*.go` | `package main` na raiz | Não pode ser importado por `cmd/cli/`; precisa mover para `internal/app/` |
| `startup()` | Hardcode `wails.NewEmitterAdapter(ctx)` | Refatorado para `StartupWithAdapters()` que recebe adapters por DI |
| Imports Wails | 3 arquivos: `main.go`, `adapters/wails/*.go` | Isolados, permanecem fora de `internal/app/` |

## Decisões

### Reestruturação: `internal/app/` + `cmd/`

Em Go, `package main` não pode ser importado por outro package. Para ter dois entrypoints (`cmd/desktop/` e `cmd/cli/`), a lógica do App precisa ser movida para um package importável.

**Estrutura resultante:**

```
main.go                    ← wails.Run() + embed frontend (permanece na raiz por //go:embed)
cmd/
  cli/main.go              ← cobra + adapters CLI (zero Wails)
  cli/chat.go              ← subcomando chat (streaming + REPL + pipe)
  cli/profiles.go          ← subcomando profiles (list, show, activate)
  cli/config.go            ← subcomando config (show, providers, model)
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
  cli/                     ← adapters CLI (importado apenas por cmd/cli/)
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

### Binário separado: `assistente-cli`
- Evita carregar dependências de Wails/WebKit/CGO
- Build independente: `go build -o assistente-cli ./cmd/cli`

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
cmd/cli/main.go            ← cobra root command
cmd/cli/chat.go            ← subcomando chat (REPL + pipe)
cmd/cli/profiles.go        ← subcomando profiles
cmd/cli/config.go          ← subcomando config

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
1. `cmd/cli/main.go` cria `app.NewApp()`
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
5. ✅ `go build .` e `go build ./cmd/cli` compilam; `go test ./...` sem regressões

### Fase 1 — Adapters CLI e entrypoint ✅

6. ✅ Criar package `adapters/cli/` com 3 implementações (Emitter, Window, Dialog)
7. ✅ Criar `cmd/cli/main.go` com cobra root command e inicialização via `app.StartupWithAdapters()`
8. ✅ EmitterAdapter com `WaitDone()` para sincronização de streaming
9. ✅ `go build ./cmd/cli` compila; `go test ./...` sem regressões

### Fase 2 — Comandos básicos ✅

10. ✅ `assistente chat "pergunta"` — envia mensagem e imprime resposta streaming
11. ✅ `assistente chat` sem args — modo REPL interativo
12. ✅ `echo "pergunta" | assistente chat` — modo pipe
13. ✅ `assistente profiles list|show|activate`
14. ✅ `assistente config show|providers|model`
15. Flags: `--model`, `--profile`, `--conversation`, `--verbose`

### Fase 3 — Interatividade e automação

16. Ctrl+C cancela geração em andamento (cancelamento gracioso via context)
17. Output formatado com markdown renderizado no terminal (opcional)
18. Auto-complete de shell (cobra built-in)

### Fase 4 — Build e distribuição

16. Task no `.vscode/tasks.json`: `Go: build cli`
17. Documentação no README

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

- `go build .` (desktop) compila e `wails dev` funciona normalmente
- `go build ./cmd/cli` compila sem deps Wails
- `assistente-cli chat "olá"` retorna resposta streaming no terminal
- `echo "olá" | assistente-cli chat` funciona em modo pipe
- `assistente-cli profiles list` lista perfis corretamente
- `go test ./...` sem regressões
- `internal/app/` não importa Wails (zero `github.com/wailsapp`)

## Referências

- `internal/core/ports/` — interfaces Emitter, WindowPort, SystemDialogPort
- `adapters/wails/` — implementação Wails (referência)
- `adapters/cli/` — implementação CLI (nova)
- `adapters/noop/` — implementação noop (testes)
- `controllers/` — camada de controllers
- AEP de backend-driven-messaging — contrato de eventos
