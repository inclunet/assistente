# Feature: Auto-extração de Credenciais ao Adicionar Provedores

**Status:** In Progress — criação/atualização entregues; cleanup de credencial no DELETE permanece pendente

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

## Fluxo REST proposto originalmente (histórico, não implementado)

> Toda esta seção registra a proposta inicial de uma API REST. Os endpoints
> `/api/providers` não existem no runtime atual. `CreateLLMProvider` e os DTOs
> têm contrapartes reais, mas não são handlers HTTP: o binding Wails em
> `internal/wailsapi/llm_providers.go` delega a
> `controllers/LLMController.CreateLLMProvider`, que chama
> `internal/providers/service.go`; o frontend usa os bindings gerados. Os
> snippets abaixo preservam apenas a forma proposta originalmente para REST.

### Backend proposto: `POST /api/providers`

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

## Endpoint REST completo (proposta histórica, não implementada)

> Os exemplos de request/response de `POST`, `PUT` e `DELETE` abaixo são
> exclusivamente o desenho REST original. Não documentam endpoints disponíveis.
> O contrato vigente é Wails + `providers.Service`, conforme a seção
> **Implementação entregue**.

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

### `DELETE /api/providers/{id}` (proposta REST histórica)

**Estado equivalente no contrato Wails atual:**

- [x] `internal/providers/service.go:Service.Delete` remove o provider do
  registry; a API Wails persiste a remoção pelo hook `PersistDelete` de
  `internal/wailsapi/llm_providers.go`.
- [ ] Remove a credencial associada. `CredentialManager` expõe
  `DeletePattern`, mas `Service.Delete` não consulta nem apaga
  `CredentialPattern`; portanto a credencial pode permanecer órfã.

O cleanup automático fazia parte do escopo aceito desta AEP e não deve ser
descrito como entregue enquanto o serviço e seus testes não comprovarem a
remoção segura, inclusive quando um mesmo pattern for compartilhado.

---

## Testes propostos originalmente (histórico)

Os snippets desta seção pertencem ao desenho REST não implementado e não são
evidência executável do runtime atual. As regressões reais estão listadas em
**Implementação entregue**.

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

## Implementação entregue

O desenho original foi absorvido pelo serviço de providers, sem criar o endpoint
HTTP paralelo sugerido no plano:

- [x] `internal/providers/service.go` recebe `APIKey` na criação/atualização,
      deriva o pattern do `BaseURL` e registra a credencial no manager.
- [x] `CreateLLMProviderRequest` transporta `api_key` sem persistir a chave no
      registro público do provider.
- [x] `frontend/src/components/settings/ProviderForm.tsx` oferece o campo de
      credencial no fluxo de criação/edição.
- [x] `internal/app/app_provider_crud_test.go` cobre criação com API key,
      atualização e ausência de credencial.
- [x] `internal/app/app_phase8_integration_test.go` cobre migração, resolução de
      pattern e injeção automática da credencial.
- [x] `internal/app/app_provider_crud_test.go` comprova a remoção do provider.
- [ ] O DELETE remove com segurança a credencial que deixou de ser usada; o
      teste atual não verifica cleanup do cofre.

O endpoint REST `POST /api/providers` proposto originalmente não foi necessário:
desktop e frontend usam o binding/controlador compartilhado de providers.
