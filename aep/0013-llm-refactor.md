# Plano de Refatoração: LLM Client Centralizado

**Status:** Done

**Data:** 6 de março de 2026  
**Objetivo:** Migrar o pacote `internal/llm` para o padrão de cliente HTTP centralizado, eliminando gambiarras e estabelecendo arquitetura limpa e bem testada.

---

## 1. Análise da Arquitetura Atual

### 1.1 Estrutura Existente

O pacote `internal/llm` possui 8 arquivos:

1. **types.go** (257 linhas) - Tipos de dados (Message, ToolCall, ChatRequest, etc.)
2. **api_errors.go** (84 linhas) - Tratamento de erros HTTP e Cloudflare
3. **toolchoice_ctx.go** - Utilitários de contexto para tool_choice
4. **localai_chatml.go** (218 linhas) - Parser de formato ChatML do LocalAI
5. **localai_chatml_test.go** - Testes do parser ChatML
6. **http_pool.go** - **PROBLEMÁTICO**: Gerencia `SharedHTTPClient` global
7. **client.go** (579 linhas) - **PROBLEMÁTICO**: Funções globais (GetModels, SendMessageSync, StreamChat)
8. **sync_client.go** (122 linhas) - **PROBLEMÁTICO**: Cliente síncrono usando SharedHTTPClient

### 1.2 Problemas Identificados

#### Problema #1: Funções Globais vs. Métodos de Struct
```go
// ❌ ATUAL: Funções globais
func GetModels(cfg *config.Config) ([]string, error) {
    resp, err := SharedHTTPClient.Do(req)  // Cliente global
}

func StreamChat(ctx, cfg, messages, params, handler, tools...) {
    resp, err := SharedHTTPClient.Do(req)  // Cliente global
}

// ✅ DESEJADO: Métodos de struct
func (c *Client) GetModels() ([]string, error) {
    resp, err := c.httpClient.Do(ctx, req)  // Cliente instanciado
}

func (c *Client) StreamChat(ctx, messages, params, handler, tools...) {
    resp, err := c.httpClient.Do(ctx, req)  // Cliente instanciado
}
```

**Por que é problema:**
- Impossível injetar credenciais via `credentials.Manager`
- Testes não podem mockar HTTP facilmente
- Viola princípio de injeção de dependências
- Inconsistente com outros componentes (Signal, Updater, HTTPRequest, etc.)

#### Problema #2: SharedHTTPClient Global
```go
// ❌ PROBLEMÁTICO (http_pool.go)
var SharedHTTPClient *http.Client

func init() {
    sharedTransport := &http.Transport{
        MaxIdleConns:          100,
        MaxIdleConnsPerHost:   20,
        // ... configurações de pooling
    }
    SharedHTTPClient = &http.Client{Transport: sharedTransport}
}
```

**Por que é problema:**
- Singleton global dificulta testes
- Não usa `httpclient.Client` centralizado (sem credenciais, sem retry, sem interceptor)
- Inconsistente com arquitetura estabelecida em Fases 1-5

#### Problema #3: Client Struct Inutilizado
```go
// ❌ EXISTE mas NÃO É USADO
type Client struct {
    cfg *config.Config
}

func NewClient(cfg *config.Config) *Client {
    return &Client{cfg: cfg}
}
```

**Descoberta:** `grep llm.NewClient` retorna 0 resultados no codebase.  
**Conclusão:** O struct existe mas todo código usa funções globais diretamente.

### 1.3 Pontos de Uso Mapeados

**Total:** 6 localizações precisam de mudanças

1. **app.go:213** - `a.llmClient = llm.NewSyncClient(cfg.APIBaseURL, cfg.APIKey)`
2. **app.go:3222** - `models, err := llm.GetModels(tempCfg)`
3. **llm.go:385** - `return llm.GetModels(cfg)`
4. **llm.go:715** - `go llm.StreamChat(a.ctx, cfg, messages, params, handler)`
5. **llm.go:1488** - `return llm.SendMessageSync(cfg, messages, params)`
6. **summary.go:201** - `client := llm.NewSyncClient(cfg.APIBaseURL, cfg.APIKey)`
7. **agent.go:291** - `llm.StreamChat(iterCtx, cfg, messages, params, handler, toolDefs...)`

