# HTTP Request - Exemplos de Uso Seguro

## Setup Inicial (em app.go ou main.go)

```go
// Criar instância com segurança
httpReqTool := web.NewHTTPRequest()

// 1. Configurar callback de confirmação (operações destrutivas)
httpReqTool.SetConfirmFunc(func(ctx context.Context, method, url, body string) (bool, error) {
    // Mostra dialog ao usuário pedindo confirmação
    // Retorna true se aprovado, false se negado
    return askUserConfirmation(method, url, body)
})

// 2. Carregar credenciais (de ambiente, vault, config, etc.)
httpReqTool.SetCredential("github_token", os.Getenv("GITHUB_TOKEN"))
httpReqTool.SetCredential("stripe_key", loadFromVault("stripe_secret"))
httpReqTool.SetCredential("api_password", getSecurePassword())

// 3. Registrar a tool
toolRegistry.MustRegister(httpReqTool)
```

## Uso pelo Modelo LLM

### ✅ Exemplo 1: GET simples (sem confirmação)
```json
{
  "url": "https://api.github.com/repos/inclunet/assistente"
}
```
**Resultado**: Executa imediatamente, retorna dados do repositório.

---

### ✅ Exemplo 2: POST com credencial segura
```json
{
  "url": "https://api.github.com/repos/inclunet/assistente/issues",
  "method": "POST",
  "credential_key": "github_token",
  "body": "{\"title\": \"Nova feature\", \"body\": \"Descrição da issue\"}"
}
```
**Resultado**: 
- Executa imediatamente (POST não exige confirmação)
- Token não aparece no contexto do modelo
- Header enviado: `Authorization: Bearer ghp_...` (resolvido em runtime)

---

### ⚠️ Exemplo 3: DELETE com confirmação
```json
{
  "url": "https://api.github.com/repos/inclunet/assistente/issues/42",
  "method": "DELETE",
  "credential_key": "github_token"
}
```
**Fluxo**:
1. Sistema detecta `DELETE` → **Pausa execução**
2. Mostra dialog ao usuário:
   ```
   Confirmar operação DELETE?
   
   DELETE https://api.github.com/repos/inclunet/assistente/issues/42
   Body: (sem body)
   
   [Permitir] [Negar]
   ```
3. **Se usuário clica "Permitir"**: Executa, retorna resposta da API
4. **Se usuário clica "Negar"**: Cancela, retorna erro "Operação DELETE cancelada pelo usuário"

---

### ⚠️ Exemplo 4: PUT com corpo e confirmação
```json
{
  "url": "https://api.example.com/users/123",
  "method": "PUT",
  "credential_key": "admin_token",
  "body": "{\"role\": \"admin\", \"active\": true}",
  "body_type": "json"
}
```
**Fluxo**:
1. Sistema detecta `PUT` → **Pausa execução**
2. Mostra dialog:
   ```
   Confirmar operação PUT?
   
   PUT https://api.example.com/users/123
   Body:
   {"role": "admin", "active": true}
   
   [Permitir] [Negar]
   ```
3. Usuário aprova → Executa
4. Usuário nega → Cancela

---

### ✅ Exemplo 5: Basic Auth com password seguro
```json
{
  "url": "https://api.internal.com/data",
  "auth_basic": {
    "username": "admin",
    "password_key": "admin_password"
  }
}
```
**Resultado**:
- Password resolvido de `credentialStore["admin_password"]`
- Header enviado: `Authorization: Basic YWRtaW46c2VjcmV0...` (base64 de admin:secret)
- Password nunca aparece no contexto do modelo

---

## Comparação: Seguro vs Inseguro

### ❌ INSEGURO (não faça isso)
```json
{
  "url": "https://api.github.com/repos/...",
  "auth_bearer": "ghp_1234567890abcdefghijklmnop"  ← Token exposto!
}
```
**Problemas**:
- Token visível no contexto do modelo
- Pode vazar em logs, debugging, exports
- Se o modelo for comprometido, token é exposto

### ✅ SEGURO (faça assim)
```json
{
  "url": "https://api.github.com/repos/...",
  "credential_key": "github_token"  ← Apenas referência
}
```
**Benefícios**:
- Token nunca sai do sistema
- Contexto do modelo não contém dados sensíveis
- Fácil rotacionar (muda no store, modelo não muda)

---

## Casos de Uso Reais

### Caso 1: Criar issue no GitHub
```go
// Setup (uma vez)
httpReqTool.SetCredential("github_token", os.Getenv("GITHUB_TOKEN"))
```

```json
// Modelo usa (sempre)
{
  "url": "https://api.github.com/repos/inclunet/assistente/issues",
  "method": "POST",
  "credential_key": "github_token",
  "body": "{\"title\": \"Bug no editor\", \"labels\": [\"bug\"]}"
}
```

### Caso 2: Atualizar configuração (com confirmação)
```go
// Setup
httpReqTool.SetCredential("api_key", loadFromVault("production_api_key"))
```

```json
// Modelo solicita
{
  "url": "https://api.production.com/config",
  "method": "PUT",
  "credential_key": "api_key",
  "body": "{\"max_users\": 1000, \"rate_limit\": 100}"
}
```
→ Usuário vê confirmação → Aprova → Executa

### Caso 3: Deletar recurso temporário (com confirmação)
```go
// Setup
httpReqTool.SetCredential("admin_token", getAdminToken())
```

```json
// Modelo solicita
{
  "url": "https://api.example.com/temp/sessions/abc123",
  "method": "DELETE",
  "credential_key": "admin_token"
}
```
→ Usuário vê confirmação → Aprova → Deleta

---

## Troubleshooting

### Erro: "Credencial 'xxx' não encontrada"
**Causa**: Credencial não foi registrada com `SetCredential()`.

**Solução**:
```go
httpReqTool.SetCredential("xxx", "seu_valor_aqui")
```

### Erro: "Operação DELETE cancelada pelo usuário"
**Causa**: Usuário clicou em "Negar" na confirmação.

**Solução**: Normal. Operação foi cancelada por segurança.

### Operação não pede confirmação
**Causa**: `confirmFn` não foi configurado.

**Solução**:
```go
httpReqTool.SetConfirmFunc(yourConfirmCallback)
```

---

## Checklist de Segurança

Antes de usar `http_request` em produção:

- [ ] Callback de confirmação configurado para DELETE/PUT/PATCH
- [ ] Todas as credenciais carregadas via `SetCredential()`
- [ ] Credenciais **nunca** hardcoded no código
- [ ] Credenciais carregadas de environment/vault/config
- [ ] Modelo instruído a usar `credential_key` em vez de tokens diretos
- [ ] Logs configurados para não logar valores de tokens
- [ ] Auditoria de operações destrutivas ativada
- [ ] Testes cobrem tanto credenciais válidas quanto inválidas

---

## Performance

- **Confirmação**: Bloqueia thread até resposta do usuário (~1-30s depende do usuário)
- **Resolução de credenciais**: O(1) lookup em hashmap (~nanosegundos)
- **Impacto**: Desprezível, exceto pelo tempo de confirmação humana

**Tip**: Para scripts automatizados, considere pre-aprovar operações conhecidas.
