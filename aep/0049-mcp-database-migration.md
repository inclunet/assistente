# AEP-0049 — Migração de MCP Servers para Banco de Dados

## Dependências

- **AEP-0046** (Migração de IDs sequenciais para UUIDv7): Deve ser implementada primeiro. Fornece o `UUIDModel` com hook `BeforeCreate` que gera UUIDv7 automaticamente. Todas as PKs das tabelas desta AEP usam esse modelo.

## Resumo

Migrar a configuração de servidores MCP de arquivos JSON individuais no disco (`~/.assistente/mcp/{slug}.json`) para SQLite via GORM. Uma tabela `mcp_servers` armazena a configuração persistente e uma tabela `mcp_server_logs` registra eventos de conexão, erros e health checks. Credenciais OAuth/bearer/basic continuam no `credentials.Manager` existente (nunca no banco de configs). O file watcher é removido; o catálogo de tools dos jobs permanece em disco.

## Motivação

1. **Consistência**: Após AEP-0048 (jobs) migrar para banco, MCP configs seriam o único recurso complexo ainda em filesystem. Unificar tudo no SQLite simplifica backup, restore e o futuro import/export (AEP-0047).

2. **Atomicidade**: Salvar config JSON + reconectar servidor não é atômico. Um crash entre "escrever arquivo" e "registrar no runtime" pode deixar estado inconsistente. Com GORM + SQLite WAL, operações são transacionais.

3. **Queries**: Listar servidores por transport, auth_type, status ou filtrar por enabled/auto_connect requer parse de N arquivos. No banco, é uma query SQL.

4. **Logs de conexão**: Não existe histórico de conexões, erros ou health check failures. A tabela `mcp_server_logs` viabiliza diagnóstico sem precisar buscar em logs do sistema.

5. **Multi-diretório eliminado**: O sistema atual resolve configs em 3 diretórios (exe, home, cwd) com prioridade. Essa complexidade é eliminada — o banco é a única fonte de verdade.

6. **Preparação para AEP-0047**: O export/import precisa acessar configs MCP de forma uniforme. Com banco, segue o mesmo pattern de Repository das demais entidades.

## Estado atual

Cada servidor MCP é **um arquivo JSON** com nome = slug:

```
~/.assistente/mcp/
├── filesystem.json
├── github.json
├── jira.json
└── ...
```

- Resolução em 3 diretórios via `configdir.Resolver` (exe → home → cwd)
- Hot reload via `fsnotify.Watcher` (debounce 500ms, cooldown 2s para escritas próprias)
- Smart defaults no parse: transport inferido, enabled/auto_connect default true
- Credenciais em `credentials.Manager` por patterns: `mcp-client:{slug}`, `mcp-tokens:{slug}`, hostname
- OAuth auto-discovery via RFC 9470 + RFC 8414
- Estado runtime em `ServerStatus` (in-memory, não persistido)
- 18 funções Wails expostas para gestão de MCP

### Struct `ServerConfig` (persistida em JSON hoje)

| Campo | Tipo | Descrição |
|---|---|---|
| `name` | string | Nome legível |
| `description` | string | Descrição |
| `transport` | string | `stdio` / `sse` / `streamable` |
| `command` | string | Comando para stdio |
| `args` | []string | Argumentos para stdio |
| `env` | map[string]string | Variáveis de ambiente |
| `url` | string | URL para sse/streamable |
| `auth_type` | string | `none` / `bearer` / `basic` / `oauth2_client_credentials` / `oauth2_pkce` |
| `oauth2_client_id` | string | Client ID OAuth |
| `oauth2_auth_url` | string | Authorization endpoint |
| `oauth2_token_url` | string | Token endpoint |
| `oauth2_scopes` | []string | Scopes OAuth |
| `oauth2_callback_port` | int | Porta do callback local |
| `oauth2_callback_host` | string | Host do callback |
| `oauth2_registration_url` | string | Dynamic client registration |
| `oauth2_device_auth_url` | string | Device authorization endpoint |
| `disable_sse` | bool | Desabilitar SSE fallback |
| `prefer_bridge` | bool | Forçar adapter em vez de nativo |
| `enabled` | bool | Se o servidor está ativo |
| `auto_connect` | bool | Conectar automaticamente ao iniciar |