---

## 2. Arquitetura Alvo (Clean Pattern)

### 2.1 Padrão Estabelecido (Fases 1-5)

Todos os componentes refatorados seguem este padrão:

```go
// 1. Struct com campos necessários
type Component struct {
    cfg        *config.Config        // Configuração específica
    credMgr    *credentials.Manager  // Gerenciador de credenciais
    httpClient *httpclient.Client    // Cliente HTTP centralizado
}

// 2. Construtor aceita credMgr
func NewComponent(cfg *config.Config, credMgr *credentials.Manager) *Component {
    return &Component{
        cfg:     cfg,
        credMgr: credMgr,
        httpClient: httpclient.New(&httpclient.Config{
            CredentialManager: credMgr,
            Timeout:          30 * time.Second,
        }, map[string]string{}),
    }
}

// 3. Métodos usam client.Do(ctx, req)
func (c *Component) DoSomething(ctx context.Context) error {
    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
    resp, err := c.httpClient.Do(ctx, req)
    // ...
}
```

### 2.2 Aplicação ao LLM Client

```go
// client.go - Arquitetura alvo
package llm

type Client struct {
    cfg        *config.Config
    credMgr    *credentials.Manager
    httpClient *httpclient.Client
}

func NewClient(cfg *config.Config, credMgr *credentials.Manager) *Client {
    return &Client{
        cfg:     cfg,
        credMgr: credMgr,
        httpClient: httpclient.New(&httpclient.Config{
            CredentialManager: credMgr,
            Timeout:          3 * time.Minute,  // Timeout maior para LLM
        }, map[string]string{}),
    }
}

// Métodos (não mais funções globais)
func (c *Client) GetModels() ([]string, error)
func (c *Client) SendMessageSync(messages []Message, params ChatParams) (string, error)
func (c *Client) StreamChat(ctx context.Context, messages []Message, params ChatParams, handler StreamHandler, tools ...ToolDefinition)
```

```go
// sync_client.go - Arquitetura alvo
type SyncClient struct {
    baseURL    string
    apiKey     string
    credMgr    *credentials.Manager
    httpClient *httpclient.Client
}

func NewSyncClient(baseURL, apiKey string, credMgr *credentials.Manager) *SyncClient {
    return &SyncClient{
        baseURL: baseURL,
        apiKey:  apiKey,
        credMgr: credMgr,
        httpClient: httpclient.New(&httpclient.Config{
            CredentialManager: credMgr,
            Timeout:          3 * time.Minute,
        }, map[string]string{}),
    }
}

func (sc *SyncClient) SimpleChat(ctx context.Context, model, systemPrompt, userMessage string) (string, error)
```

---

## 3. Plano de Execução em Fases

### Fase 1: Refatorar Client Struct (client.go)

**Objetivo:** Transformar funções globais em métodos, adicionar campos httpClient e credMgr.

**Mudanças:**

1. **Atualizar struct Client:**
   ```go
   type Client struct {
       cfg        *config.Config
       credMgr    *credentials.Manager        // NOVO
       httpClient *httpclient.Client          // NOVO
   }
   ```

2. **Atualizar NewClient:**
   ```go
   func NewClient(cfg *config.Config, credMgr *credentials.Manager) *Client {
       return &Client{
           cfg:     cfg,
           credMgr: credMgr,
           httpClient: httpclient.New(&httpclient.Config{
               CredentialManager: credMgr,
               Timeout:          3 * time.Minute,
           }, map[string]string{}),
       }
   }
   ```

3. **Transformar funções em métodos:**
   - `GetModels(cfg)` → `(c *Client) GetModels()`
   - `SendMessageSync(cfg, ...)` → `(c *Client) SendMessageSync(...)`
   - `StreamChat(ctx, cfg, ...)` → `(c *Client) StreamChat(ctx, ...)`

