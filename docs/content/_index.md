---
title: "Assistente IA"
archetype: "home"
description: "Um ambiente de trabalho com IA, feito para pessoas — independente de como cada uma usa o computador"
---

## Uma ferramenta feita para pessoas

O Assistente é um ambiente de trabalho desktop open source com inteligência artificial. Conversa, edição de texto, listas de tarefas e terminal — tudo em um só lugar, integrado com [mais de 15 provedores de IA compatíveis com a API da OpenAI](configuracao/PROVIDER_CONFIGURATION/). Funciona no Windows, macOS e Linux.

A premissa é simples: ferramentas de IA devem funcionar para pessoas, independente da forma como cada uma interage com o computador — seja por teclado, mouse, voz ou leitor de tela.

O projeto nasceu de uma frustração real: pedir melhorias de acessibilidade a empresas que nunca as implementam, ou que entregam mudanças tímidas depois de anos. Com o boom da IA, essas mesmas empresas começaram a abrir seus sistemas e APIs para integração com ferramentas inteligentes. Vimos uma oportunidade: aproveitar essas novas integrações para diminuir a distância entre pessoas e os sistemas, serviços e aplicações que não investem o suficiente em acessibilidade.

O Assistente nem tem um nome de verdade — era um experimento que se provou útil e continua sendo desenvolvido. Esse trabalho não está pronto. Novas formas de interação estão sendo incluídas e melhoradas continuamente, e o projeto é usado diariamente como ferramenta de trabalho enquanto evolui.

{{% notice style="primary" title="Baixe agora" icon="download" %}}
Disponível para **Windows**, **macOS** e **Linux** — [ir para downloads](downloads/).
{{% /notice %}}

---

## O que o Assistente oferece

### Pensado para diferentes formas de interação

A interface é projetada para funcionar com diferentes formas de uso — e novas formas continuam sendo adicionadas:

- **Navegação por teclado completa** — todas as funcionalidades operam sem mouse
- **Modo Acessibilidade** — desativa streaming e anuncia respostas inteiras de uma vez
- **Navegação intuitiva de mensagens** — `↑` no input vai para o histórico, `Enter` isola uma mensagem para leitura, `ESC` volta
- **Feedback contextual** — ações comunicadas via live regions para leitores de tela
- **5 temas** incluindo Alto Contraste (WCAG AAA)

Testado com NVDA, JAWS e Narrator.

### Voz bidirecional

Interação por voz completa — falar com a IA e ouvir as respostas.

- **Text-to-Speech**: 3 engines — OpenAI (alta qualidade), SAPI5 (integrado com leitores de tela), Web Speech (gratuito) — [configurar voz](configuracao/SPEECH_CONFIGURATION/)
- **Speech-to-Text**: Whisper API ou Web Speech, com modos push-to-talk, toggle e detecção de silêncio
- **Wake Word**: Ativação por voz mesmo com a janela minimizada

### Mais do que um chat

Um espaço de trabalho onde painéis de chat, editores, tarefas e terminais funcionam juntos — com ou sem IA:

- **[Workspaces](recursos/WORKSPACES/)** — agrupe conversas, arquivos, tarefas e terminais por projeto; cada workspace mantém seu próprio contexto
- **[Editor](recursos/EDITOR/)** — múltiplas abas, chat inline com IA, syntax highlighting (Monaco), Markdown e Mermaid
- **[Terminal](recursos/TERMINAL/)** — sessões PTY reais com histórico; a IA executa comandos quando necessário
- **[Listas de Tarefas](recursos/TASK_LISTS/)** — workflows customizáveis, subtarefas, visualização em lista ou kanban
- **[MCP (Model Context Protocol)](configuracao/MCP_CONFIG_EXAMPLES/)** — ferramentas externas conectadas ao assistente

### Qualquer modelo, local ou cloud

Compatível com [mais de 15 provedores de IA com API compatível OpenAI](configuracao/PROVIDER_CONFIGURATION/), sem dependência de nenhum:

| Para começar | Recomendação |
|---|---|
| **Gratuito e rápido** | [Groq](https://console.groq.com) — plano free generoso |
| **Sem internet** | [Ollama](https://ollama.ai) — roda 100% local |
| **Melhor qualidade** | OpenAI (GPT-4o) ou Claude (Sonnet) |
| **Maior variedade** | [OpenRouter](https://openrouter.ai) — 100+ modelos com uma API key |

---

## Origem do projeto

O Assistente começou como um experimento: seria possível usar IA para remover barreiras de acesso a serviços, sistemas e aplicativos que não investem em acessibilidade?

Ironicamente, o próprio desenvolvimento depende de ferramentas de IA que têm pouca ou nenhuma acessibilidade. Todo o código é produzido com auxílio de Cursor, GitHub Copilot, Claude Code e Antigravity — ferramentas que, elas mesmas, ainda precisam melhorar o acesso para pessoas com deficiência.

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
