# HTTP Request - Guardrails de Segurança

**Status:** In Progress — guardrails implementados; matriz de confirmação PUT/PATCH permanece sem regressão

> **Contrato vigente:** `HTTPRequest` recebe `credentials.Manager` em
> `NewHTTPRequest(credMgr)` e delega autenticação ao cliente central de
> `internal/tools/http`, que resolve credenciais pela URL. Não existem
> `SetCredential`, `credential_key`, `auth_bearer` ou `auth_basic` no contrato
> da tool. O desenho por chave descrito abaixo é histórico e foi superseded
> pelas AEPs 0018 e 0019.

## Problema

Ao dar ao modelo LLM acesso a uma ferramenta HTTP completa, surgem dois riscos principais:

1. **Operações destrutivas acidentais**: O modelo pode executar DELETE/PUT/PATCH sem intenção
2. **Exposição de credenciais**: Tokens e API keys ficam visíveis no contexto do modelo

## Solução Implementada

### 1. Confirmação de Operações Destrutivas

#### Como funciona
- Métodos `DELETE`, `PUT` e `PATCH` **sempre** exigem confirmação do usuário
- `GET`, `POST`, `HEAD`, `OPTIONS` executam sem confirmação (considerados "seguros")
- UI mostra: método, URL, preview do body
- Usuário pode aprovar ou negar

#### Implementação
```go
// No app.go, ao registrar http_request:
httpReqTool := web.NewHTTPRequest()
httpReqTool.SetConfirmFunc(func(ctx context.Context, method, url, body string) (bool, error) {
    // Mostra dialog de confirmação ao usuário
    resp, err := questionnaireMgr.RequestQuestionnaire(ctx, ...)
    return approved, nil
})
```

#### Exemplo de uso
```
Modelo: Quero deletar o usuário 123
↓
Sistema: Solicita confirmação ao usuário
↓
UI: "Confirmar operação DELETE?
     DELETE https://api.example.com/users/123
     Permitir | Negar"
↓
Usuário: Clica em "Permitir" ou "Negar"
↓
Sistema: Executa ou cancela a operação
```

#### Bypass para testes
Testes unitários desabilitam confirmação definindo `confirmFn = nil`.

---

### 2. Gestão segura de credenciais — design histórico superseded

#### Problema
❌ **Modelo vê o token**:
```json
{
  "url": "https://api.github.com/repos/...",
  "auth_bearer": "ghp_1234567890abcdef..."  ← EXPOSTO NO CONTEXTO
}
```

#### Solução originalmente proposta
✅ **Usar chaves de credenciais**:
```json
{
  "url": "https://api.github.com/repos/...",
  "credential_key": "github_token"  ← Apenas a referência
}
```

#### Como funciona
1. **Armazenar credenciais** antes de registrar a tool:
   ```go
   httpReqTool := web.NewHTTPRequest()
   httpReqTool.SetCredential("github_token", os.Getenv("GITHUB_TOKEN"))
   httpReqTool.SetCredential("stripe_key", loadFromVault("stripe_secret"))
   ```

2. **Modelo usa chave, não o valor**:
   ```json
   {
     "credential_key": "github_token"
   }
   ```

3. **Sistema resolve em runtime** (transparente ao modelo):
   ```go
   token := credentialStore["github_token"]  // "ghp_1234..."
   req.Header.Set("Authorization", "Bearer " + token)
   ```

#### Três formas de autenticação

**Opção 1: Bearer com chave armazenada** (RECOMENDADO)
```json
{
  "url": "https://api.example.com/data",
  "credential_key": "api_token"
}
```

**Opção 2: Bearer com token direto** (não recomendado, expõe token)
```json
{
  "url": "https://api.example.com/data",
  "auth_bearer": "abc123token"
}
```

**Opção 3: Basic Auth com password armazenado** (RECOMENDADO)
```json
{
  "url": "https://api.example.com/data",
  "auth_basic": {
    "username": "admin",
    "password_key": "admin_password"
  }
}
```

**Opção 4: Basic Auth com password direto** (não recomendado)
```json
{
  "url": "https://api.example.com/data",
  "auth_basic": {
    "username": "admin",
    "password": "secret123"
  }
}
```

---

### 3. Proteção anti-SSRF (validação pós-DNS)

As tools de rede (`web_fetch`, `http_request`, `feed_read`) compartilham a barreira
anti-SSRF em `internal/tools/http`. A proteção é feita em camadas:

1. **Pré-dial (textual)** — `IsPrivateHost` rejeita rapidamente URLs cujo host já é
   um IP literal local/privado (loopback, RFC 1918, CGNAT 100.64/10, link-local
   incl. `169.254.169.254`, multicast, broadcast) ou `localhost`/`.localhost`.
2. **Redirects** — `RedirectGuard` reaplica a política nos redirects (que o net/http
   segue automaticamente) e remove headers sensíveis ao cruzar limite de confiança.
3. **Pós-DNS (definitiva)** — `SetTransportGuard`/`NewGuardedTransport` instalam um
   `net.Dialer` com hook `Control` que **valida o IP REAL no momento do connect**,
   após a resolução de DNS.