4. **Atualizar chamadas HTTP:**
   - `SharedHTTPClient.Do(req)` → `c.httpClient.Do(ctx, req)`
   - Adicionar `ctx context.Context` onde necessário
   - Usar `c.cfg.APIKey` em vez de receber cfg por parâmetro

**Validação:**
```bash
go build ./internal/llm 2>&1  # Deve compilar sem erros
```

---

### Fase 2: Refatorar SyncClient (sync_client.go)

**Objetivo:** Substituir `*http.Client` por `*httpclient.Client` e adicionar credMgr.

**Mudanças:**

1. **Atualizar struct:**
   ```go
   type SyncClient struct {
       baseURL    string
       apiKey     string
       credMgr    *credentials.Manager        // NOVO
       httpClient *httpclient.Client          // MUDOU de *http.Client
   }
   ```

2. **Atualizar NewSyncClient:**
   ```go
   func NewSyncClient(baseURL, apiKey string, credMgr *credentials.Manager) *SyncClient {
       return &SyncClient{
           baseURL: baseURL,
           apiKey:  apiKey,
           credMgr: credMgr,
           httpClient: httpclient.New(&httpclient.Config{
               CredentialManager: credMgr,
               Timeout:          3 * time.Minute,
           }, map[string]string{}),
       }
   }
   ```

3. **Atualizar chamadas HTTP:**
   - `c.Client.Do(req)` → `sc.httpClient.Do(ctx, req)`

**Validação:**
```bash
go build ./internal/llm 2>&1
```

---

### Fase 3: Deprecar http_pool.go

**Objetivo:** Remover dependência de SharedHTTPClient global.

**Ações:**

1. Adicionar comentário de deprecação no topo do arquivo:
   ```go
   // Package http_pool is DEPRECATED.
   // Use the centralized httpclient.Client via Client.httpClient instead.
   // This file is kept temporarily for reference but should not be used.
   ```

2. **OPÇÃO A (Recomendada):** Deletar o arquivo completamente  
   **OPÇÃO B (Conservadora):** Manter arquivo com comentário de deprecação

**Decisão:** Deletar completamente (nenhum código referencia mais após Fases 1-2).

**Validação:**
```bash
grep -r "SharedHTTPClient" . --include="*.go" | grep -v "docs/"
# Resultado esperado: 0 matches (exceto documentação)
```

---

### Fase 4: Atualizar app.go

**Objetivo:** Passar credMgr ao criar clientes LLM.

**Mudanças:**

1. **Linha 213:**
   ```go
   // ❌ ANTES
   a.llmClient = llm.NewSyncClient(cfg.APIBaseURL, cfg.APIKey)
   
   // ✅ DEPOIS
   a.llmClient = llm.NewSyncClient(cfg.APIBaseURL, cfg.APIKey, a.credMgr)
   ```

2. **Linha 3222:**
   ```go
   // ❌ ANTES
   models, err := llm.GetModels(tempCfg)
   
   // ✅ DEPOIS
   // Criar cliente temporário
   tempClient := llm.NewClient(tempCfg, a.credMgr)
   models, err := tempClient.GetModels()
   ```

**Validação:**
```bash
go build ./app.go 2>&1
```

---

### Fase 5: Atualizar llm.go

**Objetivo:** Criar client LLM no App e usar métodos ao invés de funções globais.

**Contexto:** `llm.go` contém os bindings Wails. Precisamos criar um campo `llmClient` no struct App e reutilizá-lo.

**Mudanças:**

1. **Adicionar campo em App struct (verificar se já existe):**
   ```go
   type App struct {
       // ... campos existentes
       llmClient *llm.Client  // Cliente LLM com credentials
   }
   ```

2. **Inicializar no startup:**
   ```go
   func (a *App) startup(ctx context.Context) {
       // ...
       cfg := a.db.GetConfig()
       a.llmClient = llm.NewClient(cfg, a.credMgr)
       // ...
   }
   ```

3. **Linha 385:**
   ```go
   // ❌ ANTES
   return llm.GetModels(cfg)
   
   // ✅ DEPOIS
   return a.llmClient.GetModels()
   ```