## Decisões

### D1 — Migração total para banco (sem dual JSON+banco)

Os arquivos JSON no disco são completamente substituídos pelo banco SQLite. Não há modo dual. O file watcher (`WatchConfigs`) é removido.

### D2 — Slug obrigatório e único

O slug (nome do arquivo JSON sem extensão, ex: `github`) vira coluna `slug` com constraint `UNIQUE NOT NULL`. O PK `id` é UUIDv7 (via `UUIDModel` da AEP-0046).

O slug é o identificador usado em:
- API Wails (frontend referencia servidores por slug)
- Namespacing de tools MCP: `mcp_{slug}__{toolName}`
- Credential patterns: `mcp-client:{slug}`, `mcp-tokens:{slug}`
- Eventos Wails: `mcp:server_connected` etc. carregam slug
- Jobs: campo `tool` referencia tools MCP pelo nome completo

### D3 — Credenciais NÃO migram para esta tabela

Credenciais (tokens OAuth, bearer tokens, passwords) continuam no `credentials.Manager` existente, que já cuida de criptografia com DEK/master password. A tabela `mcp_servers` armazena apenas metadados de autenticação não sensíveis:

| O que fica na tabela | O que fica no credentials.Manager |
|---|---|
| `auth_type` | Tokens (access, refresh) |
| `oauth2_client_id` | Client secret |
| `oauth2_auth_url`, `oauth2_token_url` | Bearer token |
| `oauth2_scopes` | Username/password |
| `oauth2_callback_port/host` | |
| `oauth2_registration_url` | |
| `oauth2_device_auth_url` | |

### D4 — Configs complexas em JSON

Campos estruturados são serializados como JSON TEXT:

| Coluna | Tipo Go |
|---|---|
| `args` | `[]string` |
| `env` | `map[string]string` |
| `oauth2_scopes` | `[]string` |

### D5 — Tabela de logs de conexão

Uma tabela `mcp_server_logs` registra eventos do lifecycle do servidor:

- Conexões bem-sucedidas
- Erros de conexão (com mensagem)
- Health check failures
- Reconexões
- Desconexões

Isso permite diagnóstico sem depender de logs do sistema operacional. Retenção: 30 dias (mesma política da AEP-0048).

### D6 — Frontend inalterado

A API Wails (`ListMCPServers`, `SaveMCPServer`, `ConnectMCPServer`, etc.) mantém as mesmas assinaturas e tipos de retorno. O frontend não percebe a mudança de backing store.

### D7 — Migração one-time de filesystem para banco

Na primeira execução após a atualização, o Manager detecta se existem arquivos JSON em `~/.assistente/mcp/` E a tabela `mcp_servers` está vazia. Se sim:

1. Carrega todos os `.json` dos 3 diretórios (com resolução de prioridade)
2. Insere como registros no banco (slug = nome do arquivo sem extensão)
3. Renomeia diretório principal para `~/.assistente/mcp.migrated/` (backup)

A migração é idempotente: se `mcp.migrated/` já existe, pula. Credenciais não são tocadas — já estão no `credentials.Manager`.

### D8 — Repository pattern

A persistência é abstraída por uma interface `Repository`:

```go
type MCPRepository interface {
    ListServers() ([]ServerConfig, error)
    GetServer(slug string) (*ServerConfig, error)
    GetServerByID(id string) (*ServerConfig, error)
    SaveServer(cfg *ServerConfig) error
    DeleteServer(slug string) error
    DuplicateServer(slug, newSlug string) (*ServerConfig, error)
    LogEvent(entry *MCPServerLog) error
    GetLogs(slug string, limit int) ([]MCPServerLog, error)
    CleanOldLogs(maxAge time.Duration) (int, error)
}
```

Implementação concreta: `DBMCPRepository` que recebe `*gorm.DB`. O Manager recebe a interface (testável com mocks).

