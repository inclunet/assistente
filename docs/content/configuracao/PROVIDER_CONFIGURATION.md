---
title: "Provedores LLM"
weight: 1
---

# Configuração de Provedores LLM

O Assistente suporta múltiplos provedores de LLM, tanto comerciais (cloud) quanto locais. Basta adicionar um provedor nas configurações (`Alt + 2`) e informar a chave de API quando necessário.

## Provedores Suportados

### Provedores Cloud (API Key obrigatória)

| Provedor | URL Base | Modelo Padrão | Como obter API Key |
|---|---|---|---|
| **OpenAI** | `https://api.openai.com/v1` | `gpt-4o-mini` | [platform.openai.com/api-keys](https://platform.openai.com/api-keys) |
| **Claude (Anthropic)** | `https://api.anthropic.com/v1` | `claude-3-7-sonnet-20250219` | [console.anthropic.com](https://console.anthropic.com) |
| **DeepSeek** | `https://api.deepseek.com/v1` | `deepseek-chat` | [platform.deepseek.com](https://platform.deepseek.com) |
| **xAI (Grok)** | `https://api.x.ai/v1` | `grok-2` | [console.x.ai](https://console.x.ai) |
| **Mistral AI** | `https://api.mistral.ai/v1` | `mistral-large-latest` | [console.mistral.ai](https://console.mistral.ai) |
| **Groq** | `https://api.groq.com/openai/v1` | `llama-3.3-70b-versatile` | [console.groq.com](https://console.groq.com) — Gratuito |
| **Together AI** | `https://api.together.xyz/v1` | `meta-llama/Llama-3.3-70B-Instruct-Turbo` | [api.together.ai](https://api.together.ai) |
| **Fireworks AI** | `https://api.fireworks.ai/inference/v1` | `accounts/fireworks/models/llama-v3p3-70b-instruct` | [fireworks.ai](https://fireworks.ai) |
| **Perplexity** | `https://api.perplexity.ai` | `sonar` | [perplexity.ai/settings/api](https://perplexity.ai/settings/api) |
| **Google (Gemini)** | `https://generativelanguage.googleapis.com/v1beta/openai/` | `gemini-2.0-flash` | [makersuite.google.com/app/apikey](https://makersuite.google.com/app/apikey) |
| **OpenRouter** | `https://openrouter.ai/api/v1` | `openai/gpt-4o-mini` | [openrouter.ai/keys](https://openrouter.ai/keys) — 100+ modelos |

### Provedores Locais (sem API Key)

| Provedor | URL Base | Modelo Padrão | Notas |
|---|---|---|---|
| **Ollama** | `http://localhost:11434/api` | `llama2` | [ollama.ai](https://ollama.ai) — 100% gratuito, URL editável, timeout de 300s |
| **LocalAI** | `http://localhost:8080` | — | URL editável, token opcional |

### Proxies e Custom

| Provedor | URL Base | API Key | Notas |
|---|---|---|---|
| **LiteLLM Proxy** | `http://localhost:4000` | Obrigatória | URL editável — proxy para múltiplos provedores |
| **Custom** | (definida pelo usuário) | Obrigatória | Configure manualmente qualquer provedor compatível com OpenAI API |

## Dicas para Iniciantes

- **Gratuito e rápido**: Comece com **Groq** — plano gratuito generoso e respostas muito rápidas
- **Sem internet**: Use **Ollama** — roda 100% no seu computador, sem custo
- **Melhor qualidade**: **OpenAI** (GPT-4o) ou **Claude** (Claude 3.7 Sonnet)
- **Maior variedade**: **OpenRouter** — acesso a 100+ modelos de vários provedores com uma só API key

## Adicionando um Provedor

1. Acesse **Configurações** (`Alt + 2`)
2. Na seção **Provedores**, clique em **Adicionar Provedor**
3. Escolha o **tipo** do provedor
4. O nome e URL são preenchidos automaticamente
5. Informe a **API Key** (se necessário)
6. O sistema testa a conexão automaticamente
7. Clique em **Salvar**

## Credenciais

As chaves de API são armazenadas de forma segura no gerenciador de credenciais do sistema operacional (Keychain no macOS, Credential Manager no Windows, libsecret no Linux).

O sistema também detecta automaticamente credenciais em variáveis de ambiente comuns:
- `OPENAI_API_KEY`
- `ANTHROPIC_API_KEY`
- `GROQ_API_KEY`
- Entre outras

## Configurações Avançadas

Cada provedor possui:
- **Timeout**: 180s (padrão) ou 300s (Ollama, para modelos grandes)
- **Headers customizados**: Para autenticação alternativa ou proxy
- **Credential Pattern**: Domínio usado para resolver credenciais automaticamente (ex: `api.openai.com`)

## Adicionando Provedores ao Código

### Frontend

Adicione a configuração em `frontend/src/components/settings/ProviderForm.tsx`:

```typescript
seuProvedor: {
  label: 'Seu Provedor',
  defaultUrl: 'https://api.seuprovedor.com/v1',
  urlEditable: false,
  apiKeyRequired: true,
  testRequiresApiKey: true,
  helpText: 'Instruções para obter a API key',
}
```

### Backend

Adicione o tipo em `internal/llm/provider.go`:

```go
ProviderSeuProvedor ProviderType = "seuprovedor"
```

E registre a configuração padrão em `app.go` no método de inicialização de provedores.

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