4. **Linha 715:**
   ```go
   // ❌ ANTES
   go llm.StreamChat(a.ctx, cfg, messages, params, handler)
   
   // ✅ DEPOIS
   go a.llmClient.StreamChat(a.ctx, messages, params, handler)
   ```

5. **Linha 1488:**
   ```go
   // ❌ ANTES
   return llm.SendMessageSync(cfg, messages, params)
   
   // ✅ DEPOIS
   return a.llmClient.SendMessageSync(messages, params)
   ```

**Importante:** Verificar se cfg (API key, model, etc.) mudou durante runtime. Se sim, recriar cliente:
```go
func (a *App) ensureLLMClient() {
    cfg := a.db.GetConfig()
    if a.llmClient == nil || a.llmClientCfgHash != cfgHash(cfg) {
        a.llmClient = llm.NewClient(cfg, a.credMgr)
        a.llmClientCfgHash = cfgHash(cfg)
    }
}
```

**Validação:**
```bash
go build ./llm.go 2>&1
```

---

### Fase 6: Atualizar Outros Arquivos

**Objetivo:** Atualizar summary.go e agent.go.

**Mudanças:**

1. **summary.go:201:**
   ```go
   // ❌ ANTES
   client := llm.NewSyncClient(cfg.APIBaseURL, cfg.APIKey)
   
   // ✅ DEPOIS
   client := llm.NewSyncClient(cfg.APIBaseURL, cfg.APIKey, a.credMgr)
   ```
   
   **Nota:** Verificar se `a.credMgr` está disponível no contexto. Se não, passar como parâmetro ou criar helper.

2. **agent.go:291:**
   ```go
   // ❌ ANTES
   llm.StreamChat(iterCtx, cfg, messages, params, handler, toolDefs...)
   
   // ✅ DEPOIS
   // Assumindo que agent tem referência ao llmClient ou cria um
   llmClient := llm.NewClient(cfg, a.credMgr)
   llmClient.StreamChat(iterCtx, messages, params, handler, toolDefs...)
   ```

**Validação:**
```bash
go build ./summary.go ./agent.go 2>&1
```

---

### Fase 7: Validação Final

**Objetivo:** Garantir que tudo compila e todos os testes passam.

**Checklist:**

1. **Compilação completa:**
   ```bash
   go build ./... 2>&1
   ```

2. **Testes Go:**
   ```bash
   go test ./... -v
   ```

3. **Testes Frontend:**
   ```bash
   npm --prefix frontend run test
   ```

4. **Lint Frontend:**
   ```bash
   npm --prefix frontend run lint
   ```

5. **Build Frontend:**
   ```bash
   npm --prefix frontend run build
   ```

6. **Task completa:**
   ```bash
   # Executar via VS Code Task: "Check: all"
   ```

**Critérios de Sucesso:**
- ✅ 0 erros de compilação Go
- ✅ Todos os testes Go passando
- ✅ 285 testes frontend passando (baseline)
- ✅ 0 erros no lint frontend (470 warnings aceitáveis - baseline)
- ✅ Build frontend concluído com sucesso

---

## 4. Benefícios da Refatoração

### 4.1 Técnicos

1. **Injeção de Dependências:** Permite testes unitários com mocks
2. **Centralização:** Uma única fonte de verdade para HTTP client
3. **Credentials Manager:** Intercepta e adiciona credenciais automaticamente
4. **Retry Logic:** Lógica de retry integrada do `httpclient.Client`
5. **Timeout Configurável:** Controle fino de timeouts por operação
6. **Consistência:** Padrão unificado com todos os outros componentes

### 4.2 Manutenibilidade

1. **Menos Globals:** Elimina variáveis globais problemáticas
2. **Código Testável:** Facilita criação de testes
3. **Rastreabilidade:** Fluxo de dados mais claro (config → client → métodos)
4. **Escalabilidade:** Fácil adicionar novos recursos (ex: circuit breaker)

### 4.3 Comparação com Arquitetura Anterior

