---
title: "Assistente IA"
archetype: "home"
description: "Cliente de chat multi-modelo e multi-conversa completamente acessível — feito por uma pessoa cega, para ela mesma"
---

## Um chat de IA feito por quem não consegue usar os outros

A maioria dos clientes de chat com IA ignora quem depende de leitores de tela. Eles não são "inacessíveis" — simplesmente nunca foram pensados por alguém que precisa deles.

**O Assistente é diferente.** Foi criado por uma pessoa cega que queria uma ferramenta de trabalho real, não uma adaptação. Cada recurso nasceu da pergunta: *"Como isso me ajudaria hoje?"*

O resultado é um cliente desktop que funciona com **qualquer provedor de IA** (OpenAI, Claude, Groq, Ollama, e [mais 15](configuracao/PROVIDER_CONFIGURATION/)), otimizado para **NVDA, JAWS e Narrator**, mas igualmente poderoso para qualquer usuário.

{{% notice style="primary" title="Baixe agora" icon="download" %}}
Disponível para **Windows**, **macOS** e **Linux** — [ir para downloads](downloads/).
{{% /notice %}}

---

## O que torna o Assistente único

### Acessibilidade que não é checkbox

Não é uma camada de ARIA colocada depois. A interface foi desenhada _a partir_ da experiência com leitor de tela:

- **Navegação por teclado completa** — tudo funciona sem mouse, sem exceção
- **Modo Acessibilidade** — desativa streaming e anuncia respostas inteiras de uma vez
- **Navegação de mensagens como IDE** — `↑` no input vai para o histórico, `Enter` isola uma mensagem para leitura, `ESC` volta. Sem dead-ends
- **Feedback por voz e anúncios** — ações são comunicadas via live regions para leitores de tela
- **5 temas** incluindo Alto Contraste (WCAG AAA)

Testado com NVDA, JAWS e Narrator.

### Voz bidirecional

Fale com a IA e ouça as respostas — sem tocar no teclado se preferir.

- **Text-to-Speech**: 3 engines — OpenAI (alta qualidade), SAPI5 (integrado com leitores de tela), Web Speech (gratuito) — [configurar](configuracao/SPEECH_CONFIGURATION/)
- **Speech-to-Text**: Whisper API ou Web Speech, com modos push-to-talk, toggle e detecção de silêncio
- **Wake Word**: Ative o assistente por voz mesmo com a janela minimizada

### Mais do que um chat

O Assistente evoluiu para um ambiente de trabalho completo:

- **[Editor integrado](recursos/EDITOR/)** — múltiplas abas, chat inline com IA, syntax highlighting (Monaco), Markdown e Mermaid
- **[Terminal](recursos/TERMINAL/)** — sessões PTY reais com histórico, a IA executa comandos quando precisa
- **[Listas de Tarefas](recursos/TASK_LISTS/)** — workflows customizáveis, subtarefas, visualização em lista ou kanban
- **[Workspaces](recursos/WORKSPACES/)** — agrupe conversas, editor, terminal e tarefas por projeto
- **[MCP (Model Context Protocol)](configuracao/MCP_CONFIG_EXAMPLES/)** — conecte ferramentas externas ao assistente

### Qualquer modelo, local ou cloud

Use [15+ provedores](configuracao/PROVIDER_CONFIGURATION/) sem ficar preso a nenhum:

| Para começar | Recomendação |
|---|---|
| **Gratuito e rápido** | [Groq](https://console.groq.com) — plano free generoso |
| **Sem internet** | [Ollama](https://ollama.ai) — roda 100% no seu PC |
| **Melhor qualidade** | OpenAI (GPT-4o) ou Claude (Sonnet) |
| **Maior variedade** | [OpenRouter](https://openrouter.ai) — 100+ modelos com uma API key |

---

## Como aconteceu

Este projeto é também um experimento: **até onde uma pessoa cega consegue ir construindo software usando IA como assistente?**

Todo o desenvolvimento foi feito com Cursor, GitHub Copilot, Claude Code e Antigravity. A meta era escrever o mínimo de código manualmente, delegando a construção para IAs enquanto o foco ficava em **arquitetura, acessibilidade e experiência do usuário**.

O que começou como prova de conceito se tornou ferramenta de trabalho diária. E continua crescendo.

---

## Atalhos Essenciais

| Atalho | Ação |
|---|---|
| `F1` | Menu de ajuda |
| `Alt + 1` / `Alt + 2` | Chat / Configurações |
| `Ctrl + N` | Nova conversa |
| `Ctrl + K` | Busca rápida |
| `↑` (no input) | Ir para última mensagem |
| `↑/↓` | Navegar entre mensagens |
| `Enter` (em mensagem) | Isolar com foco total |
| `ESC` | Voltar ao contexto anterior |

---

## Documentação

| Seção | Conteúdo |
|---|---|
| **[Downloads](downloads/)** | Windows, macOS e Linux |
| **[Configuração](configuracao/)** | [Provedores LLM](configuracao/PROVIDER_CONFIGURATION/), [voz](configuracao/SPEECH_CONFIGURATION/), [MCP](configuracao/MCP_CONFIG_EXAMPLES/), [Slack](configuracao/SLACK_CHANNEL_SETUP/) e [skills](configuracao/SKILL_TEMPLATE_CONTEXT/) |
| **[Recursos](recursos/)** | [Workspaces](recursos/WORKSPACES/), [editor](recursos/EDITOR/), [terminal](recursos/TERMINAL/), [tarefas](recursos/TASK_LISTS/), [deep links](recursos/DEEP_LINKS/) e [hotkeys](recursos/HOTKEYS/) |
| **[Guias](guias/)** | [Build](guias/BUILD_WITH_VERSION/), [release](guias/RELEASE_QUICKSTART/), [versionamento](guias/VERSIONING/) e [code signing](guias/CODE_SIGNING_SETUP/) |

## Stack Técnica

| Camada | Tecnologia |
|---|---|
| Backend | Go — alto desempenho e compilação nativa |
| Frontend | React + TypeScript |
| Desktop | Wails v2 — Go + WebView nativo |
| Auto-update | Sistema próprio via GitHub Releases API |

## Links

- [GitHub](https://github.com/inclunet/assistente)
- [Releases](https://github.com/inclunet/assistente/releases)
- [Reportar Bug](https://github.com/inclunet/assistente/issues)
