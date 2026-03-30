---
title: "Downloads"
weight: 1
---

# Downloads

Baixe a última versão do Assistente para o seu sistema operacional. Os links abaixo são atualizados automaticamente a cada nova release.

{{< github-downloads >}}

## Provedores de IA Suportados

Basta configurar a **URL Base da API** para usar diferentes serviços:

| Provedor | URL Base | Como Obter API Key |
|---|---|---|
| **OpenAI** (padrão) | `https://api.openai.com/v1` | [platform.openai.com](https://platform.openai.com/api-keys) |
| **Groq** | `https://api.groq.com/openai/v1` | [console.groq.com](https://console.groq.com) — Gratuito e rápido |
| **Google AI** | `https://generativelanguage.googleapis.com/v1` | [makersuite.google.com](https://makersuite.google.com/app/apikey) |
| **OpenRouter** | `https://openrouter.ai/api/v1` | [openrouter.ai](https://openrouter.ai/keys) — 100+ modelos |
| **Ollama** (Local) | `http://localhost:11434/v1` | Sem API key — [ollama.ai](https://ollama.ai) |
| **LM Studio** (Local) | `http://localhost:1234/v1` | Sem API key — [lmstudio.ai](https://lmstudio.ai) |

{{% notice style="tip" title="Dica para iniciantes" %}}
Comece com **Groq** (gratuito e rápido) ou **Ollama** (roda no seu PC, sem internet).
{{% /notice %}}

## Primeira Execução

Na primeira inicialização, você será guiado para configurar:

1. **Chave de API** — Sua API key do provedor escolhido
2. **URL Base** — Endpoint da API (padrão: `https://api.openai.com/v1`)

As configurações são salvas em:
- **Windows**: `%USERPROFILE%\.assistente\config.json`
- **macOS/Linux**: `~/.assistente/config.json`

## Modo Acessibilidade

{{% notice style="warning" title="Recomendado para usuários de leitores de tela" %}}
1. Acesse Configurações (`Alt + 2`)
2. Ative "Modo Acessibilidade"
3. Quando ativado, o streaming é desabilitado e respostas completas são anunciadas de uma vez
{{% /notice %}}

## Para Desenvolvedores

Instale o [Wails CLI](https://wails.io/docs/gettingstarted/installation):

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Clone e compile:

```bash
git clone https://github.com/inclunet/assistente.git
cd assistente
cd frontend && npm install && cd ..
wails dev
```

Requisitos: [Go 1.23+](https://golang.org/dl/), [Node.js 20+](https://nodejs.org/), Wails CLI.