| Aspecto | Antes (Global Functions) | Depois (Struct Methods) |
|---------|-------------------------|-------------------------|
| HTTP Client | `SharedHTTPClient` global | `c.httpClient` instanciado |
| Credenciais | Sem suporte | Via `credentials.Manager` |
| Retry | Manual/incompleto | Automático via `httpclient` |
| Testes | Difícil (mock globals) | Fácil (mock struct) |
| Timeout | Fixo em `init()` | Configurável por cliente |
| API | Funções `llm.GetModels(cfg)` | Métodos `client.GetModels()` |
| Consistência | Diferente de outros componentes | Alinhado com Fases 1-5 |

---

## 5. Riscos e Mitigações

### 5.1 Breaking Changes na API

**Risco:** Código existente que chama `llm.GetModels(cfg)` vai quebrar.

**Mitigação:**
- ✅ Apenas 6 pontos de uso identificados (escopo controlado)
- ✅ Mudanças atômicas: tudo atualizado de uma vez
- ✅ Validação com testes completos em cada fase
- ✅ Sem necessidade de manter compatibilidade (decisão aprovada pelo usuário)

### 5.2 Performance do Connection Pooling

**Risco:** `SharedHTTPClient` tinha pooling otimizado (MaxIdleConns: 100, MaxIdleConnsPerHost: 20).

**Mitigação:**
- ✅ `httpclient.New()` já configura pooling adequado
- ✅ Se necessário, podemos customizar via `httpclient.Config`
- ✅ LLM tipicamente usa poucos hosts (1-2), pooling padrão suficiente

### 5.3 Timeout Diferenciado para LLM

**Risco:** LLM precisa de timeout maior (3+ minutos) vs. outros serviços (30s).

**Mitigação:**
- ✅ `httpclient.Config.Timeout` configurável por cliente
- ✅ Definir 3 minutos explicitamente no `NewClient()` e `NewSyncClient()`

---

## 6. Documentação e Rastreabilidade

### 6.1 Atualização de Docs

Após implementação, atualizar:

1. **docs/HTTP_CLIENT_CENTRALIZATION_PLAN.md:**
   - Marcar Fase 6 (LLM) como ✅ Completa
   - Adicionar resumo das mudanças
   - Documentar pontos de uso atualizados

2. **docs/LLM_REFACTOR_PLAN.md (este arquivo):**
   - Marcar fases executadas
   - Adicionar notas de implementação
   - Registrar problemas encontrados e soluções

### 6.2 Commit Message Padrão

```
refactor(llm): migrate to centralized HTTP client pattern

- Transform GetModels, SendMessageSync, StreamChat from global functions to Client methods
- Add credMgr and httpClient fields to Client and SyncClient structs
- Update NewClient() and NewSyncClient() to accept credentials.Manager
- Replace SharedHTTPClient.Do() with httpClient.Do(ctx, req) (6 locations)
- Deprecate/remove http_pool.go (SharedHTTPClient no longer needed)
- Update app.go, llm.go, summary.go, agent.go to use new methods

Breaking Changes:
- llm.GetModels(cfg) → client.GetModels()
- llm.SendMessageSync(cfg, ...) → client.SendMessageSync(...)
- llm.StreamChat(ctx, cfg, ...) → client.StreamChat(ctx, ...)
- llm.NewSyncClient(url, key) → llm.NewSyncClient(url, key, credMgr)

Benefits:
- Consistent with HTTP client centralization (Phases 1-5)
- Credentials Manager integration
- Better testability (no global state)
- Configurable timeouts per client
- Automatic retry logic

Tests: All passing (285 frontend + Go test suite)
Related: docs/LLM_REFACTOR_PLAN.md, docs/HTTP_CLIENT_CENTRALIZATION_PLAN.md
```

---

## 7. Checklist de Execução

### Pré-Implementação
- [x] Análise completa da estrutura LLM
- [x] Mapeamento de todos os pontos de uso
- [x] Documentação do plano em fases
- [x] Aprovação do usuário