### D9 — Smart defaults preservados

Os smart defaults atuais do `ParseServerConfig` são preservados na camada de aplicação (não no banco):

- Transport inferido de URL/command
- `enabled` e `auto_connect` default `true`
- `auth_type` inferido dos campos OAuth

Esses defaults são aplicados ao criar um novo servidor, antes de persistir. No banco, os campos são explícitos.

### D10 — Campos `env` podem conter dados sensíveis

O campo `env` (variáveis de ambiente) pode conter API keys ou tokens. Diferente das credenciais OAuth/bearer (que ficam no `credentials.Manager`), `env` é parte da config do servidor e fica no banco local.

Isso é aceitável porque:
- O banco SQLite é local ao dispositivo do usuário
- `env` é necessário para inicializar processos stdio (precisa estar disponível sem master password)
- A AEP-0047 (export) já documenta que `env` é incluído com warning na UI

### D11 — Multi-diretório eliminado

O sistema de resolução em 3 diretórios (exe → home → cwd) é eliminado. O banco é a única fonte de verdade. Configs que estavam em diretórios de menor prioridade são mescladas na migração one-time com precedência correta.

## Tabelas

### mcp_servers

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 via `UUIDModel.BeforeCreate` |
| `slug` | TEXT | UNIQUE NOT NULL, INDEX | Ex: `github`, `filesystem` |
| `name` | TEXT | NOT NULL | Nome legível |
| `description` | TEXT | | Opcional |
| `transport` | TEXT | NOT NULL | `stdio` / `sse` / `streamable` |
| `command` | TEXT | | Comando para stdio |
| `args` | TEXT | | JSON array: `["arg1","arg2"]` |
| `env` | TEXT | | JSON object: `{"KEY":"value"}` |
| `url` | TEXT | | URL para sse/streamable |
| `auth_type` | TEXT | NOT NULL, DEFAULT 'none' | `none`/`bearer`/`basic`/`oauth2_client_credentials`/`oauth2_pkce` |
| `oauth2_client_id` | TEXT | | Client ID (não é secret) |
| `oauth2_auth_url` | TEXT | | Authorization endpoint |
| `oauth2_token_url` | TEXT | | Token endpoint |
| `oauth2_scopes` | TEXT | | JSON array: `["scope1","scope2"]` |
| `oauth2_callback_port` | INT | | Porta do callback local |
| `oauth2_callback_host` | TEXT | | Host do callback |
| `oauth2_registration_url` | TEXT | | Dynamic client registration |
| `oauth2_device_auth_url` | TEXT | | Device authorization endpoint |
| `disable_sse` | BOOL | DEFAULT false | |
| `prefer_bridge` | BOOL | DEFAULT false | |
| `enabled` | BOOL | NOT NULL, DEFAULT true | |
| `auto_connect` | BOOL | NOT NULL, DEFAULT true | |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

### mcp_server_logs

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 |
| `server_id` | TEXT | FK→mcp_servers.id, INDEX | |
| `timestamp` | DATETIME | NOT NULL, INDEX | Momento do evento |
| `type` | TEXT | NOT NULL, INDEX | `connected`/`disconnected`/`error`/`health_fail`/`reconnecting`/`config_changed` |
| `message` | TEXT | | Descrição legível |
| `data` | TEXT | | JSON: dados contextuais (ex: error details, tool count) |
| `created_at` | DATETIME | | |

## Mapeamento de dados: filesystem → banco

### ServerConfig (JSON → tabela `mcp_servers`)

