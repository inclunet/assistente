# 0045 — Interface CLI como alternativa ao Wails

Autor: Leonardo Gleison Ferreira (Leo) / Assistente
Data: 2026-04-18
Status: em implementação

## Resumo executivo

Adicionar um entrypoint CLI (`cmd/cli/main.go`) que permite usar o assistente via terminal, sem dependência do Wails ou interface gráfica. Para isso, é necessário primeiro reestruturar o projeto: mover a lógica do `App` (hoje em `package main` na raiz) para `internal/app/`, tornando-a importável tanto pelo entrypoint desktop (`cmd/desktop/`) quanto pelo CLI (`cmd/cli/`).

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
cmd/
  desktop/main.go          ← wails.Run() + embed frontend (único lugar com import Wails)
  cli/main.go              ← cobra + adapters CLI (zero Wails)
internal/
  app/                     ← App struct + StartupWithAdapters() + toda orquestração
    app.go                 ← App struct, NewApp(), StartupWithAdapters(), Shutdown()
    app_chat.go            ← métodos de chat
    app_profiles.go        ← métodos de perfis
    ...                    ← demais ~50 arquivos (mesma lógica, package app)
    db.go, llm.go, etc.   ← lógica auxiliar
  ...                      ← packages existentes (chat, llm, speech, etc.)
adapters/
  wails/                   ← adapters Wails (importado apenas por cmd/desktop/)
  cli/                     ← adapters CLI (importado apenas por cmd/cli/)
  noop/                    ← adapters noop (testes)
controllers/               ← inalterado
```

**O que move para `internal/app/`:**
- ~51 arquivos `.go` da raiz (exceto `main.go`)
- Mudança mecânica: `package main` → `package app`
- Zero alteração de lógica — apenas rename de package
- `internal/app/` **não importa Wails** — usa apenas interfaces de `ports`

**O que NÃO move:**
- `main.go` da raiz → vira `cmd/desktop/main.go`
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
cmd/desktop/main.go        ← wails.Run(), embed, startup() wrapper com adapters Wails
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
1. `cmd/desktop/main.go` cria `app.NewApp()`
2. `wails.Run()` chama `startup(ctx)` que injeta adapters Wails
3. `startup()` chama `app.StartupWithAdapters(ctx, wailsEmitter, wailsWindow, wailsDialog)`

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

### Fase 0 — Reestruturação do projeto (pré-requisito)

1. Mover ~51 arquivos `.go` da raiz para `internal/app/` (`package main` → `package app`)
2. Mover `main.go` para `cmd/desktop/main.go` (mantém `package main`, importa `internal/app`)
3. Exportar tipos e funções necessários (ex.: `NewApp()`, `StartupWithAdapters()`, `Shutdown()`)
4. Atualizar `wails.json` para apontar para o novo path de build
5. **Verificação:** `go build ./cmd/desktop` compila; `go test ./...` sem regressões; `wails dev` funciona

### Fase 1 — Adapters CLI e entrypoint

6. Criar package `adapters/cli/` com 3 implementações (Emitter, Window, Dialog) ✅ (já feito)
7. Criar `cmd/cli/main.go` com cobra root command e inicialização via `app.StartupWithAdapters()`
8. **Verificação:** `go build ./cmd/cli` compila; `go test ./...` sem regressões

### Fase 2 — Comandos básicos

9. Implementar `assistente chat "pergunta"` — envia mensagem e imprime resposta streaming
10. Implementar `assistente profiles list|show|activate`
11. Implementar `assistente config get|set`
12. **Verificação:** comandos funcionam end-to-end

### Fase 3 — Interatividade e automação

13. REPL interativo — `assistente chat` sem args
14. Pipe mode — `echo "pergunta" | assistente chat`
15. Ctrl+C cancela geração em andamento

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

- `go build ./cmd/desktop` compila e `wails dev` funciona normalmente
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
