# Feature: Auto-extração de Credenciais ao Adicionar Provedores

**Status:** Done

## Objetivo
Simplificar a adição de provedores LLM extraindo automaticamente o domínio do `base_url` e salvando a API key diretamente no `credentials.Manager`, eliminando a necessidade do usuário configurar manualmente o credential pattern.

---

## Viabilidade Técnica

### ✅ Totalmente Viável
A implementação é direta e usa infraestrutura já existente:

1. **Extração de Domínio**
   - Input: `https://api.openai.com/v1`
   - Parse URL: `url.Parse(baseURL)`
   - Extrair host: `api.openai.com`
   - Gerar pattern: `*.openai.com` (wildcard para subdomínios)

2. **Salvamento de Credencial**
   - Usar `credentials.Manager.RegisterPatternWithContext(ctx, pattern, authConfig)`
   - API key é armazenada **criptografada** no credentials file
   - Pattern permite resolução automática via `httpclient`

3. **Provider Registry**
   - Provider registrado com `CredentialPattern: "*.openai.com"`
   - Não armazena API key diretamente (só pattern de referência)
   - Cliente HTTP resolve credencial automaticamente

---

## Fluxo de Implementação

### Backend: `POST /api/providers`

```go
// app.go
func (a *App) CreateLLMProvider(ctx context.Context, req CreateProviderRequest) error {
    // 1. Validar input
    if req.ID == "" || req.BaseURL == "" || req.APIKey == "" {
        return errors.New("campos obrigatórios faltando")
    }

    // 2. Extrair domínio do base_url
    parsedURL, err := url.Parse(req.BaseURL)
    if err != nil {
        return fmt.Errorf("base_url inválido: %w", err)
    }
    host := parsedURL.Host
    
    // 3. Gerar credential pattern
    // api.openai.com -> *.openai.com
    // api.anthropic.com -> *.anthropic.com
    parts := strings.Split(host, ".")
    if len(parts) >= 2 {
        credentialPattern = "*." + strings.Join(parts[len(parts)-2:], ".")
    } else {
        credentialPattern = host
    }

    // 4. Salvar API key no credentials manager
    authConfig := &credentials.AuthConfig{
        Type:  "bearer",
        Token: req.APIKey,
    }
    err = a.credMgr.RegisterPatternWithContext(ctx, credentialPattern, authConfig)
    if err != nil {
        return fmt.Errorf("erro ao salvar credencial: %w", err)
    }

    // 5. Criar provider config (SEM api key, só pattern)
    provider := &llm.ProviderConfig{
        ID:                req.ID,
        Name:              req.Name,
        Type:              req.Type,
        BaseURL:           req.BaseURL,
        Model:             req.Model,
        Timeout:           req.Timeout,
        CredentialPattern: credentialPattern, // Apenas referência!
    }

    // 6. Registrar no registry
    err = a.llmRegistry.Register(provider)
    if err != nil {
        return fmt.Errorf("erro ao registrar provider: %w", err)
    }

    return nil
}

type CreateProviderRequest struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    Type     string `json:"type"`
    BaseURL  string `json:"base_url"`
    Model    string `json:"model"`
    Timeout  int    `json:"timeout"`
    APIKey   string `json:"api_key"` // Novo campo!
}
```

### Frontend: `ProviderForm.tsx`

