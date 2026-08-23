# Plano: Cliente HTTP Unificado com Interceptor de Autenticação

**Status:** Done

## Problema Atual

A autenticação via credenciais (API tokens, Bearer tokens, etc.) está **duplicada** em múltiplos adapters:

- **Signal**: resolve token do cofre, adiciona header `Authorization: Bearer {token}`
- **HTTPRequest (web tools)**: resolve token manualmente, cria headers
- **WebFetch**: cria cliente HTTP diretamente, sem interceptor centralizado
- **Telegram/Slack**: podem precisar de tokens para APIs

## Solução Proposta

Criar uma **camada HTTP centralizada** (`internal/tools/http/client.go`) que:

1. **Intercepta requisições HTTP** por domínio/padrão
2. **Resolve credenciais** automaticamente do credential manager
3. **Adiciona headers de autenticação** sem que cada adapter se preocupe
4. É usada por: Signal, HTTPRequest, WebFetch, Telegram, Slack

## Arquitetura

```
┌─────────────────────────────────────────────────────────┐
│                   Credential Manager                    │
│             (resolve por padrão de domínio)             │
└──────────────────────┬──────────────────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────────────────┐
│         HTTP Client Unificado                           │
│  ┌─────────────────────────────────────────────────────┤
│  │ • Interceptor de autenticação                       │
│  │ • Resolver credenciais por domínio                 │
│  │ • Aplicar headers (Authorization, custom headers)  │
│  │ • Request/Response logging (opcional)              │
│  │ • Retry logic, timeouts                            │
│  └─────────────────────────────────────────────────────┤
└──────────────────────┬──────────────────────────────────┘
        │              │              │              │
        ▼              ▼              ▼              ▼
    Signal         HTTPRequest    WebFetch      Telegram
    Adapter         (Tools)        (Tools)        /Slack
```

## Implementação

### 1. Cliente HTTP Centralizado (`internal/tools/http/client.go`)

```go
type ClientConfig struct {
    CredentialManager *credentials.Manager
    Logger           LoggerFn  // opcional
    RetryPolicy      RetryPolicy
}

type AuthInterceptor struct {
    credMgr *credentials.Manager
    // mapeamento de domínio → padrão de credencial
    // ex: "api.slack.com" → "slack-bot-token"
    domainPatterns map[string]string
}

// Do() executa requisição com interceptação de auth
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error)

// método auxiliar para resolver credencial por domínio
func (ai *AuthInterceptor) applyAuth(req *http.Request) error
```

### 2. Configuração de Domínios

Cada integração define seu próprio padrão:

```go
// Signal
signalClient := http.NewClient(cfg, map[string]string{
    "localhost": credentials.PatternSignalAPIToken,
    "api.example.com": credentials.PatternSignalAPIToken,
})

// HTTPRequest (web tools)
httpClient := http.NewClient(cfg, map[string]string{
    "*": credentials.PatternHTTPAuth, // fallback genérico
})
```

### 3. Refatoração Signal

**Antes:**
```go
func (a *Adapter) Do(req *http.Request, apiToken string) {
    if apiToken != "" {
        req.Header.Set("Authorization", "Bearer " + apiToken)
    }
    resp, _ := http.DefaultClient.Do(req)
}
```

**Depois:**
```go
func (a *Adapter) Do(req *http.Request) {
    resp, _ := a.httpClient.Do(ctx, req) // auth é automática
}
```

### 4. Refatoração HTTPRequest

Migrar `internal/tools/web/http_request.go` para usar cliente unificado.

### 5. Refatoração WebFetch

Migrar `internal/tools/web/fetch.go` para usar cliente unificado.

### 6. Telegram/Slack

- **Telegram Bot API**: usa token na URL (`?token=xxx`), não em header
- **Slack API**: usa token em `Authorization: Bearer token`
- Ambos podem usar o interceptor se configurados corretamente

## Remoção de Duplicação

### O que será removido:

1. `signal/register.go`:
   - Remover `applyAuthHeader` helper (agora no interceptor)
   - Simplificar assinaturas de função para não passar `apiToken`

2. `signal/adapter.go`:
   - Remover `applyAuthHeader` method
   - Remover `apiToken` field (resolvido automaticamente)
   - Simplificar `NewAdapter` signature

3. `app.go`:
   - Remover `resolveSignalAPIToken` helper (agora no interceptor)
   - Simplificar `persistChannelCredentials` para Signal (já será salvo via manager)
   - Remover lógica de resolver refs em `initMessaging` para Signal

4. `http_request.go`:
   - Remover resolução manual de credenciais
   - Remover construção manual de headers de auth

## Testes

Testes para o cliente HTTP unificado:

1. **Interceptor**: testa se headers são adicionados corretamente
2. **Credential resolution**: testa se credenciais são resolvidas por padrão
3. **Multiple adapters**: testa se Signal/HTTPRequest funcionam com cliente centralizado

## Timeline

1. ✅ Documentação (este arquivo)
2. Criar `internal/tools/http/client.go`
3. Criar `internal/tools/http/client_test.go`
4. Refatorar Signal (register.go + adapter.go)
5. Refatorar HTTPRequest (http_request.go)
6. Refatorar WebFetch (fetch.go)
7. Remover código duplicado do `app.go`
8. Testar Signal/HTTPRequest/WebFetch
9. Validar com Telegram/Slack (se aplicável)
10. Executar suite de testes completa

## Benefícios

- ✅ **DRY**: sem duplicação de lógica de auth
- ✅ **Manutenibilidade**: mudanças em autenticação feitas em um único lugar
- ✅ **Extensibilidade**: fácil adicionar novos adapters ou protocolos de auth
- ✅ **Testabilidade**: interceptor pode ser testado isoladamente
- ✅ **Segurança**: headers de auth sempre aplicados corretamente
