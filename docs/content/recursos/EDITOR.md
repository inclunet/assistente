---
title: "Editor"
weight: 2
---

# Editor Integrado

O Assistente possui um editor de texto integrado com suporte a múltiplas abas, modos variados e chat inline com IA.

## Modos de Edição

| Modo | Descrição |
|---|---|
| **Código** | Editor Monaco (mesmo do VS Code) com syntax highlighting |
| **Rico** | Editor de texto rico com formatação |
| **Visualização** | Documento renderizado para leitura e navegação |

## Funcionalidades

### Múltiplas Abas

Cada aba representa um arquivo ou draft. O estado das abas é preservado por workspace:

- Abrir arquivo: **Menu File → Abrir** ou via deep link
- Novo arquivo: **Menu File → Novo**
- Salvar: **Ctrl + S** (salva no disco) ou auto-save
- Salvar como: **Menu File → Salvar Como**
- O modo Código, Rico ou Visualização pertence à aba e é restaurado
  mesmo depois de fechar e abrir novamente o Assistente. Fechar a aba remove
  essa escolha.

### Auto-Save

Quando ativado, o editor salva drafts automaticamente conforme você edita. Drafts são preservados mesmo se fechar o app e reabrir.

### Chat Inline

O chat inline permite pedir para a IA editar, gerar ou transformar conteúdo diretamente no editor:

1. Selecione o texto que quer alterar (ou posicione o cursor)
2. Abra o chat inline
3. Descreva o que quer (ex: "traduza para inglês", "refatore esta função")
4. A IA gera sugestões que podem ser aplicadas como patch

### Monitoramento de Arquivos

O editor monitora mudanças em arquivos abertos no disco (via `fsnotify`). Se um arquivo for modificado externamente:

- O editor notifica sobre o conflito
- Oferece opção de recarregar ou manter a versão local
- Suporta merge em caso de conflitos

## Formatos Suportados

- Texto puro
- Markdown (.md)
- Código-fonte (todas as linguagens suportadas pelo Monaco)
- Diagramas Mermaid

## Recursos avançados

### Apresentação Reveal

Arquivos Markdown com `---` como separador de slides podem ser exibidos como apresentação Reveal.js. Cada seção vira um slide navegável por `PageUp`/`PageDown` e anunciado para leitores de tela.

### Mermaid acessível

Diagramas Mermaid são renderizados com descrição textual alternativa e navegação por teclado. Erros de sintaxe são mostrados fora do bloco para não poluir o conteúdo, com cartaz de erro acessível.

### Leitura de documentos

O editor pode abrir documentos do workspace como visualização somente leitura. **Alt+3** ou a ação **Visualização** do menu entram no modo renderizado e levam o foco diretamente ao documento interno, sem exigir Enter adicional. Também é possível pressionar **Enter** na âncora da área de conteúdo ao alcançá-la pela navegação comum. O leitor de telas passa a navegar títulos, links e demais elementos do conteúdo sem alcançar as barras de ferramentas pelas setas; **Tab** e **Shift+Tab** continuam livres para alcançar controles e sair da área, e **F6** continua alternando entre as regiões do workspace.

A leitura continua ativa quando Tab ou F6 levam o foco para outra região. Pressionar **Alt+3** novamente retorna diretamente ao documento. Enquanto o preview atual permanecer visível e ativo, **Esc** também devolve o foco ao documento interno a partir de uma barra de ferramentas ou outro controle fora do conteúdo. Quando o foco já está no documento, **Esc** não executa nenhuma ação. Modais e menus abertos conservam seu próprio comportamento.

Ao abrir um documento no modo de código, o foco entra no Monaco assim que o editor termina de carregar. Um diálogo aberto ou outro campo de texto que já esteja ativo mantém o foco.

Ao mudar de aba com **Ctrl+Tab**, **Ctrl+Shift+Tab**, **Ctrl+PageUp** ou
**Ctrl+PageDown**, o foco entra diretamente na superfície correspondente ao
modo salvo da aba: Monaco, editor rico ou documento renderizado.

## Atalhos

| Atalho | Ação |
|---|---|
| `Ctrl + S` | Salvar |
| `Ctrl + W` | Fechar aba |
| `Ctrl + Tab` | Próxima aba |
| `Ctrl + Shift + Tab` | Aba anterior |
| `Ctrl + PageDown` | Próxima aba |
| `Ctrl + PageUp` | Aba anterior |
| `Alt + 1` | Modo Código |
| `Alt + 2` | Modo de texto rico |
| `Alt + 3` | Visualização e leitura do documento |
| Deep link | `assistente://editor/{id}` |
