# Configuração de Provedores LLM

## Visão Geral

O sistema de provedores foi projetado para ser extensível e fácil de manter. Cada provedor é definido através de uma configuração estruturada que controla seu comportamento no formulário e durante a validação.

## Estrutura de Configuração

Os provedores são configurados em [ProviderForm.tsx](../frontend/src/components/settings/ProviderForm.tsx) através do objeto `PROVIDER_CONFIG`:

```typescript
interface ProviderConfig {
  label: string;                    // Nome exibido no dropdown
  defaultUrl: string;               // URL padrão do provedor
  urlEditable: boolean;             // Se a URL pode ser alterada pelo usuário
  apiKeyRequired: boolean;          // Se a API Key é obrigatória
  testRequiresApiKey: boolean;      // Se a API Key é necessária para testar
  helpText?: string;                // Texto de ajuda exibido no formulário
}
```

## Tipos de Provedores

### 1. Provedores Comerciais (API)

Exemplo: **OpenAI**, **Anthropic**, **Google**, **OpenRouter**, **xAI (Grok)**

Características:
- URL fixa (não editável)
- API Key obrigatória
- Teste requer API Key
- Exemplo de configuração:

```typescript
openai: {
  label: 'OpenAI',
  defaultUrl: 'https://api.openai.com/v1',
  urlEditable: false,
  apiKeyRequired: true,
  testRequiresApiKey: true,
  helpText: 'Get your API key from https://platform.openai.com/api-keys',
}
```

### 2. Provedores Locais (Auto-hospedados)

Exemplo: **Ollama**, **LocalAI**

Características:
- URL editável (pode ser localhost, IP remoto, etc.)
- API Key opcional
- Teste funciona sem API Key
- Exemplo de configuração:

```typescript
ollama: {
  label: 'Ollama (local)',
  defaultUrl: 'http://localhost:11434',
  urlEditable: true,
  apiKeyRequired: false,
  testRequiresApiKey: false,
  helpText: 'Running locally. You can change the URL if using a different host/port',
}
```

### 3. Proxies e Gateways

Exemplo: **LiteLLM**

Características:
- URL editável
- API Key obrigatória
- Teste requer API Key
- Exemplo de configuração:

```typescript
litellm: {
  label: 'LiteLLM Proxy',
  defaultUrl: 'http://localhost:4000',
  urlEditable: true,
  apiKeyRequired: true,
  testRequiresApiKey: true,
  helpText: 'LiteLLM proxy server. Requires URL and API key',
}
```

### 4. Custom/Genérico

Para provedores genéricos ou desconhecidos:

```typescript
custom: {
  label: 'Custom',
  defaultUrl: '',
  urlEditable: true,
  apiKeyRequired: true,
  testRequiresApiKey: true,
  helpText: 'Configure your custom LLM provider',
}
```

## Como Adicionar um Novo Provedor

### 1. Adicionar Configuração no Frontend

No arquivo [ProviderForm.tsx](../frontend/src/components/settings/ProviderForm.tsx), adicione uma entrada no objeto `PROVIDER_CONFIG`:

```typescript
const PROVIDER_CONFIG: Record<string, ProviderConfig> = {
  // ... provedores existentes ...
  
  // Novo provedor
  seuProvedor: {
    label: 'Seu Provedor',
    defaultUrl: 'https://api.seuprovedor.com',
    urlEditable: false,
    apiKeyRequired: true,
    testRequiresApiKey: true,
    helpText: 'Instruções para obter a API key',
  },
};
```

### 2. Garantir Suporte no Backend

O backend em [app.go](../app.go) já suporta provedores genéricos através da função `TestLLMProvider()`. Nenhuma mudança é necessária no backend para adicionar novos provedores.

### 3. Testes (Opcional)

Se desejar adicionar testes específicos para o novo provedor, atualize [app_provider_crud_test.go](../app_provider_crud_test.go):

```go
// TestCreateProviderSeuProvedor testa criação de provedor customizado
func TestCreateProviderSeuProvedor(t *testing.T) {
  // ... teste específico ...
}
```

