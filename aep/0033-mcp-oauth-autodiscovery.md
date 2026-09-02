# MCP OAuth Auto-Discovery

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

## O que implementar

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
     - Preencher somente campos ainda vazios, preservando valores manuais
     - Mostrar hint de sucesso: "Configuração OAuth detectada automaticamente ({resourceName})" ou similar
  5. Se o resultado for parcial ou ausente:
     - Não alterar campos existentes
     - Mostrar hint correspondente e permitir conclusão manual
     - Campos permanecem editáveis normalmente

**Estado de discovery**: adicionar ao componente (ou como props da McpPage):
```typescript
const [discoveryStatus, setDiscoveryStatus] = useState<
  'idle' | 'loading' | 'found' | 'partial' | 'not_found'
>('idle');
```

**UX dos campos auto-discovered**:
- O estado encontrado mostra apenas os campos que ainda exigem entrada do usuário
- Um botão "Configurar manualmente" troca para o formulário completo sem apagar
  os valores existentes
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
discoveryStatus: 'idle' | 'loading' | 'found' | 'partial' | 'not_found';
onManualOverride: () => void; // apresenta a configuração manual completa
onUrlBlur: () => void; // dispara discovery
```

O componente deriva a apresentação do `discoveryStatus`; o estado não torna
campos manuais somente leitura nem bloqueia o salvamento.

## Arquivos a modificar

1. **`internal/mcp/discovery.go`** — NOVO — lógica de discovery HTTP
2. **`app.go`** — adicionar binding `DiscoverMCPServerAuth`
3. **`frontend/src/pages/McpPage.tsx`** — estado de discovery + callback + efeito de trigger
4. **`frontend/src/components/mcp/McpConnectionSection.tsx`** — onBlur na URL, props de discovery, estados e hints

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

- **Acessibilidade**: o status do discovery deve ser anunciado corretamente por
  leitores de tela. A ação para abandonar o resultado e configurar manualmente
  permanece disponível por teclado e com nome acessível.
- **Não bloquear o save**: se o discovery falhar, o formulário funciona normalmente — é apenas uma conveniência.
- **client_id**: NÃO temos como descobrir o client_id automaticamente via well-known (ele vem do registro do app). O campo client_id permanece editável sempre. No caso do Atlassian, existe um `registration_endpoint` — mas implementar Dynamic Client Registration (RFC 7591) é escopo futuro, não agora. Deixar o campo em branco com hint se `registrationUrl` estiver presente.

## Evolução: discovery genérico relacionado a caminhos

### Resumo

O discovery passa a considerar a hierarquia completa do caminho do recurso,
sem reconhecer domínio, marca ou fornecedor. A evolução combina as localizações
normativas de RFC 9728, RFC 8414 e OIDC Discovery com os fallbacks relativos
historicamente aceitos pelo Assistente.

### Motivação

Recursos MCP podem estar atrás de gateways com caminhos profundos e não
publicar Protected Resource Metadata na raiz. Reduzir antecipadamente a URL do
recurso à origin impede encontrar metadata OAuth/OIDC válida em um ancestral.
Além disso, tratar discovery como apenas encontrado/não encontrado ocultava
casos em que o PRM existia, mas a configuração ainda precisava ser completada.

### Decisões

1. A URL do recurso é normalizada sem query, fragment ou credenciais. Segmentos
   `.` e `..` são resolvidos antes de qualquer request.
2. Os candidatos PRM são gerados de modo determinístico para recurso,
   ancestrais e origin. Para cada nível, tenta-se primeiro a forma de RFC 9728
   (`/.well-known/oauth-protected-resource{path}`) e depois o fallback relativo
   já suportado (`{path}/.well-known/oauth-protected-resource`), com
   deduplicação. A expansão é limitada a 16 bases: preserva o recurso completo,
   os ancestrais mais próximos e sempre inclui a origin como fallback final.
3. Quando o PRM não informa `authorization_servers`, recurso, ancestrais e
   origin tornam-se bases candidatas. Para cada base são derivados:
   - RFC 8414: `/.well-known/oauth-authorization-server{issuer-path}`;
   - OIDC Discovery: `{issuer}/.well-known/openid-configuration`;
   - somente depois, os fallbacks de localização anteriormente suportados.
4. Respostas diferentes de 200 nunca são metadata. Status, desafio
   `WWW-Authenticate`, `Location` e os campos JSON `error` e
   `error_description` podem virar hints, sempre limitados e saneados.
   Tokens, cookies, credenciais, userinfo, query e fragment não são expostos.
5. Redirects preservam timeout e limite, rejeitam esquema não HTTP(S),
   credenciais na URL, downgrade de HTTPS e mais de cinco saltos.
6. O contrato distingue `complete`, `partial` e `not_found`, além de indicar
   separadamente PRM e Authorization Server Metadata encontrados. Ausência de
   `registration_endpoint` exige conclusão manual, mas não invalida metadata
   OAuth/OIDC.
7. Valores manuais são soberanos: discovery preenche apenas campos vazios,
   nunca impede salvar e mantém a edição manual disponível em falha parcial ou
   total.

### Fases

1. Normalizar e expandir candidatos de PRM e Authorization Server Metadata.
2. Classificar respostas e produzir hints seguros.
3. Expor estados parciais no binding e na configuração MCP.
4. Cobrir backend e frontend com testes locais sem rede externa.

### Riscos

- Mais candidatos aumentam o número máximo de requests. A mitigação é o limite
  de 16 bases, deduplicação, timeout de 5 segundos por request e parada no
  primeiro schema válido.
- Gateways podem devolver conteúdo hostil. Corpos de diagnóstico têm limite
  pequeno, somente JSON escalar explicitamente permitido é aproveitado e HTML
  é descartado.
- Fallbacks legados podem ser ambíguos. Eles permanecem depois das derivações
  normativas, preservando compatibilidade sem alterar sua precedência sobre os
  padrões.

### Critérios de aceitação

- Caminhos profundos geram candidatos ordenados e sem duplicação ou traversal.
- Issuer com path funciona tanto em RFC 8414 quanto em OIDC Discovery.
- Ausência de PRM ainda permite encontrar metadata em bases ancestrais.
- 401/403 fornecem apenas hints saneados e não são aceitos como metadata.
- Redirects seguros funcionam; corpos excessivos são truncados para diagnóstico.
- Falhas parciais e totais são distinguíveis na UI.
- Campos manuais existentes permanecem intactos e o formulário continua
  salvável.
- Backend e frontend possuem testes obrigatórios para esses comportamentos.
