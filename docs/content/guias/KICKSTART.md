---
title: "Kickstart"
weight: 1
description: "Primeiros passos: o que o Assistente é (e não é), como navegar e os atalhos essenciais"
---

## O que o Assistente é

O Assistente é um **ambiente de trabalho desktop** (Windows, macOS, Linux) que reúne chat, editor, terminal e listas de tarefas. Ele **não tem um LLM próprio**: para conversar com IA você conecta um provedor de terceiros — OpenAI, Anthropic, Google, Groq, Ollama local, OpenRouter e outros 10+ compatíveis com a API OpenAI. Sem provedor configurado, o app funciona como workspace offline (sem IA).

Configure em **Configurações → Provedores**: informe a URL e a chave do provedor. Veja [Provedores LLM](../configuracao/PROVIDER_CONFIGURATION/).

## Navegação em listas de mensagens

A lista de mensagens é o centro do chat. Padrões que se repetem em todo o app:

- `↑` com o input vazio — vai para a última mensagem e permite navegar pelo histórico com `↑`/`↓`.
- `↑`/`↓` dentro da lista — move o foco entre mensagens.
- `Enter` em uma mensagem — isola a mensagem com foco total (modo leitura), útil para leitores de tela; `ESC` volta.
- `Tab` / `Shift+Tab` — sai da lista para o próximo elemento interativo; o foco sempre volta ao fechar um diálogo.

A mesma lógica vale para o histórico de conversas e outras listas verticais.

## Navegação em grids

Tabelas e grades (tarefas, contatos, canais) usam o componente `DataGrid` com `role="grid"`:

- `↑`/`↓`/`←`/`→` — navega entre células.
- `Enter` — ativa a linha/célula.
- `Home`/`End` — início/fim da linha; `PageUp`/`PageDown` quando disponível.
- O grid anuncia mudanças via live regions para NVDA/JAWS.

## Navegação em toolbars

Barras de ferramentas (editor, chat, jobs) usam `role="toolbar"`:

- `Tab` — entra na toolbar.
- `←`/`→` ou `↑`/`↓` — navega entre botões dentro da toolbar (padrão ARIA toolbar).
- `Enter`/`Espaço` — aciona o botão focado.
- `ESC` — sai da toolbar e devolve o foco ao conteúdo (editor, lista ou input).

Botões só com ícone têm `aria-label`; ícones decorativos têm `aria-hidden="true"`.

## Principais atalhos

| Atalho | Ação |
|---|---|
| `F1` | Ajuda |
| `Alt + 1` / `Alt + 2` | Chat / Configurações |
| `Ctrl + N` | Nova conversa |
| `Ctrl + K` | Busca rápida |
| `Ctrl + Shift + P` | Paleta de comandos |
| `ESC` | Fechar modal/menu ou voltar ao contexto anterior |
| `Ctrl + Shift + R` | Repetir a pergunta em diálogos de decisão bloqueantes |

Lista completa em [Hotkeys](../../recursos/HOTKEYS/).

## Próximos passos

1. Configure um provedor em **Configurações → Provedores**.
2. Crie um workspace para agrupar conversas e arquivos por projeto.
3. Explore o editor com chat inline, o terminal e as listas de tarefas — tudo funciona com ou sem IA.
