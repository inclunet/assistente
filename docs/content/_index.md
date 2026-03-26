---
title: "Assistente IA"
archetype: "home"
description: "Cliente de chat multi-modelo e multi-conversa completamente acessível"
---

**Cliente de chat multi-modelo e multi-conversa completamente acessível, feito por quem usa para quem usa.**

{{% notice style="primary" title="Download" icon="download" %}}
Baixe a última versão para **Windows**, **macOS** ou **Linux** na [página de downloads](downloads/).
{{% /notice %}}

## Por que o Assistente?

Este projeto começou como uma **prova de conceito**: será que é possível criar uma aplicação de IA verdadeiramente acessível para pessoas cegas? Não apenas "usável", mas **otimizada** para quem depende de leitores de tela?

A resposta é sim. E o projeto evoluiu muito além disso.

Concebido inicialmente para testar a viabilidade de um chat acessível compatível com qualquer provedor de IA, o Assistente rapidamente se tornou uma **ferramenta de trabalho indispensável**. O projeto cresceu organicamente, incorporando novas funcionalidades conforme surgiam necessidades reais no dia-a-dia.

**Este não é um projeto feito "para" pessoas cegas. É feito "por" uma pessoa cega, para ela mesma.** Cada recurso foi pensado a partir da pergunta: "Como isso me ajudaria hoje?"

## Um Experimento Pessoal

Este projeto também é um experimento sobre **até onde uma pessoa cega consegue ir construindo software usando IA como assistente**. Todo o desenvolvimento foi feito usando ferramentas como Cursor, GitHub Copilot, Antigravity e Claude Code.

A meta era **escrever o mínimo de código manualmente possível**, delegando a construção para IAs enquanto focava em arquitetura, design de acessibilidade e experiência do usuário.

## Recursos

| Recurso | Descrição |
|---|---|
| 🗂️ **Multi-Conversa em Abas** | Várias conversas simultâneas em abas paralelas, alternando contextos sem perder o fio |
| ⌨️ **Navegação Inovadora** | ↑ no input vai pra última mensagem, ↑/↓ navega, Enter isola, ESC retorna |
| 🔊 **TTS Multi-Engine** | OpenAI voices, Web Speech API, SAPI5 — [configurar voz](configuracao/SPEECH_CONFIGURATION/) |
| 🎤 **STT com Wake Word** | Reconhecimento de voz fora da janela do app |
| ♿ **Acessibilidade Real** | Otimizado para NVDA, JAWS, Narrator — design centrado em quem usa |
| 🔌 **15+ Provedores** | OpenAI, Claude, DeepSeek, Groq, Mistral, Ollama e mais — [ver todos](configuracao/PROVIDER_CONFIGURATION/) |
| 🛠️ **MCP Integrado** | Model Context Protocol para ferramentas externas — [exemplos](configuracao/MCP_CONFIG_EXAMPLES/) |
| 📝 **Editor Integrado** | Editor com múltiplas abas, chat inline com IA e suporte a Mermaid — [saiba mais](recursos/EDITOR/) |
| 💻 **Terminal** | Terminal integrado com sessões PTY persistentes — [saiba mais](recursos/TERMINAL/) |
| 📋 **Listas de Tarefas** | Workflows customizáveis, subtarefas e kanban — [saiba mais](recursos/TASK_LISTS/) |
| 📦 **Workspaces** | Organize projetos em espaços de trabalho separados — [saiba mais](recursos/WORKSPACES/) |
| 🔗 **Deep Links** | Navegação programática via `assistente://` — [saiba mais](recursos/DEEP_LINKS/) |
| 🔄 **Auto-Update** | Atualizações automáticas silenciosas |
| 🌐 **Multiplataforma** | Windows, macOS e Linux com interface nativa |

## Acessibilidade em Primeiro Lugar

### Navegação e Estrutura

- Skip link para pular direto ao conteúdo principal
- Landmarks ARIA para navegação rápida entre seções
- Navegação completa por teclado — nenhuma funcionalidade requer mouse
- Focus visível e ordem lógica de tabulação

### Leitores de Tela

- Live regions para comunicar mudanças dinâmicas
- **Modo Acessibilidade** — desativa streaming, anuncia respostas completas
- Labels descritivos em todos os controles e botões
- Roles e estados ARIA corretamente implementados
- Anúncios contextuais para feedback de ações

### Visual e Design

- Alto contraste com suporte a preferências do sistema
- Áreas de toque mínimas de 44x44px (WCAG AAA)
- Textos redimensionáveis sem perda de funcionalidade

**Testado com:** NVDA, JAWS e Narrator (Windows)

## Atalhos Essenciais

| Atalho | Ação |
|---|---|
| `F1` | Abrir menu de ajuda |
| `Alt + 1` | Ir para Chat |
| `Alt + 2` | Ir para Configurações |
| `Ctrl + N` | Nova conversa |
| `Ctrl + K` | Busca rápida |
| `↑` (no input) | Ir para última mensagem |
| `↑/↓` (navegando) | Navegar entre mensagens |
| `Enter` (em mensagem) | Isolar mensagem com foco total |
| `ESC` | Retornar ao contexto anterior |
| `Shift + Enter` | Nova linha |

## Inspirações

A interface foi inspirada em ferramentas que funcionam bem com leitores de tela:

- **IDEs modernos** — Visual Studio Code, Cursor: navegação por atalhos, busca rápida
- **Ferramentas de comunicação** — Slack: threads e canais, navegação entre mensagens
- **Terminais** — Bash, PowerShell, CMD: comandos eficientes, navegação por histórico
- **Aplicações produtivas** — Windows Explorer, Excel, Google Sheets: atalhos estruturados

## Documentação

- [Downloads](downloads/) — Baixe a última versão para Windows, macOS ou Linux
- [Guias](guias/) — Build, release, versionamento e code signing
- [Configuração](configuracao/) — Provedores LLM, voz, MCP, Slack e skills
- [Recursos](recursos/) — Workspaces, editor, terminal, tarefas, deep links e hotkeys

## Stack Técnica

- **Backend**: Go (Golang) — alto desempenho e compilação nativa
- **Frontend**: React + TypeScript — interface moderna e acessível
- **Desktop**: Wails v2 — combina Go + WebView nativo do sistema
- **Auto-update**: Sistema próprio via GitHub Releases API

## Links

- [GitHub](https://github.com/inclunet/assistente)
- [Releases](https://github.com/inclunet/assistente/releases)
- [Reportar Bug](https://github.com/inclunet/assistente/issues)