| Campo JSON | Coluna DB | Transformação |
|---|---|---|
| (nome do arquivo) | `slug` | Nome sem extensão `.json` |
| — | `id` | Novo UUIDv7 auto-gerado |
| `name` | `name` | Direto |
| `description` | `description` | Direto |
| `transport` | `transport` | Direto (após smart defaults) |
| `command` | `command` | Direto |
| `args` | `args` | `[]string` → JSON array |
| `env` | `env` | `map[string]string` → JSON object |
| `url` | `url` | Direto |
| `auth_type` | `auth_type` | Direto (após smart defaults) |
| `oauth2_client_id` | `oauth2_client_id` | Direto |
| `oauth2_auth_url` | `oauth2_auth_url` | Direto |
| `oauth2_token_url` | `oauth2_token_url` | Direto |
| `oauth2_scopes` | `oauth2_scopes` | `[]string` → JSON array |
| `oauth2_callback_port` | `oauth2_callback_port` | Direto |
| `oauth2_callback_host` | `oauth2_callback_host` | Direto |
| `oauth2_registration_url` | `oauth2_registration_url` | Direto |
| `oauth2_device_auth_url` | `oauth2_device_auth_url` | Direto |
| `disable_sse` | `disable_sse` | Direto |
| `prefer_bridge` | `prefer_bridge` | Direto |
| `enabled` | `enabled` | Direto |
| `auto_connect` | `auto_connect` | Direto |

Credenciais (`mcp-client:{slug}`, `mcp-tokens:{slug}`) não são tocadas — permanecem no `credentials.Manager`.

## Fases

### Fase 1 — Models GORM + AutoMigrate

1. Criar `internal/database/models_mcp.go` com 2 models GORM:
   - `MCPServerModel` com `UUIDModel` embeddado + todos os campos da tabela `mcp_servers`
   - `MCPServerLogModel` com `UUIDModel` embeddado + campos de `mcp_server_logs`
2. Funções de conversão: `MCPServerModel ↔ mcp.ServerConfig`, `MCPServerLogModel ↔ mcp.MCPServerLog`
3. Adicionar os 2 models ao `AutoMigrate` em `internal/database/database.go`

### Fase 2 — Repository layer

4. Criar `internal/mcp/repository.go` com interface `MCPRepository` (D8)
5. Implementar `DBMCPRepository` que recebe `*gorm.DB`
6. Testes: CRUD de servers, logs, limpeza por idade

### Fase 3 — Migrar Manager para usar Repository

7. Alterar `Manager`: receber `MCPRepository` em vez de depender de filesystem para configs
8. Reescrever `LoadConfigs()`: carregar do DB via Repository
9. Reescrever `SaveConfig()`: persistir via Repository em vez de JSON file
10. Reescrever `DeleteConfig()`: deletar via Repository em vez de `os.Remove`
11. Reescrever `DuplicateConfig()`: usar `Repository.DuplicateServer()`
12. Reescrever `GetConfig()`: buscar via Repository
13. Remover `WatchConfigs()` (file watcher)

### Fase 4 — Logs de conexão

14. Adicionar chamadas `Repository.LogEvent()` nos pontos do lifecycle:
    - `Connect()` → log `connected` (com tool count)
    - `Connect()` erro → log `error` (com mensagem)
    - `Disconnect()` → log `disconnected`
    - `healthCheckLoop` falha → log `health_fail`
    - Reconexão → log `reconnecting`
15. Expor via API Wails: `GetMCPServerLogs(slug, limit)`
16. Goroutine de limpeza: 30 dias (reutilizar pattern da AEP-0048)

### Fase 5 — Migração one-time filesystem → banco

17. Criar `internal/mcp/migration.go`:
    - Detectar JSON files nos 3 diretórios E tabela vazia
    - Carregar com resolução de prioridade (cwd > home > exe)
    - Inserir no banco
    - Renomear `~/.assistente/mcp/` → `~/.assistente/mcp.migrated/`
18. Chamar no `Manager.Start()` / `initMCP()` antes de carregar do DB

### Fase 6 — Remoção de código filesystem

19. Remover file watcher do Manager
20. Remover `configdir.Resolver` do MCP (manter para outros recursos que ainda usam disco)
21. Remover funções de leitura/escrita JSON do Manager
22. Simplificar `ParseServerConfig()` — manter smart defaults, remover I/O
23. Adicionar campo `Slug` e `ID` ao struct `ServerConfig` (hoje slug vem do nome do arquivo)

### Fase 7 — Testes

