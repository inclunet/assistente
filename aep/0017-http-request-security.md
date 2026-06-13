# HTTP Request - Guardrails de Segurança

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

### 2. Gestão Segura de Credenciais

#### Problema
❌ **Modelo vê o token**:
```json
{
  "url": "https://api.github.com/repos/...",
  "auth_bearer": "ghp_1234567890abcdef..."  ← EXPOSTO NO CONTEXTO
}
```

#### Solução
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
   `DialContext` que **valida o IP REAL após a resolução de DNS** e só conecta
   fixando o IP já validado.

> **Validação pós-DNS (issue #237):** validar apenas o host textual não basta. Um
> hostname público que resolve para um IP privado (DNS rebinding, CNAME para
> `169.254.169.254`) e formas numéricas não-padrão (`http://2130706433/`,
> `http://0x7f000001/`, `http://[::ffff:127.0.0.1]/`) burlavam a checagem textual. O
> `GuardedTransport` resolve o host, aplica `isBlockedIP` a **cada** IP candidato
> (fail-closed: qualquer candidato privado recusa o host inteiro) e disca no IP já
> validado, fechando o TOCTOU. Como o `http.Transport` é reusado, os redirects
> passam pelo mesmo guard. A lista de ranges é centralizada em `isBlockedIP`
> (`ssrf.go`), fonte única de verdade compartilhada pelas duas camadas. O resolver e
> o dialer são injetáveis para teste (`dialer_test.go`).

---

## Configuração no Sistema

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

## Exemplos Práticos

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