### Fase 1: Client Struct
- [ ] Adicionar campos credMgr e httpClient
- [ ] Atualizar NewClient() para aceitar credMgr
- [ ] Transformar GetModels() em método
- [ ] Transformar SendMessageSync() em método
- [ ] Transformar StreamChat() em método
- [ ] Substituir SharedHTTPClient.Do() por httpClient.Do(ctx, req)
- [ ] Testar compilação: `go build ./internal/llm`

### Fase 2: SyncClient
- [ ] Adicionar campos credMgr e httpClient
- [ ] Atualizar NewSyncClient() para aceitar credMgr
- [ ] Substituir Client *http.Client por httpClient *httpclient.Client
- [ ] Atualizar chamadas Do() para Do(ctx, req)
- [ ] Testar compilação: `go build ./internal/llm`

### Fase 3: http_pool.go
- [ ] Deletar arquivo http_pool.go (ou adicionar deprecation)
- [ ] Verificar: `grep SharedHTTPClient` retorna 0 matches

### Fase 4: app.go
- [ ] Atualizar linha 213 (NewSyncClient)
- [ ] Atualizar linha 3222 (GetModels)
- [ ] Testar compilação: `go build ./app.go`

### Fase 5: llm.go
- [ ] Adicionar campo llmClient no App struct (se não existir)
- [ ] Inicializar llmClient no startup
- [ ] Atualizar linha 385 (GetModels)
- [ ] Atualizar linha 715 (StreamChat)
- [ ] Atualizar linha 1488 (SendMessageSync)
- [ ] Testar compilação: `go build ./llm.go`

### Fase 6: Outros Arquivos
- [ ] Atualizar summary.go:201
- [ ] Atualizar agent.go:291
- [ ] Testar compilação: `go build ./summary.go ./agent.go`

### Fase 7: Validação Final
- [ ] `go build ./... 2>&1` (0 erros)
- [ ] `go test ./... -v` (todos passando)
- [ ] Task "Frontend: lint" (0 erros, 470 warnings OK)
- [ ] Task "Frontend: test" (285 testes passando)
- [ ] Task "Frontend: build" (sucesso)
- [ ] Task "Check: all" (tudo passando)

### Pós-Implementação
- [ ] Atualizar docs/HTTP_CLIENT_CENTRALIZATION_PLAN.md
- [ ] Marcar fases neste documento
- [ ] Commit com mensagem detalhada
- [ ] Atualizar TODO list

---

## 8. Notas de Implementação

### 8.1 Timeout Recommendations

- **Client LLM (streaming):** 3-5 minutos (modelos grandes podem demorar)
- **SyncClient (chamadas curtas):** 2-3 minutos
- **Reasoning models (o1, o3, DeepSeek R1):** 5+ minutos

### 8.2 Context Usage

Todos os métodos devem aceitar `context.Context`:
- Permite cancelamento de requests longos
- Timeout configurável via context
- Suporte a tracing/logging (futuro)

### 8.3 Config Refresh

Se config (API key, model, base URL) mudar em runtime:
- **Opção A:** Recriar cliente quando detectar mudança
- **Opção B:** Fazer Client ser "hot-reloadable" (ler cfg a cada chamada)

**Recomendação:** Opção A (performance) com helper `ensureLLMClient()`.

---

## 9. Conclusão

Esta refatoração elimina os últimos resquícios de arquitetura problemática no pacote LLM, alinhando-o com o padrão limpo estabelecido nas Fases 1-5 da centralização HTTP.

**Princípios aplicados:**
- ✅ Sem gambiarras
- ✅ Sem compatibilidade forçada
- ✅ Código bem-feito e testado
- ✅ Reuso de padrões estabelecidos
- ✅ Excelência técnica

**Resultado esperado:**
- Código limpo, testável e manutenível
- Consistência arquitetural em todo o projeto
- Facilidade para adicionar features futuras (circuit breaker, métricas, etc.)

---

**Status:** 📋 Planejamento Completo - Pronto para Execução  
**Próximo Passo:** Aprovação e execução Fase 1
