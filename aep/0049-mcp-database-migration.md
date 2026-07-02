# AEP-0049 — Migração de MCP Servers para Banco de Dados

## Dependências

- **AEP-0046** (Migração de IDs sequenciais para UUIDv7): Deve ser implementada primeiro. Fornece o `UUIDModel` com hook `BeforeCreate` que gera UUIDv7 automaticamente. Todas as PKs das tabelas desta AEP usam esse modelo.
- **Multi-user accounts** (implementado no PR #94): Deve estar disponível para que servidores MCP sejam sempre vinculados ao usuário logado via `user_id`.

## Resumo

Migrar a configuração de servidores MCP de arquivos JSON individuais no disco (`~/.assistente/mcp/{slug}.json`) para SQLite via GORM, sempre vinculando cada servidor MCP ao usuário logado. Uma tabela `mcp_servers` armazena a configuração persistente, uma tabela `mcp_server_logs` registra eventos de conexão, erros e health checks, e uma tabela `tool_catalog` registra tools builtin e tools MCP descobertas.

Credenciais OAuth/bearer/basic continuam no `credentials.Manager` existente (nunca no banco de configs). O file watcher é removido. O banco passa a ser a fonte persistida para servidores MCP, catálogo de tools e metadados de seleção; o registry em memória passa a ser uma projeção runtime usada para execução. Importação e exportação de MCP servers são integradas ao sistema `internal/portability` da AEP-0047, incluindo export canônico em `resources.mcpServers` e export compatível `mcpServers` para outros clientes MCP.

A AEP-0063 passa a consumir `tool_catalog` como fonte canônica para registrar execuções em `tool_invocations`. O catálogo não deve duplicar histórico de execução; ele identifica a tool, enquanto `tool_invocations` registra chamadas efêmeras feitas por chat, jobs, dry-run ou sistema.

## Motivação

1. **Consistência**: Após AEP-0048 (jobs) migrar para banco, MCP configs seriam o único recurso complexo ainda em filesystem. Unificar tudo no SQLite simplifica backup, restore e o futuro import/export (AEP-0047).

2. **Atomicidade**: Salvar config JSON + reconectar servidor não é atômico. Um crash entre "escrever arquivo" e "registrar no runtime" pode deixar estado inconsistente. Com GORM + SQLite WAL, operações são transacionais.

3. **Queries**: Listar servidores por transport, auth_type, status ou filtrar por enabled/auto_connect requer parse de N arquivos. No banco, é uma query SQL.

4. **Logs de conexão**: Não existe histórico de conexões, erros ou health check failures. A tabela `mcp_server_logs` viabiliza diagnóstico sem precisar buscar em logs do sistema.

5. **Multi-diretório eliminado**: O sistema atual resolve configs em 3 diretórios (exe, home, cwd) com prioridade. Essa complexidade é eliminada — o banco é a única fonte de verdade.

6. **Preparação para AEP-0047**: O export/import precisa acessar configs MCP de forma uniforme. Com banco, segue o mesmo pattern de Repository das demais entidades.

7. **Redução de contexto de tools**: Chats simples não devem enviar todas as tools builtin e MCP em todo turno. Persistir um catálogo classificado permite selecionar tools por pacote/capacidade antes de montar o payload do LLM.

8. **Escopo por usuário**: MCP servers são recursos do usuário logado. Tools MCP herdam esse escopo via servidor e nunca devem aparecer para outro usuário.

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
- OAuth auto-discovery via RFC 9728 + RFC 8414
- Estado runtime em `ServerStatus` (in-memory, não persistido)
- Tools MCP descobertas ficam apenas em `ServerStatus.Tools` e no `tools.Registry` runtime
- Tools builtin ficam apenas registradas em memória no `tools.Registry`
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

### D1 — Banco como fonte runtime, JSON legado como importação

O banco SQLite é a fonte de verdade runtime para servidores MCP. Arquivos JSON antigos no disco (`~/.assistente/mcp/*.json`) deixam de ser backing store e passam a ser apenas entrada de importação idempotente no startup pós-login. Não há modo dual de escrita/leitura runtime por JSON. O file watcher (`WatchConfigs`) é removido.

### D2 — Slug obrigatório e único

O slug (nome do arquivo JSON sem extensão, ex: `github`) vira coluna `slug` com constraint `NOT NULL`. O PK `id` é UUIDv7 (via `UUIDModel` da AEP-0046).

Como servidores MCP são sempre vinculados a um usuário, a unicidade do slug passa a ser composta por usuário: `UNIQUE(user_id, slug)`. Dois usuários podem ter um servidor `github`; o mesmo usuário não pode ter dois servidores com o mesmo slug.

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

### D6 — Compatibilidade das APIs Wails existentes

As APIs Wails existentes (`ListMCPServers`, `SaveMCPServer`, `ConnectMCPServer`, etc.) mantêm as mesmas assinaturas e tipos de retorno. O frontend não percebe a mudança de backing store. Novas APIs podem ser adicionadas para logs, catálogo e dry run, desde que não quebrem os contratos já publicados.

### D7 — Importação idempotente de filesystem para banco

Em todo startup pós-login, o app executa uma etapa global de importações legadas antes de qualquer manager carregar seu runtime do banco. Para MCP, essa etapa detecta arquivos JSON em `~/.assistente/mcp/` e os importa para o banco. Essa importação é segura para repetir:

1. Carrega todos os `.json` dos 3 diretórios (com resolução de prioridade)
2. Para cada slug, consulta o banco no escopo do usuário logado
3. Se o servidor já existe no banco, não sobrescreve nem altera nada
4. Se não existe, insere como novo registro (slug = nome do arquivo sem extensão)
5. Mantém os arquivos originais intocados

Credenciais não são tocadas — já estão no `credentials.Manager`. Isso preserva compatibilidade mínima com instalações antigas sem manter o runtime filesystem anterior.

O caminho de persistência e a orquestração da importação usam o mesmo serviço de portabilidade usado por import/export geral. O app dispara essa fase pós-login de forma centralizada; o Manager fornece apenas uma fonte read-only para os arquivos legados e, depois disso, `LoadConfigs()` carrega somente o banco. Descoberta, parsing para formato portátil, idempotência, importação e contadores ficam em `internal/portability`. Esse contrato deve ser reaproveitado por futuros recursos migrados de arquivos para banco.

### D8 — Repository pattern

A persistência é abstraída por uma interface `Repository`:

```go
type MCPRepository interface {
    ListServers(ctx context.Context) ([]ServerConfig, error)
    GetServer(ctx context.Context, slug string) (*ServerConfig, error)
    GetServerByID(ctx context.Context, id string) (*ServerConfig, error)
    SaveServer(ctx context.Context, cfg *ServerConfig) error
    DeleteServer(ctx context.Context, slug string) error
    DuplicateServer(ctx context.Context, slug, newSlug string) (*ServerConfig, error)
    LogEvent(ctx context.Context, entry *MCPServerLog) error
    GetLogs(ctx context.Context, slug string, limit int) ([]MCPServerLog, error)
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

O sistema de resolução em 3 diretórios (exe → home → cwd) é eliminado do runtime. O banco é a única fonte de verdade. A importação dos arquivos legados ainda usa a resolução de prioridade existente para ler formatos antigos e popular apenas registros ausentes.

### D12 — MCP servers são sempre user-scoped

Todo registro em `mcp_servers` possui `user_id NOT NULL`. Operações de CRUD, listagem, conexão, reconexão, logs e discovery sempre recebem/derivam o usuário logado e filtram por `user_id`.

Consequências:

- A API Wails continua recebendo `slug`, mas o backend resolve `slug` no escopo do usuário logado.
- Eventos podem continuar carregando `slug`, mas o runtime interno deve manter também `server_id` quando persistir logs/catalog.
- Importação filesystem → banco atribui os servidores importados ao usuário logado no momento do startup/importação ou ao owner definido pelo fluxo de startup.
- Credenciais MCP existentes continuam usando patterns por slug, mas a resolução deve ser feita no contexto do usuário logado para evitar colisão entre usuários.

### D13 — Catálogo persistido de tools

Adicionar `tool_catalog` como catálogo persistido de capabilities. Ele inclui:

- **Builtin tools** do app (`read_file`, `web_search`, `run_command`, etc.): `origin = builtin`, sem `mcp_server_id`, globais e disponíveis para todos os usuários.
- **MCP tools** descobertas em servidores MCP: `origin = mcp_bridge` ou `mcp_native`, com `mcp_server_id NOT NULL`, herdando o `user_id` do servidor.

O catálogo não executa tools. Ele serve para descoberta, UI, auditoria, teste/dry run, seleção por pacote e cálculo de custo. A execução continua passando pelo `tools.Registry` runtime e pelo `MCPToolBridge`/provider nativo quando aplicável.

### D14 — Registry em memória é projeção runtime

O `tools.Registry` deixa de ser tratado como fonte persistida. Ele é a projeção executável do que está disponível no processo:

- Builtin tools são registradas no startup e sincronizadas para `tool_catalog`.
- MCP tools são registradas/removidas conforme conexão/discovery do servidor e sincronizadas para `tool_catalog`.
- Tools ausentes não são apagadas imediatamente do banco; são marcadas como indisponíveis.

### D15 — Disponibilidade de tools é estado persistido observável

Se uma tool MCP ou builtin esperada não estiver disponível no runtime/discovery, o catálogo deve preservar o registro e atualizar `availability_status = unavailable` com `last_unavailable_at` e motivo. Quando a tool reaparecer, o registro volta para `available`, atualizando schema hash, descrição e timestamps se necessário.

Isso evita perda silenciosa de dados e permite à UI explicar por que uma tool não pode ser selecionada no momento.

### D16 — Seleção de tools via catálogo, não por heurística de idioma

A seleção dinâmica de tools para chat deve usar uma tool pequena de catálogo/seleção (`tool_catalog` ou `select_tools`) como caminho principal. O modelo recebe poucas tools iniciais e pode consultar o catálogo por capacidade, categoria, risco, servidor MCP e disponibilidade. O backend ativa o pacote selecionado para a próxima iteração do agentic loop.

Regras determinísticas baseadas em texto do usuário não são o mecanismo principal, porque dependem de idioma e vocabulário. Elas podem existir apenas como otimizações conservadoras baseadas em perfil/superfície.

#### Follow-up arquitetural — política única de seleção de tools

A implementação desta AEP garante o contrato de segurança no caminho crítico: quando `EnabledTools` é explícito no perfil, a expansão dinâmica via catálogo não pode ativar tools fora dessa allowlist; quando `DisableTools=true`, nenhuma tool é ativada; e quando `EnabledTools=nil`, a seleção dinâmica permanece aberta ao catálogo.

Ainda assim, a política de seleção/autorização de tools ficou distribuída entre helpers de tool definitions, integração com MCP nativo, callback de expansão dinâmica e filtros do catálogo. Isso é aceitável para esta entrega, mas não é a arquitetura final desejada.

Follow-up: centralizar essa lógica em uma abstração única (por exemplo `ToolSelectionPolicy` ou `ToolScope`) derivada do perfil ativo e do runtime/provider atual. Essa política deve ser a fonte de verdade para:

- tools iniciais enviadas ao LLM
- expansão dinâmica a partir de `tool_catalog`
- `AllowedTools` de MCP nativo
- remoção de bridge tools duplicadas quando MCP nativo estiver ativo
- semântica de `EnabledTools=nil`, `EnabledTools=[]`, allowlist explícita e `DisableTools`
- eventual restrição da própria consulta/listagem do catálogo ao escopo permitido pelo perfil

Issue de acompanhamento: <https://github.com/inclunet/assistente/issues/119>.

#### Follow-ups arquiteturais adicionais

A solução deste PR deixa alguns pontos deliberadamente funcionais, mas que devem evoluir em trabalhos futuros para reduzir acoplamento e melhorar a governança do catálogo:

- **Catálogo como capability geral**: `tool_catalog` hoje é persistido pelo repository MCP por conveniência de entrega, mas o catálogo já indexa builtin tools globais e deve virar repository/service próprio, sem depender de `internal/mcp`. Issue: <https://github.com/inclunet/assistente/issues/120>.
- **Planner real de tools**: a tool `tool_catalog` entrega descoberta e expansão dinâmica, mas ainda não substitui um planner completo com orçamento por bytes de schema, ranking, pacotes preferenciais, policies e resolução formal de conflitos. Issue: <https://github.com/inclunet/assistente/issues/121>.
- **Metadados declarativos nas builtin tools**: os metadados de categoria/classe/pacote/risco das builtin tools estão centralizados em mapa determinístico. No futuro, cada builtin tool deve declarar seus próprios metadados no descriptor/registro da tool. Issue: <https://github.com/inclunet/assistente/issues/122>.
- **Importações legadas como serviço observável**: o gatilho pós-login atual é suficiente para MCP, mas quando skills e outros recursos entrarem no fluxo, a importação legada deve virar um serviço registrável e observável, com resultados estruturados. Issue: <https://github.com/inclunet/assistente/issues/123>.

#### Refinamento posterior — AEP-0081

AEP-0081 refina a D16 sem substituir o catálogo persistido desta AEP: `tool_catalog` passa a ser a interface única de control-plane para `search`, `load`, `unload` e `list_loaded`, com `action` opcional e default `search` para compatibilidade. A seleção por perfil deixa de ser apenas `EnabledTools=nil|[]|lista` e passa a resolver estados tri-state (`disabled`, `on_demand`, `preloaded`), preservando a semântica legada como migração.

### D17 — Dry run/teste de tools

Builtin tools e MCP tools devem poder ser testadas por um fluxo de dry run semelhante ao dos jobs:

- Builtin tools são testadas por nome, schema e argumentos.
- MCP tools são testadas por `mcp_server_id` + nome da tool, via bridge local ou caminho nativo quando aplicável.
- O resultado registra disponibilidade, erro, duração, origem, servidor MCP e bloqueios por política.
- Tools destrutivas ou de escrita exigem confirmação/política explícita.

### D18 — Protocolo de implementação no PR

Esta AEP deve ser atualizada no próprio PR antes do código. A implementação acontece na branch do PR em fases contínuas, com commit e push a cada fase. Só deve pausar por problema técnico real, risco de perda de dados, conflito arquitetural ou dúvida intransponível. Cada fase deve deixar a base em estado consistente, com testes relevantes.

## Tabelas

### mcp_servers

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 via `UUIDModel.BeforeCreate` |
| `user_id` | TEXT | NOT NULL, INDEX, FK→users.id | Dono do servidor MCP |
| `slug` | TEXT | NOT NULL, INDEX | Ex: `github`, `filesystem`; único por usuário |
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

Índices/constraints:

- `UNIQUE(user_id, slug)`

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

### tool_catalog

| Coluna | Tipo | Constraints | Notas |
|---|---|---|---|
| `id` | TEXT | PK | UUIDv7 |
| `user_id` | TEXT | INDEX | Null para builtin global; preenchido para MCP por conveniência de query |
| `mcp_server_id` | TEXT | FK→mcp_servers.id, INDEX | Obrigatório para MCP tools; null para builtin |
| `name` | TEXT | NOT NULL, INDEX | Nome executável, ex: `read_file` ou `mcp_github__create_issue` |
| `display_name` | TEXT | NOT NULL | Nome curto para UI |
| `description` | TEXT | | Descrição para seleção |
| `origin` | TEXT | NOT NULL, INDEX | `builtin` / `mcp_bridge` / `mcp_native` |
| `category` | TEXT | INDEX | `filesystem`, `web`, `shell`, `tasklist`, `http`, `mcp:<server>` |
| `class` | TEXT | INDEX | `read_context`, `edit_files`, `run_commands`, `web_lookup`, `http_api`, `task_management`, `mcp_tool` |
| `package` | TEXT | INDEX | `coding_readonly`, `coding_edit`, `web`, `tasks`, `mcp:<server>` |
| `risk` | TEXT | INDEX | `read`, `write`, `destructive`, `network`, `shell` |
| `schema` | TEXT | | JSON Schema completo |
| `schema_hash` | TEXT | INDEX | Hash para detectar mudança de schema |
| `schema_bytes` | INT | | Custo aproximado do schema |
| `tags` | TEXT | | JSON array |
| `availability_status` | TEXT | NOT NULL, INDEX | `available` / `unavailable` |
| `availability_reason` | TEXT | | Último motivo de indisponibilidade |
| `last_seen_at` | DATETIME | INDEX | Última vez vista no registry/discovery |
| `last_available_at` | DATETIME | | Última vez disponível |
| `last_unavailable_at` | DATETIME | | Última vez marcada indisponível |
| `last_tested_at` | DATETIME | | Último dry run/teste |
| `last_test_status` | TEXT | | `ok` / `error` / `blocked` |
| `last_test_error` | TEXT | | Mensagem do último teste |
| `created_at` | DATETIME | | |
| `updated_at` | DATETIME | | |

Índices/constraints:

- Builtin: `origin = builtin`, `mcp_server_id IS NULL`, `user_id IS NULL`, `UNIQUE(origin, name)`
- MCP: `origin IN (mcp_bridge, mcp_native)`, `mcp_server_id NOT NULL`, `UNIQUE(mcp_server_id, name)`

## Mapeamento de dados: filesystem → banco

### ServerConfig (JSON → tabela `mcp_servers`)

| Campo JSON | Coluna DB | Transformação |
|---|---|---|
| (nome do arquivo) | `slug` | Nome sem extensão `.json` |
| — | `id` | Novo UUIDv7 auto-gerado |
| — | `user_id` | Usuário logado/owner da migração |
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

### Tools runtime → tabela `tool_catalog`

| Origem runtime | Colunas principais | Transformação |
|---|---|---|
| Builtin `tools.Tool` | `origin`, `name`, `description`, `schema` | `origin = builtin`, `user_id = NULL`, `mcp_server_id = NULL` |
| `MCPToolBridge` | `origin`, `mcp_server_id`, `name`, `display_name`, `schema` | Vincula ao servidor MCP do usuário; `name` usa namespace `mcp_{slug}__{tool}` |
| MCP nativo elegível | `origin`, `mcp_server_id`, `name`, `display_name`, `schema` | Mesmo catálogo; seleção alimenta `AllowedTools` |
| Tool ausente | `availability_status` | Marca `unavailable`, preservando registro |
| Tool reaparecida | `availability_status`, `schema_hash`, timestamps | Marca `available` e atualiza schema/metadados |

## Fases

### Fase 0 — Atualizar AEP no PR

1. Atualizar esta AEP com user scope, catálogo de tools, disponibilidade, seleção via catálogo, dry run e protocolo de execução.
2. Commitar e pushar a atualização da AEP antes de iniciar código.

### Fase 1 — Models GORM + AutoMigrate

1. Criar `internal/database/models_mcp.go` com models GORM:
   - `MCPServerModel` com `UUIDModel` embeddado + todos os campos da tabela `mcp_servers`, incluindo `UserID`
   - `MCPServerLogModel` com `UUIDModel` embeddado + campos de `mcp_server_logs`
   - `ToolCatalogModel` com `UUIDModel` embeddado + campos de `tool_catalog`
2. Funções de conversão: `MCPServerModel ↔ mcp.ServerConfig`, `MCPServerLogModel ↔ mcp.MCPServerLog`, `ToolCatalogModel ↔ tools.ToolCatalogEntry`
3. Adicionar os models ao `AutoMigrate` em `internal/database/database.go`

### Fase 2 — Repository layer

4. Criar `internal/mcp/repository.go` com interface `MCPRepository` (D8), recebendo `context.Context` para resolver `user_id`
5. Implementar `DBMCPRepository` que recebe `*gorm.DB`
6. Criar repository de catálogo de tools ou expandir repository MCP com métodos de catálogo
7. Testes: CRUD de servers por usuário, logs, catálogo, indisponibilidade/reaparecimento, limpeza por idade

### Fase 3 — Migrar Manager para usar Repository

8. Alterar `Manager`: receber `MCPRepository` em vez de depender de filesystem para configs
9. Reescrever `LoadConfigs()`: carregar do DB via Repository filtrando usuário logado
10. Reescrever `SaveConfig()`: persistir via Repository em vez de JSON file, com `user_id`
11. Reescrever `DeleteConfig()`: deletar via Repository em vez de `os.Remove`
12. Reescrever `DuplicateConfig()`: usar `Repository.DuplicateServer()` dentro do escopo do usuário
13. Reescrever `GetConfig()`: buscar via Repository no escopo do usuário
14. Remover `WatchConfigs()` (file watcher)

### Fase 4 — Logs de conexão

15. Adicionar chamadas `Repository.LogEvent()` nos pontos do lifecycle:
    - `Connect()` → log `connected` (com tool count)
    - `Connect()` erro → log `error` (com mensagem)
    - `Disconnect()` → log `disconnected`
    - `healthCheckLoop` falha → log `health_fail`
    - Reconexão → log `reconnecting`
16. Expor via API Wails: `GetMCPServerLogs(slug, limit)`
17. Goroutine de limpeza: 30 dias (reutilizar pattern da AEP-0048)

### Fase 5 — Importação filesystem → banco

18. Criar suporte MCP em `internal/portability`:
    - Detectar JSON files nos 3 diretórios por uma fonte read-only fornecida pelo MCP
    - Carregar com resolução de prioridade (cwd > home > exe)
    - Inserir no banco com `user_id` do usuário logado/owner da migração
    - Não sobrescrever registros já existentes no banco
    - Não renomear, apagar ou editar arquivos originais
19. Chamar a partir de uma etapa global pós-login em `internal/app`, antes de `mcp.Manager.LoadConfigs()` e dos demais managers que dependam de dados migrados; deve ser seguro executar em todo startup pós-login

### Fase 6 — Catálogo de tools

20. Sincronizar builtin tools do `tools.Registry` para `tool_catalog` como `origin = builtin`
21. Sincronizar MCP tools descobertas em `refreshServerOfferingsWithContext` para `tool_catalog` com `mcp_server_id`
22. Marcar tools ausentes como `unavailable` sem removê-las
23. Marcar tools reaparecidas como `available`, atualizando schema hash e timestamps
24. Expor APIs para listar catálogo, filtrar por categoria/pacote/risco/disponibilidade e testar tool

### Fase 7 — Tool catalog/select_tools no chat

25. Criar tool pequena `tool_catalog`/`select_tools` para descoberta e ativação de pacotes
26. Implementar `ToolPlanner` com orçamento por quantidade/bytes e modos `all`, `none`, `preset`, `catalog`, `auto`
27. Alterar o envio de chat para começar com tools mínimas e ativar tools completas na iteração seguinte do agentic loop
28. Integrar seleção com `ApplyNativeMCP`, preenchendo `AllowedTools` para MCP nativo

### Fase 8 — Dry run/teste de tools

29. Implementar teste de builtin tools por nome + argumentos
30. Implementar teste de MCP tools por servidor + tool
31. Persistir resultado resumido no `tool_catalog`
32. Respeitar políticas/confirmations para escrita, shell, HTTP mutável e operações destrutivas

### Fase 9 — Remoção de código filesystem

33. Remover file watcher do Manager
34. Remover `configdir.Resolver` do MCP (manter para outros recursos que ainda usam disco)
35. Remover funções de leitura/escrita JSON do Manager
36. Simplificar `ParseServerConfig()` — manter smart defaults, remover I/O
37. Adicionar campo `Slug`, `ID` e `UserID` ao struct `ServerConfig` (hoje slug vem do nome do arquivo)

### Fase 10 — Testes

38. Testes Repository: CRUD servers por usuário, logs, catálogo, limpeza por idade, roundtrip JSON de fields complexos
39. Testes Manager: LoadConfigs, SaveConfig, DeleteConfig, DuplicateConfig com DB e isolamento por usuário
40. Testes importação: JSON files → DB, resolução de prioridade multi-dir, idempotência e arquivos originais intocados
41. Testes catálogo: builtin global, MCP vinculada ao servidor, unavailable/reavailable, schema hash
42. Testes chat: `nil` vs `[]enabled_tools`, seleção via catálogo, MCP STDIO, MCP nativo com `AllowedTools`
43. Atualizar testes existentes do Manager

## Arquivos afetados

### Novos

| Arquivo | Descrição |
|---|---|
| `internal/database/models_mcp.go` | Models GORM: `MCPServerModel`, `MCPServerLogModel`, `ToolCatalogModel` |
| `internal/mcp/repository.go` | Interface `MCPRepository` + `DBMCPRepository` |
| `internal/mcp/repository_test.go` | Testes do repository |
| `internal/portability/legacy_import.go` | Orquestração reutilizável de importações legadas filesystem → DB |
| `internal/portability/mcp_servers.go` | Import/export portátil e importação idempotente de MCP servers |
| `internal/app/app_legacy_imports.go` | Gatilho pós-login centralizado para importações legadas |
| `internal/mcp/migration.go` | Adapter read-only da fonte legada MCP |
| `internal/mcp/migration_test.go` | Testes de carregamento DB-only do Manager |
| `internal/tools/catalog.go` | Tipos e sincronização do catálogo de tools |
| `internal/tools/catalog_test.go` | Testes do catálogo |
| `internal/tools/catalog_tool.go` | Tool pequena `tool_catalog`/`select_tools` |
| `internal/chat/tool_planner.go` | Planejamento de seleção de tools para chat |

### Modificados

| Arquivo | Mudança |
|---|---|
| `internal/mcp/types.go` | Adicionar `ID`, `Slug` e `UserID` ao `ServerConfig`; criar tipo `MCPServerLog` |
| `internal/mcp/manager.go` | Refatorar para usar Repository; remover file watcher; remover I/O de disco |
| `internal/database/database.go` | Adicionar models MCP e catálogo ao `AutoMigrate` |
| `controllers/mcp_controller.go` | Ajustes mínimos; expor `GetMCPServerLogs` |
| `controllers/tools_controller.go` | Listar catálogo, disponibilidade e dry run/teste |
| `internal/app/app_mcp.go` | Expor `GetMCPServerLogs` via Wails; ajustar inicialização |
| `internal/core/usecases/send_message.go` | Usar ToolPlanner e seleção via catálogo no chat |
| `internal/chat/tool_defs.go` | Construir defs selecionadas e alimentar MCP native allowlists |

### Sem alteração

- **OAuth/credentials**: fluxos inalterados — `credentials.Manager` continua responsável por tokens.
- **Bridge pattern**: `MCPToolBridge` inalterado — continua registrando tools com namespace `mcp_{slug}__{tool}`.
- **Health check**: lógica inalterada — apenas adiciona logging.
- **MCP nativo**: decisão stdio vs HTTP native inalterada.

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|---|
| R1 | Perda de configs na importação | Baixa | Alto | Arquivos originais intocados; importação idempotente; credenciais intocadas |
| R2 | Multi-dir configs com conflitos | Média | Médio | Resolução de prioridade preservada na migração (cwd > home > exe) |
| R3 | `env` com API keys fica no banco sem criptografia | Baixa | Médio | Banco é local; `env` precisa estar acessível sem master password para iniciar processos |
| R4 | OAuth flows quebram após migração | Baixa | Alto | Credenciais ficam no `credentials.Manager` (intocadas); apenas config de endpoints muda de local |
| R5 | Smart defaults perdem-se após salvar no banco | Baixa | Baixo | Smart defaults aplicados antes de persistir; campos explícitos no banco |
| R6 | File watcher removido impede edição manual | Média | Baixo | Edição manual de JSON era uso avançado; UI e CLI cobrem todos os casos |
| R7 | Servidor/tool MCP aparece para outro usuário | Baixa | Alto | `user_id` obrigatório em `mcp_servers`; queries sempre user-scoped; testes de isolamento |
| R8 | Tool some do catálogo por falha temporária | Média | Médio | Marcar `unavailable` em vez de deletar; preservar vínculo e schema hash |
| R9 | Seleção por catálogo adiciona iteração extra | Média | Baixo | Usar pacote mínimo + catálogo; para o usuário continua no mesmo turno |
| R10 | Tool destrutiva é ativada por seleção ampla | Baixa | Alto | Metadados de risco, policies e confirmações continuam obrigatórios |

## Critérios de aceitação

1. **CRUD completo**: criar, listar, buscar, atualizar, deletar e duplicar servidores MCP funciona via banco
2. **Conexão preservada**: conectar, desconectar, reconectar funcionam identicamente ao comportamento atual
3. **OAuth funcional**: fluxo PKCE, client credentials e auto-discovery continuam funcionando
4. **Logs de conexão**: eventos de lifecycle são registrados em `mcp_server_logs`
5. **Importação filesystem**: configs JSON existentes são importadas para o banco de forma idempotente em startups pós-login
6. **Multi-dir resolvido**: configs de diferentes diretórios são mescladas com prioridade correta
7. **Arquivos legados intocados**: diretório e arquivos JSON originais não são renomeados, apagados ou alterados
8. **Frontend inalterado**: mesma API Wails, sem mudanças em stores/componentes
9. **Credenciais intocadas**: `credentials.Manager` não é afetado; tokens OAuth persistem entre migrações
10. **Retenção de logs**: registros mais velhos que 30 dias são removidos automaticamente
11. **Tools MCP**: bridge pattern e namespacing inalterados; jobs continuam referenciando tools por nome
12. **File watcher removido**: código de watch de filesystem eliminado
13. **User scope**: servidores MCP e tools MCP são isolados por usuário
14. **Builtin tools globais**: tools builtin aparecem no catálogo como globais, sem `mcp_server_id`
15. **Catálogo de tools**: builtin e MCP tools são sincronizadas no banco com origem, categoria, risco, schema hash e disponibilidade
16. **Disponibilidade preservada**: tools indisponíveis são marcadas como `unavailable` e voltam a `available` quando reaparecem
17. **Seleção por catálogo**: chat inicia com poucas tools e usa `tool_catalog`/`select_tools` para ativar pacotes no mesmo turno do usuário via agentic loop
18. **MCP nativo allowlist**: seleção alimenta `AllowedTools` para providers com MCP nativo
19. **Dry run de tools**: builtin e MCP tools podem ser testadas com resultado persistido/resumido
20. **Testes contra perda de dados**: importação idempotente, arquivos originais intocados, credenciais intocadas e isolamento por usuário cobertos
21. **Testes**: repository, manager, importação, catálogo, seleção e dry run cobertos por testes Go
