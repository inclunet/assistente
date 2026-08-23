# MCP OAuth Auto-Discovery

**Status:** In Progress — discovery e testes existem; contrato de campos readonly/manual override permanece incompleto

## Objetivo

Implementar auto-discovery de configuração OAuth para servidores MCP remotos (transport `streamable` ou `sse`). Quando o usuário preenche a URL do servidor e sai do campo, o sistema consulta automaticamente os endpoints well-known do servidor para preencher os campos de autenticação.

## Contexto Técnico

A spec MCP Authorization (https://modelcontextprotocol.io/specification/2025-03-26/basic/authorization) define dois endpoints de discovery padrão:

1. **Protected Resource Metadata**: `GET {server_origin}/.well-known/oauth-protected-resource`
   - Retorna `authorization_servers[]`, `scopes_supported[]`, `resource_name`
   
2. **Authorization Server Metadata** (RFC 8414): `GET {auth_server}/.well-known/oauth-authorization-server`
   - Retorna `authorization_endpoint`, `token_endpoint`, `scopes_supported[]`, `code_challenge_methods_supported[]`, `grant_types_supported[]`, `registration_endpoint`

Exemplos reais que validei:

**Slack** (`https://mcp.slack.com/mcp`):
- Protected Resource: `https://mcp.slack.com/.well-known/oauth-protected-resource` → confirma auth server `https://mcp.slack.com`
- Auth Server: `https://mcp.slack.com/.well-known/oauth-authorization-server` → retorna auth_url, token_url, scopes, S256

**Atlassian** (`https://mcp.atlassian.com/v1/sse`):
- Protected Resource: 404 (não implementado)
- Auth Server: `https://mcp.atlassian.com/.well-known/oauth-authorization-server` → retorna auth_url, token_url, registration_endpoint

## Estado atual

- [x] Discovery backend em `internal/mcp/discovery.go`, com testes em
      `internal/mcp/oauth_test.go`.
- [x] Binding e disparo no `McpPage`, com estados de loading/found/not_found.
- [x] Feedback acessível e testes do painel em
      `McpConnectionSection.test.tsx`.
- [ ] `McpConnectionSection` recebe `discoveredFields`, mas atualmente o renomeia
      para `_discoveredFields` e não o usa para tornar campos readonly.
- [ ] O override manual previsto ainda não fecha o contrato por campo descrito
      nesta AEP.

## Plano original

### 1. Backend — Novo endpoint Go `DiscoverMCPServerAuth`

Criar uma função pública em `app.go` (binding Wails) que recebe uma URL de servidor MCP e retorna os metadados de autenticação descobertos.

**Arquivo**: `internal/mcp/discovery.go` (novo)

```go
// OAuthDiscoveryResult contém os metadados OAuth descobertos de um servidor MCP.
type OAuthDiscoveryResult struct {
    Found              bool     `json:"found"`
    AuthType           AuthType `json:"authType"`           // "oauth2_pkce" ou "oauth2_client_credentials"
    AuthURL            string   `json:"authUrl"`
    TokenURL           string   `json:"tokenUrl"`
    Scopes             []string `json:"scopes"`
    ClientID           string   `json:"clientId,omitempty"` // se registration endpoint fornecer
    RegistrationURL    string   `json:"registrationUrl,omitempty"`
    ResourceName       string   `json:"resourceName,omitempty"`
    SupportsPKCE       bool     `json:"supportsPkce"`
    Error              string   `json:"error,omitempty"`
}
```

**Lógica de discovery (em ordem)**:

1. Extrair a **origin** da URL do servidor (scheme + host)
2. Tentar `GET {origin}/.well-known/oauth-protected-resource`
   - Se 200: extrair `authorization_servers[0]` como auth server base
   - Se falhar: usar a origin do servidor como auth server base
3. Tentar `GET {auth_server_base}/.well-known/oauth-authorization-server`
   - Se 200: extrair `authorization_endpoint`, `token_endpoint`, `scopes_supported`, `code_challenge_methods_supported`
   - Se falhar: retornar `Found: false`
4. Determinar `AuthType`:
   - Se `code_challenge_methods_supported` inclui `"S256"` → `oauth2_pkce`
   - Senão, se `grant_types_supported` inclui `"client_credentials"` → `oauth2_client_credentials`
   - Senão → `oauth2_pkce` (default mais seguro)
5. Retornar `OAuthDiscoveryResult` preenchido

**Timeouts**: usar 5s para cada request HTTP. Não falhar ruidosamente — se não encontrar, retornar `Found: false`.

**Binding em `app.go`**:
```go
func (a *App) DiscoverMCPServerAuth(serverURL string) mcp.OAuthDiscoveryResult {
    return mcp.DiscoverOAuth(serverURL)
}
```

### 2. Frontend — Auto-discovery no blur da URL

**Arquivo**: `frontend/src/components/mcp/McpConnectionSection.tsx`

**Comportamento no campo URL (transport `streamable` ou `sse`)**:

- No `onBlur` do campo URL (e também quando transport muda para `streamable`/`sse` se URL já estiver preenchida):
  1. Se a URL é válida (começa com `https://`) e mudou desde o último discovery
  2. Mostrar indicador de loading discreto no campo (pode ser um spinner inline ou texto "Verificando...")
  3. Chamar `DiscoverMCPServerAuth(url)` (novo binding Wails)
  4. Se `result.found`:
     - Preencher `authType` com `result.authType`
     - Preencher `oauth2AuthUrl` com `result.authUrl`
     - Preencher `oauth2TokenUrl` com `result.tokenUrl`
     - Preencher `oauth2Scopes` com `result.scopes.join(' ')`
     - Marcar esses campos como **auto-discovered** (readonly visualmente, com hint explicando)
     - Mostrar hint de sucesso: "Configuração OAuth detectada automaticamente ({resourceName})" ou similar
  5. Se `!result.found`:
     - Não alterar campos existentes
     - Mostrar hint neutro: "Servidor não expõe metadados OAuth. Configure manualmente se necessário."
     - Campos permanecem editáveis normalmente

**Estado de discovery**: adicionar ao componente (ou como props da McpPage):
```typescript
// Novo estado no componente pai (McpPage.tsx)
const [discoveryStatus, setDiscoveryStatus] = useState<'idle' | 'loading' | 'found' | 'not_found'>('idle');
const [discoveredFields, setDiscoveredFields] = useState<Set<string>>(new Set());
// discoveredFields controla quais campos foram auto-preenchidos e devem ficar readonly
```

**UX dos campos auto-discovered**:
- Campos preenchidos por discovery ficam `readOnly` com visual diferenciado (opacidade reduzida ou borda tracejada, como preferir)
- Um link/botão discreto "Editar manualmente" ao lado do bloco de auth remove o readonly e permite edição livre
- Se o usuário limpar a URL ou mudar para outra, resetar o discovery

### 3. Trigger duplo: blur da URL E mudança de transport

No `McpPage.tsx`, adicionar efeito que dispara discovery quando:
- URL muda (blur) E transport é `streamable` ou `sse`
- Transport muda para `streamable`/`sse` E URL já está preenchida

Isso cobre os dois fluxos:
- Usuário escolhe transport primeiro, depois cola URL → discovery no blur
- Usuário cola URL primeiro, depois muda transport → discovery na mudança

### 4. Prop drilling: passar discovery state para McpConnectionSection

Adicionar props ao `McpConnectionSection`:

```typescript
// Novas props
discoveryStatus: 'idle' | 'loading' | 'found' | 'not_found';
discoveredFields: Set<string>; // ex: new Set(['authType', 'oauth2AuthUrl', 'oauth2TokenUrl', 'oauth2Scopes'])
onManualOverride: () => void; // limpa discoveredFields, permite edição
onUrlBlur: () => void; // dispara discovery
```

Os campos de auth OAuth2 verificam se estão em `discoveredFields` para decidir `readOnly`.

## Arquivos a modificar

1. **`internal/mcp/discovery.go`** — NOVO — lógica de discovery HTTP
2. **`app.go`** — adicionar binding `DiscoverMCPServerAuth`
3. **`frontend/src/pages/McpPage.tsx`** — estado de discovery + callback + efeito de trigger
4. **`frontend/src/components/mcp/McpConnectionSection.tsx`** — onBlur na URL, props de discovery, readOnly condicional, hints

## Arquivos de referência (leia para entender padrões)

- `internal/mcp/types.go` — tipos `ServerConfig`, `AuthType`
- `internal/mcp/oauth.go` — fluxos OAuth2 existentes (Client Credentials + PKCE)
- `internal/mcp/manager.go` — `buildHTTPClientForAuth()` como referência de como auth é consumido
- `frontend/src/store/mcpStore.ts` — store Zustand existente
- `frontend/src/components/mcp/McpGeneralSection.tsx` — componente irmão
- `app.go` — bindings Wails existentes (buscar por `func (a *App).*MCP` para ver o padrão)

## Respostas dos well-known para referência

### Slack — Protected Resource (`https://mcp.slack.com/.well-known/oauth-protected-resource`)
```json
{
  "authorization_servers": ["https://mcp.slack.com"],
  "bearer_methods_supported": ["header", "form"],
  "resource": "https://mcp.slack.com",
  "resource_name": "Slack API",
  "scopes_supported": ["search:read.public", "search:read.private", "search:read.mpim", "search:read.im", "search:read.files", "search:read.users", "chat:write", "channels:history", "groups:history", "mpim:history", "im:history", "canvases:read", "canvases:write", "users:read", "users:read.email"]
}
```

### Slack — Auth Server (`https://mcp.slack.com/.well-known/oauth-authorization-server`)
```json
{
  "authorization_endpoint": "https://slack.com/oauth/v2_user/authorize",
  "code_challenge_methods_supported": ["S256"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "issuer": "https://slack.com",
  "response_types_supported": ["code"],
  "scopes_supported": ["search:read.public", "search:read.private", "..."],
  "token_endpoint": "https://slack.com/api/oauth.v2.user.access"
}
```

### Atlassian — Auth Server (`https://mcp.atlassian.com/.well-known/oauth-authorization-server`)
```json
{
  "authorization_endpoint": "https://mcp.atlassian.com/v1/authorize",
  "code_challenge_methods_supported": ["plain", "S256"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "issuer": "https://cf.mcp.atlassian.com",
  "registration_endpoint": "https://cf.mcp.atlassian.com/v1/register",
  "token_endpoint": "https://cf.mcp.atlassian.com/v1/token"
}
```

## Notas importantes

- **Acessibilidade**: campos readOnly devem ser anunciados corretamente por screen readers. Usar `aria-readonly="true"` e incluir hint descritivo (ex: "preenchido automaticamente via discovery").
- **Não bloquear o save**: se o discovery falhar, o formulário funciona normalmente — é apenas uma conveniência.
- **client_id**: NÃO temos como descobrir o client_id automaticamente via well-known (ele vem do registro do app). O campo client_id permanece editável sempre. No caso do Atlassian, existe um `registration_endpoint` — mas implementar Dynamic Client Registration (RFC 7591) é escopo futuro, não agora. Deixar o campo em branco com hint se `registrationUrl` estiver presente.
- **Não precisa de testes por agora**: foco na implementação funcional.