> **Validação pós-DNS (issue #237):** validar apenas o host textual não basta. Um
> hostname público que resolve para um IP privado (DNS rebinding, CNAME para
> `169.254.169.254`) e formas numéricas não-padrão (`http://2130706433/`,
> `http://0x7f000001/`, `http://[::ffff:127.0.0.1]/`) burlavam a checagem textual. O
> `GuardedTransport` usa um `net.Dialer` com `Control func(network, address string,
> c syscall.RawConn) error`, chamado imediatamente antes de cada `connect()` com o
> `address` já no formato IP:porta concreto. O `Control` aplica `isBlockedIP` ao IP
> real e retorna erro (**fail-closed**) se for local/privado, abortando aquela
> tentativa — um host que só resolve para IPs privados falha por completo. Como o IP
> validado é exatamente o que será conectado, não há TOCTOU; e por usar o dialer
> nativo preserva-se o **Happy Eyeballs** (tentativas IPv6/IPv4 concorrentes com
> fallback), sem regressão de latência. Como o `http.Transport` é reusado, os
> redirects passam pelo mesmo guard. A lista de ranges é centralizada em
> `isBlockedIP` (`ssrf.go`), fonte única de verdade compartilhada pelas duas camadas,
> e normaliza IPv4-mapped IPv6 via `To4()` para fechar bypass de broadcast/privado
> em forma mapeada.

> **Autorização explícita para destinos bloqueados (AEP-0082):** o hard-deny
> anti-SSRF deixou de ser terminal. Quando o guard pós-DNS barra um destino
> (`BlockedIPError`) e há um `NetworkAuthorizer` configurado, o `Client.Do`
> abre um fluxo de **consentimento explícito + allowlist escopável** (sessão /
> workspace / perfil / global) e **reexecuta** a request liberando apenas o(s)
> **IP(s) resolvido(s)** do host autorizado (trust por-request via
> `WithTrustedIPs`, nunca a faixa inteira). Sem authorizer ou com o usuário
> negando, o erro passa a ser acionável (`BlockedDestinationError`: host, IP,
> categoria, ações). Detalhes e decisões em `aep/0082-network-trust-allowlist.md`.

> **Proxy desabilitado (conexão direta obrigatória):** o `GuardedTransport` define
> `Transport.Proxy = nil`, **ignorando `HTTP_PROXY`/`HTTPS_PROXY`/`ALL_PROXY`**. Isso
> é deliberado: com um proxy ativo, o `DialContext` validaria/dialaria o IP do
> *proxy*, não o do destino final — o que reabriria o bypass anti-SSRF (uma URL para
> IP privado seria alcançada através de um proxy público, sem o IP de destino ser
> validado pós-DNS) e quebraria a garantia desta camada. A política é sempre conexão
> direta com validação do IP real. **Implicação:** em ambientes que dependem de proxy
> corporativo para saída à internet, estas tools (`web_fetch`, `http_request`,
> `feed_read`) não usarão o proxy; um eventual suporte a proxy exigiria uma política
> explícita que valide o destino final antes de delegar ao proxy (não implementado).

---

## Configuração por chave — exemplos históricos, não usar

### Carregar credenciais de variáveis de ambiente

**`app.go`**:
```go
httpReqTool := web.NewHTTPRequest()
httpReqTool.SetConfirmFunc(confirmCallback)

// Carrega credenciais de variáveis de ambiente
if token := os.Getenv("GITHUB_TOKEN"); token != "" {
    httpReqTool.SetCredential("github_token", token)
}
if token := os.Getenv("OPENAI_API_KEY"); token != "" {
    httpReqTool.SetCredential("openai_key", token)
}
if pwd := os.Getenv("ADMIN_PASSWORD"); pwd != "" {
    httpReqTool.SetCredential("admin_password", pwd)
}

a.toolRegistry.MustRegister(httpReqTool)
```

### Carregar de arquivo de configuração

**`config.json`**:
```json
{
  "http_credentials": {
    "github_token": "ghp_...",
    "stripe_key": "sk_test_...",
    "admin_password": "..."
  }
}
```

**`app.go`**:
```go
// Carrega config
cfg := loadConfig()
httpReqTool := web.NewHTTPRequest()

for key, value := range cfg.HTTPCredentials {
    httpReqTool.SetCredential(key, value)
}
```

### Carregar de vault/secret manager (produção)

```go
httpReqTool := web.NewHTTPRequest()

// Exemplo com AWS Secrets Manager
githubToken, _ := awsSecretsManager.GetSecret("prod/github_token")
httpReqTool.SetCredential("github_token", githubToken)

// Exemplo com HashiCorp Vault
stripeKey, _ := vaultClient.Logical().Read("secret/data/stripe_key")
httpReqTool.SetCredential("stripe_key", stripeKey.Data["value"].(string))
```

---

## Benefícios

### Segurança
1. ✅ Nenhuma operação destrutiva sem aprovação explícita
2. ✅ Credenciais nunca aparecem no contexto do modelo
3. ✅ Logs não contêm tokens (apenas chaves de referência)
4. ✅ Auditoria completa (método, URL, timestamp, aprovação)

### Usabilidade
1. ✅ Modelo usa chaves simples: `"credential_key": "github_token"`
2. ✅ Sem necessidade de expor tokens na conversa
3. ✅ Confirmação visual clara e segura
4. ✅ Fácil rotação de credenciais (atualiza no store, modelo não muda)

### Operacional
1. ✅ Credenciais centralizadas (uma fonte da verdade)
2. ✅ Compatível com secret managers corporativos
3. ✅ Fácil adicionar/remover credenciais em runtime
4. ✅ Testes podem usar credenciais fake sem modificar código

---

## Exemplos práticos do design histórico

### Exemplo 1: GitHub API (seguro)

**Setup**:
```go
httpReqTool.SetCredential("github_token", os.Getenv("GITHUB_TOKEN"))
```

**Modelo usa**:
```json
{
  "url": "https://api.github.com/repos/inclunet/assistente/issues",
  "method": "POST",
  "credential_key": "github_token",
  "body": "{\"title\": \"Bug fix\", \"body\": \"Descrição...\"}"
}
```

**Headers enviados** (transparente):
```
Authorization: Bearer ghp_1234567890abcdef...
```

### Exemplo 2: DELETE com confirmação

**Modelo solicita**:
```json
{
  "url": "https://api.example.com/users/123",
  "method": "DELETE",
  "credential_key": "admin_token"
}
```

**Sistema**:
1. Detecta método `DELETE` → dispara confirmação
2. Mostra UI: "Confirmar DELETE https://api.example.com/users/123?"
3. Usuário aprova → executa
4. Usuário nega → retorna erro "Operação DELETE cancelada pelo usuário"

### Exemplo 3: Multiple credentials

```go
// Setup inicial
httpReqTool.SetCredential("github_token", "ghp_...")
httpReqTool.SetCredential("stripe_key", "sk_test_...")
httpReqTool.SetCredential("slack_webhook", "https://hooks.slack.com/...")
```

**Modelo pode usar qualquer uma**:
```json
// Para GitHub
{"credential_key": "github_token"}

// Para Stripe
{"credential_key": "stripe_key"}

// Para Slack
{"credential_key": "slack_webhook"}
```

---

## Limitações e Considerações

### Limitações
1. **Primeira execução**: Credenciais precisam estar configuradas antes de usar
2. **Runtime apenas**: Não persiste credenciais (segurança por design)
3. **Confirmação não é async**: Operações destrutivas bloqueiam até aprovação

### Considerações de Segurança
1. **Não logar tokens**: Logs devem usar apenas chaves de referência
2. **Rotacionar regularmente**: Credenciais devem ter tempo de vida limitado
3. **Princípio do menor privilégio**: Cada credencial deve ter apenas permissões necessárias
4. **Auditoria**: Registrar todas as operações (método, URL, timestamp, usuário que aprovou)

### Boas Práticas
1. ✅ Sempre usar `credential_key` em vez de tokens diretos
2. ✅ Carregar credenciais de environment/vault, nunca hardcode
3. ✅ Nomear credenciais de forma descritiva: `github_token`, não `token1`
4. ✅ Documentar quais credenciais cada skill/profile precisa
5. ✅ Revisar confirmações de DELETE/PUT antes de aprovar

---

## Roadmap Futuro

### Features planejadas
- [ ] UI para gerenciar credenciais (adicionar/remover sem código)
- [ ] Expiração automática de credenciais
- [ ] Rate limiting por credencial
- [ ] Logs de auditoria detalhados
- [ ] Integração com OAuth 2.0 flows
- [ ] Suporte a multiple environments (dev/staging/prod)

### Melhorias de UX
- [ ] Preview de diff para PUT/PATCH (mostrar o que vai mudar)
- [ ] Histórico de operações aprovadas/negadas
- [ ] Templates de aprovação (auto-aprovar DELETE de recursos de teste)
- [ ] Dry-run mode (simular sem executar)

## Critérios e evidências do escopo entregue

- [x] `internal/tools/web/http_request.go` encaminha `DELETE`, `PUT` e `PATCH`
  ao callback de confirmação.
- [x] `internal/tools/web/http_request_test.go` cobre DELETE sem callback e
  DELETE negado/cancelado pelo callback.
- [ ] Cobrir aprovação de DELETE e adicionar regressões de aprovação/negação
  equivalentes para PUT e PATCH.
- [x] O modelo não recebe token, senha ou chave de credencial no schema da
  tool.
- [x] O `credentials.Manager` é injetado no cliente HTTP central, que resolve
  autenticação por URL.
- [x] Proteções anti-SSRF pré/pós-DNS e em redirects vivem em
  `internal/tools/http` e possuem regressões próprias.
- [x] Ausência dos métodos históricos `SetCredential` e dos argumentos
  `credential_key`/`auth_*` está reconciliada como substituição arquitetural,
  não como funcionalidade pendente.