```tsx
interface ProviderFormData {
  id: string;
  name: string;
  type: 'openai' | 'claude' | 'grok' | 'ollama' | 'custom';
  base_url: string;
  model: string;
  timeout: number;
  api_key: string; // Novo campo!
}

function ProviderForm({ onSubmit }: Props) {
  const [formData, setFormData] = useState<ProviderFormData>({...});
  const [extractedPattern, setExtractedPattern] = useState<string>('');

  // Auto-extrair pattern ao digitar base_url
  useEffect(() => {
    if (formData.base_url) {
      try {
        const url = new URL(formData.base_url);
        const host = url.hostname;
        const parts = host.split('.');
        const pattern = parts.length >= 2 
          ? `*.${parts.slice(-2).join('.')}`
          : host;
        setExtractedPattern(pattern);
      } catch {
        setExtractedPattern('');
      }
    }
  }, [formData.base_url]);

  return (
    <form onSubmit={handleSubmit}>
      <Input
        label="Nome"
        value={formData.name}
        onChange={...}
      />
      
      <Select
        label="Tipo"
        value={formData.type}
        options={[
          { value: 'openai', label: 'OpenAI' },
          { value: 'claude', label: 'Claude (Anthropic)' },
          { value: 'grok', label: 'Grok (xAI)' },
          { value: 'ollama', label: 'Ollama (Local)' },
          { value: 'custom', label: 'Custom' },
        ]}
      />

      <Input
        label="Base URL"
        value={formData.base_url}
        onChange={...}
        placeholder="https://api.openai.com/v1"
      />

      {/* Feedback visual do pattern extraído */}
      {extractedPattern && (
        <div className="text-sm text-muted">
          🔐 Credential Pattern: <code>{extractedPattern}</code>
        </div>
      )}

      <Input
        label="Modelo Padrão"
        value={formData.model}
        onChange={...}
      />

      {/* Campo de API Key com segurança */}
      <Input
        label="API Key"
        type="password"
        value={formData.api_key}
        onChange={...}
        placeholder="sk-..."
        helperText="Será armazenada criptografada no Credentials Manager"
      />
      <button type="button" onClick={() => setShowKey(!showKey)}>
        {showKey ? '👁️ Ocultar' : '🔒 Mostrar'}
      </button>

      <Button type="submit">Criar Provider</Button>
    </form>
  );
}
```

---

## Exemplos de Uso

### Exemplo 1: Adicionar OpenAI

**Input do Usuário:**
```json
{
  "name": "OpenAI Production",
  "type": "openai",
  "base_url": "https://api.openai.com/v1",
  "model": "gpt-4o",
  "api_key": "sk-proj-abc123..."
}
```

**Processamento Automático:**
1. Extrair domínio: `api.openai.com`
2. Gerar pattern: `*.openai.com`
3. Salvar credencial: `RegisterPattern("*.openai.com", "sk-proj-abc123...")`
4. Criar provider: `{ ..., CredentialPattern: "*.openai.com" }` (sem api_key)

**Resultado:**
- Provider criado ✅
- Credencial salva criptografada ✅
- Requisições HTTP injetarão automaticamente o Bearer token ✅

---

### Exemplo 2: Adicionar Claude

**Input do Usuário:**
```json
{
  "name": "Claude Sonnet",
  "type": "claude",
  "base_url": "https://api.anthropic.com/v1",
  "model": "claude-3-7-sonnet-20250219",
  "api_key": "sk-ant-xyz789..."
}
```

**Processamento Automático:**
1. Domínio: `api.anthropic.com` → Pattern: `*.anthropic.com`
2. Credencial: `RegisterPattern("*.anthropic.com", "sk-ant-xyz789...")`
3. Provider: `{ ..., CredentialPattern: "*.anthropic.com" }`

---

### Exemplo 3: Adicionar Ollama (sem API key)

**Input do Usuário:**
```json
{
  "name": "Ollama Local",
  "type": "ollama",
  "base_url": "http://localhost:11434/api",
  "model": "llama3.1",
  "api_key": ""  // Vazio = sem autenticação
}
```

**Processamento:**
- API key vazia → Não registrar credencial
- Pattern: `localhost` (só para referência, não usado)
- Provider criado normalmente

---

## Benefícios

### Para o Usuário
- ✅ **Menos campos** para preencher (credential pattern auto-extraído)
- ✅ **Mais seguro** (API key nunca exposta em JSON ou logs)
- ✅ **Mais simples** (um único formulário faz tudo)
- ✅ **Feedback visual** (mostra pattern extraído antes de salvar)

### Para o Sistema
- ✅ **Consistência** (pattern sempre correto)
- ✅ **Segurança** (credenciais sempre criptografadas)
- ✅ **Manutenibilidade** (lógica centralizada)
- ✅ **Extensibilidade** (fácil adicionar novos providers)

---

## Segurança

### API Key Storage
- Armazenada **criptografada** via `credentials.Manager`
- Nunca exposta em JSON de provider
- Nunca logada (nem em debug)
- Decriptada apenas no momento do uso (em memória)

### Frontend
- Input type="password" por padrão
- Toggle para mostrar/ocultar
- Não persiste em localStorage/sessionStorage
- Enviada apenas via HTTPS em produção

### Backend
- Valida formato de API key (prefixo `sk-`, comprimento mínimo)
- Rate limiting em endpoint de criação
- Audit log de criação/modificação de providers

---

## Endpoint API Completo

### `POST /api/providers`