## Fluxo de Validação

### Para Provedores com API Key Obrigatória

1. ✓ Usuário preenche Nome
2. ✓ Usuário seleciona Tipo
3. ✓ URL é preenchida automaticamente (se fixe)
4. ✓ Usuário deve preencher API Key
5. ✓ Sistema testa automaticamente ao sair do campo de API Key
6. ✓ Botão "Salvar" só ativa após teste bem-sucedido

### Para Provedores com API Key Opcional

1. ✓ Usuário preenche Nome
2. ✓ Usuário seleciona Tipo
3. ✓ URL pode ser alterada pelo usuário
4. ✓ API Key é opcional (campo rotulado como tal)
5. ✓ Existe botão "Testar Conexão" explícito
6. ✓ Teste funciona com ou sem API Key
7. ✓ Botão "Salvar" só ativa após teste bem-sucedido

## Variáveis de Controle

O componente `ProviderForm` usa estas variáveis para controlar comportamento:

```typescript
const config = PROVIDER_CONFIG[formData.type];
const isUrlReadonly = !config.urlEditable && !provider;      // URL é apenas leitura?
const requiresApiKey = config.apiKeyRequired;                 // API Key é obrigatória?
const testRequiresApiKey = config.testRequiresApiKey;        // Teste precisa de Key?
```

## Tipos Suportados Atualmente

| Tipo | Categoria | URL Fixa | Key Obrigatória | Suporta Teste Sem Key |
|------|-----------|----------|-----------------|----------------------|
| `openai` | Commercial | ✓ | ✓ | ✗ |
| `anthropic` | Commercial | ✓ | ✓ | ✗ |
| `google` | Commercial | ✓ | ✓ | ✗ |
| `openrouter` | Commercial | ✓ | ✓ | ✗ |
| `xai` | Commercial | ✓ | ✓ | ✗ |
| `cohere` | Commercial | ✓ | ✓ | ✗ |
| `ollama` | Local | ✗ | ✗ | ✓ |
| `localai` | Local | ✗ | ✗ | ✓ |
| `litellm` | Proxy | ✗ | ✓ | ✗ |
| `custom` | Generic | ✗ | ✓ | ✗ |

## Notas Importantes

1. **Teste de Conectividade**: O `TestLLMProvider()` no backend faz um GET simples para a URL do provedor com headers de autenticação se uma API Key for fornecida.

2. **Segurança de API Keys**: As chaves de API são armazenadas criptografadas no gerenciador de credenciais do sistema operacional.

3. **Mensagens de Ajuda**: Use o campo `helpText` para guiar usuários sobre como obter credenciais ou configurar o provedor.

4. **URLs Padrão**: Sempre forneça uma URL padrão sensata, especialmente para provedores locais (ex: `http://localhost:PORT`).

## Exemplos de Adição Rápida

### Adicionar Claude (Anthropic alternativo)
```typescript
claude: {
  label: 'Claude (Anthropic)',
  defaultUrl: 'https://api.anthropic.com',
  urlEditable: false,
  apiKeyRequired: true,
  testRequiresApiKey: true,
  helpText: 'Same as Anthropic provider - get key from https://console.anthropic.com',
}
```

### Adicionar Ollama em Docker
```typescript
ollamaDocker: {
  label: 'Ollama (Docker)',
  defaultUrl: 'http://ollama:11434',  // Assume docker network
  urlEditable: true,
  apiKeyRequired: false,
  testRequiresApiKey: false,
  helpText: 'Ollama running in Docker. Uses docker network hostname by default.',
}
```

### Adicionar Provedor Self-Hosted Genérico
```typescript
selfHosted: {
  label: 'Self-Hosted LLM',
  defaultUrl: 'http://localhost:8000',
  urlEditable: true,
  apiKeyRequired: true,
  testRequiresApiKey: true,
  helpText: 'Generic self-hosted LLM server. Configure URL and authentication token.',
}
```