24. Testes Repository: CRUD servers, logs, limpeza por idade, roundtrip JSON de fields complexos
25. Testes Manager: LoadConfigs, SaveConfig, DeleteConfig, DuplicateConfig com DB
26. Testes migração: JSON files → DB, resolução de prioridade multi-dir, idempotência
27. Atualizar testes existentes do Manager

## Arquivos afetados

### Novos

| Arquivo | Descrição |
|---|---|
| `internal/database/models_mcp.go` | Models GORM: `MCPServerModel`, `MCPServerLogModel` |
| `internal/mcp/repository.go` | Interface `MCPRepository` + `DBMCPRepository` |
| `internal/mcp/repository_test.go` | Testes do repository |
| `internal/mcp/migration.go` | Migração one-time filesystem → DB |
| `internal/mcp/migration_test.go` | Testes da migração |

### Modificados

| Arquivo | Mudança |
|---|---|
| `internal/mcp/types.go` | Adicionar `ID` e `Slug` ao `ServerConfig`; criar tipo `MCPServerLog` |
| `internal/mcp/manager.go` | Refatorar para usar Repository; remover file watcher; remover I/O de disco |
| `internal/database/database.go` | Adicionar 2 models ao `AutoMigrate` |
| `controllers/mcp_controller.go` | Ajustes mínimos; expor `GetMCPServerLogs` |
| `internal/app/app_mcp.go` | Expor `GetMCPServerLogs` via Wails; ajustar inicialização |

### Sem alteração

- **Frontend**: mesma API Wails, mesmos tipos (D6). Store, componentes e páginas inalterados.
- **OAuth/credentials**: fluxos inalterados — `credentials.Manager` continua responsável por tokens.
- **Bridge pattern**: `MCPToolBridge` inalterado — continua registrando tools com namespace `mcp_{slug}__{tool}`.
- **Health check**: lógica inalterada — apenas adiciona logging.
- **MCP nativo**: decisão stdio vs HTTP native inalterada.

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|---|
| R1 | Perda de configs na migração | Baixa | Alto | Backup em `mcp.migrated/`; migração idempotente; credenciais intocadas |
| R2 | Multi-dir configs com conflitos | Média | Médio | Resolução de prioridade preservada na migração (cwd > home > exe) |
| R3 | `env` com API keys fica no banco sem criptografia | Baixa | Médio | Banco é local; `env` precisa estar acessível sem master password para iniciar processos |
| R4 | OAuth flows quebram após migração | Baixa | Alto | Credenciais ficam no `credentials.Manager` (intocadas); apenas config de endpoints muda de local |
| R5 | Smart defaults perdem-se após salvar no banco | Baixa | Baixo | Smart defaults aplicados antes de persistir; campos explícitos no banco |
| R6 | File watcher removido impede edição manual | Média | Baixo | Edição manual de JSON era uso avançado; UI e CLI cobrem todos os casos |

## Critérios de aceitação

1. **CRUD completo**: criar, listar, buscar, atualizar, deletar e duplicar servidores MCP funciona via banco
2. **Conexão preservada**: conectar, desconectar, reconectar funcionam identicamente ao comportamento atual
3. **OAuth funcional**: fluxo PKCE, client credentials e auto-discovery continuam funcionando
4. **Logs de conexão**: eventos de lifecycle são registrados em `mcp_server_logs`
5. **Migração filesystem**: configs JSON existentes são importadas para o banco na primeira execução
6. **Multi-dir resolvido**: configs de diferentes diretórios são mescladas com prioridade correta
7. **Backup**: diretório original renomeado para `mcp.migrated/` após migração
8. **Frontend inalterado**: mesma API Wails, sem mudanças em stores/componentes
9. **Credenciais intocadas**: `credentials.Manager` não é afetado; tokens OAuth persistem entre migrações
10. **Retenção de logs**: registros mais velhos que 30 dias são removidos automaticamente
11. **Tools MCP**: bridge pattern e namespacing inalterados; jobs continuam referenciando tools por nome
12. **File watcher removido**: código de watch de filesystem eliminado
13. **Testes**: repository, manager, migração cobertos por testes Go