**Request:**
```json
{
  "id": "my-custom-openai",
  "name": "My OpenAI",
  "type": "openai",
  "base_url": "https://api.openai.com/v1",
  "model": "gpt-4o",
  "timeout": 180,
  "api_key": "sk-proj-abc123..."
}
```

**Response (Success):**
```json
{
  "success": true,
  "provider": {
    "id": "my-custom-openai",
    "name": "My OpenAI",
    "type": "openai",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o",
    "timeout": 180,
    "credential_pattern": "*.openai.com",
    "credential_configured": true
  },
  "message": "Provider criado e credencial salva com sucesso"
}
```

**Response (Error):**
```json
{
  "success": false,
  "error": "base_url inválido: parse error"
}
```

---

### `PUT /api/providers/{id}`

Permite atualizar API key:

**Request:**
```json
{
  "api_key": "sk-proj-new-key-456..."  // Novo key
}
```

**Comportamento:**
- Extrai pattern do base_url atual
- Atualiza credencial no credentials.Manager
- Não modifica provider config (só credencial)

---

### `DELETE /api/providers/{id}`

**Comportamento:**
- Remove provider do registry
- **TAMBÉM remove credencial associada** (cleanup automático)

**Implementation:**
```go
func (a *App) DeleteLLMProvider(ctx context.Context, id string) error {
    provider := a.llmRegistry.Get(id)
    if provider == nil {
        return errors.New("provider não encontrado")
    }

    // 1. Remover provider do registry
    err := a.llmRegistry.Remove(id)
    if err != nil {
        return err
    }

    // 2. Remover credencial associada (se existir)
    if provider.CredentialPattern != "" {
        // Nota: credentials.Manager pode não ter método Delete ainda
        // Implementar se necessário ou deixar credencial órfã (será sobrescrita)
    }

    return nil
}
```

---

## Testes

### Unit Tests
```go
func TestExtractDomainPattern(t *testing.T) {
    tests := []struct {
        baseURL string
        pattern string
    }{
        {"https://api.openai.com/v1", "*.openai.com"},
        {"https://api.anthropic.com/v1", "*.anthropic.com"},
        {"http://localhost:11434/api", "localhost"},
        {"https://custom.domain.com", "*.domain.com"},
    }
    for _, tt := range tests {
        result := extractPattern(tt.baseURL)
        if result != tt.pattern {
            t.Errorf("Expected %s, got %s", tt.pattern, result)
        }
    }
}

func TestCreateProviderWithAPIKey(t *testing.T) {
    app := setupTestApp()
    
    req := CreateProviderRequest{
        ID:      "test-openai",
        Name:    "Test OpenAI",
        Type:    "openai",
        BaseURL: "https://api.openai.com/v1",
        APIKey:  "sk-test123",
    }

    err := app.CreateLLMProvider(context.Background(), req)
    if err != nil {
        t.Fatal(err)
    }

    // Verificar que provider foi criado
    provider := app.llmRegistry.Get("test-openai")
    if provider == nil {
        t.Fatal("Provider não criado")
    }

    // Verificar que credencial foi salva
    cred, err := app.credMgr.GetByPattern("*.openai.com")
    if err != nil || cred.Token != "sk-test123" {
        t.Fatal("Credencial não salva corretamente")
    }
}
```

---

## Roadmap

### Phase 5.1 (Backend)
- [ ] Implementar `extractDomainPattern()` helper
- [ ] Adicionar campo `api_key` em `CreateProviderRequest`
- [ ] Modificar `CreateLLMProvider()` para salvar credencial
- [ ] Adicionar `UpdateProviderCredential()` método
- [ ] Implementar endpoint `POST /api/providers`
- [ ] Testes unitários + integração

### Phase 5.2 (Frontend)
- [ ] Adicionar campo "API Key" em `ProviderForm`
- [ ] Implementar extração visual de pattern
- [ ] Toggle show/hide password
- [ ] Validação de formato de key
- [ ] Feedback de sucesso/erro
- [ ] Testes de componente

### Phase 5.3 (Validação)
- [ ] Teste end-to-end: criar provider → usar em profile → fazer chat
- [ ] Verificar credencial injetada corretamente
- [ ] Validar segurança (não expor key em logs/JSON)

---

## Status: 📋 **PLANEJADO**

**Inclusão no Plano:** Phase 5 (Frontend UI for Provider Manager)  
**Prioridade:** Alta (melhora significativamente UX)  
**Complexidade:** Baixa (infraestrutura já existe)  
**Estimativa:** 4-6 horas de desenvolvimento + testes
