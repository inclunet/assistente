# DataGrid - Atalhos de Teclado

O DataGrid implementa navegação avançada com teclado, similar ao Windows Explorer.

## Navegação Básica

| Atalho | Ação |
|--------|------|
| `↑` `↓` | Move para linha anterior/próxima |
| `←` `→` | Move para coluna anterior/próxima |
| `Home` | Primeira coluna da linha atual |
| `End` | Última coluna da linha atual |
| `Page Up` | Sobe 10 linhas |
| `Page Down` | Desce 10 linhas |
| `Ctrl+Home` | Primeira célula do grid (linha 0, coluna 0) |
| `Ctrl+End` | Última célula do grid (última linha, última coluna) |

## Seleção (modo `multiSelect`)

| Atalho | Ação |
|--------|------|
| `Ctrl+Space` | Toggle seleção da linha atual (seleção aleatória) |
| `Shift+↑/↓` | Seleciona intervalo de linhas |
| `Shift+Page Up/Down` | Seleciona intervalo de 10 linhas |
| `Shift+Home/End` | Seleciona até primeira/última coluna (mantém linha) |
| `Ctrl+A` | Seleciona todas as linhas |
| `Escape` | Limpa toda a seleção |

## Ações

| Atalho | Ação |
|--------|------|
| `Enter` | Ativa a linha (callback `onActivate`) |
| `Delete` | Deleta a linha (callback `onDelete`) |
| `F2` | Inicia edição da célula (se `editable: true`) |
| `Space` | Executa ação da célula (se `action: true`) |

## Edição

| Atalho | Ação |
|--------|------|
| `F2` | Inicia edição da célula focada |
| `Enter` (durante edição) | Salva e sai da edição |
| `Escape` (durante edição) | Cancela e sai da edição |

## Comportamento da Seleção

### Sem Modificadores
Navegação padrão **seleciona automaticamente** a linha atual (comportamento similar ao Explorer sem `Ctrl`).

### Com `Ctrl`
Navegação **mantém** a seleção existente, permitindo navegar sem alterar o que está selecionado.

### Com `Shift`
Navegação **estende a seleção** da última linha focada até a atual, criando um intervalo.

### `Ctrl+Space`
**Toggle individual**: adiciona/remove linha da seleção sem afetar outras, permitindo seleção não-contígua.

## Acessibilidade

- **ARIA Grid Pattern**: role="grid", role="gridcell"
- **Leitores de Tela**: Conteúdo das células é lido naturalmente
- **Focus Visível**: Borda azul de 3px na célula focada
- **Anúncios**: Ações discretas ("Marcado", "Salvo", "Cancelado")
- **TabIndex Dinâmico**: 0 para célula focada, -1 para outras
- **Contagem ARIA**: aria-rowcount e aria-colcount informam total de linhas/colunas
- **Feedback Sonoro**: Som de "bump" ao tentar navegar além dos limites (não circular)
- **Ações Acessíveis**: Emojis têm aria-label descritivo (ex: "Abrir conversa", "Excluir FAQ")

## Exemplos de Uso

### Seleção Contígua (intervalo)
1. Clique na linha 5
2. `Shift+↓` (3x) → Seleciona linhas 5-8
3. `Shift+Page Down` → Estende seleção em 10 linhas

### Seleção Não-Contígua (aleatória)
1. Clique na linha 2
2. `Ctrl+Space` → Seleciona linha 2
3. `↓` (5x) com `Ctrl` → Navega para linha 7 (mantém linha 2 selecionada)
4. `Ctrl+Space` → Adiciona linha 7 à seleção
5. Resultado: linhas 2 e 7 selecionadas

### Navegação Rápida
- `Ctrl+Home` → Vai para o topo
- `Ctrl+End` → Vai para o fim
- `Page Down` (3x) → Pula 30 linhas
- `Home` → Volta para primeira coluna

## Implementação

Ver [DataGrid.tsx](../src/components/ui/DataGrid.tsx) linhas 170-318 para a implementação completa da navegação.

A lógica de seleção com Shift é implementada nas linhas 290-315, criando intervalos automaticamente entre a posição anterior e a nova posição.
