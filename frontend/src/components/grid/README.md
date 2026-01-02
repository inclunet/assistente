# Grid Components

Componentes de grid acessíveis com navegação por teclado, seleção e suporte a ARIA.

## Componentes

| Componente | Descrição |
|------------|-----------|
| `DataGrid` | Grid tabular com colunas definidas, edição inline, navegação 2D |
| `CardGrid` | Grid de cards responsivo com slots, navegação 1D |

## Features Comuns

- ✅ **Navegação por teclado** - Setas, Home, End, PageUp/Down
- ✅ **Seleção** - Única ou múltipla (Shift/Ctrl+click)
- ✅ **ARIA completo** - role="grid", aria-selected, aria-rowindex, etc.
- ✅ **Theming** - CSS custom properties
- ✅ **Eventos** - activate, delete, focusChange, selectionChange

## Instalação

```javascript
import { DataGrid, CardGrid } from './components/grid';
```

---

# DataGrid

Grid tabular com colunas definidas, estilo Windows Explorer.

## Uso

```svelte
<script>
  import { DataGrid } from './components/grid';
  
  let items = [
    { id: 1, name: 'Item 1', status: 'Ativo' },
    { id: 2, name: 'Item 2', status: 'Inativo' }
  ];
  
  const columns = [
    { key: 'name', label: 'Nome' },
    { key: 'status', label: 'Status', width: '100px' },
    { key: 'edit', label: 'Editar', action: true, actionIcon: '✏️' },
    { key: 'delete', label: 'Excluir', action: true, actionIcon: '🗑️' }
  ];
  
  function handleActivate(e) {
    console.log('Ativou:', e.detail.item);
  }
  
  function handleCellAction(e) {
    const { item, column } = e.detail;
    if (column.key === 'delete') {
      // Excluir item
    }
  }
</script>

<DataGrid 
  {items}
  {columns}
  label="Lista de itens"
  on:activate={handleActivate}
  on:cellAction={handleCellAction}
/>
```

## Props

| Prop | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `items` | `array` | `[]` | Lista de items |
| `columns` | `array` | `[]` | Definição de colunas |
| `label` | `string` | `'Grid de dados'` | Label para acessibilidade |
| `getItemId` | `function` | `(item) => item.id` | Função para obter ID do item |
| `selectedIds` | `Set` | `new Set()` | IDs selecionados |
| `multiSelect` | `boolean` | `false` | Permite seleção múltipla |

### Definição de Coluna

```typescript
interface Column {
  key: string;          // Chave do dado no item
  label: string;        // Label exibido no cabeçalho
  width?: string;       // Largura fixa (ex: '100px')
  truncate?: boolean;   // Trunca texto longo com ellipsis
  format?: (value, item) => string;  // Formata o valor
  editable?: boolean;   // Permite edição inline (default: true)
  action?: boolean;     // É uma coluna de ação (botão)
  actionIcon?: string;  // Ícone/texto do botão
  showIf?: (item) => boolean;  // Condição para mostrar botão
}
```

## Eventos

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `activate` | `{ item, rowIndex }` | Enter ou duplo-clique |
| `delete` | `{ item, rowIndex }` | Tecla Delete |
| `edit` | `{ item, column, rowIndex, colIndex, oldValue, newValue }` | Edição inline |
| `cellAction` | `{ item, column, rowIndex, colIndex }` | Clique em botão de ação |
| `focusChange` | `{ item, column, rowIndex, colIndex }` | Foco mudou de célula |
| `selectionChange` | `{ selectedIds }` | Seleção mudou |

## Navegação

| Tecla | Ação |
|-------|------|
| `↑/↓` | Move entre linhas |
| `←/→` | Move entre colunas |
| `Home` | Primeira coluna |
| `End` | Última coluna |
| `Ctrl+Home` | Primeira célula do grid |
| `Ctrl+End` | Última célula do grid |
| `PageUp/Down` | Move 10 linhas |
| `Enter` | Ativa item |
| `Delete` | Exclui item |
| `F2` | Inicia edição inline |
| `Espaço` | Ativa botão de ação / Toggle seleção (Ctrl+Espaço) |
| `Escape` | Limpa seleção / Cancela edição |
| `Ctrl+A` | Seleciona todos |

---

# CardGrid

Grid de cards responsivo com slots para conteúdo customizado.

## Uso

```svelte
<script>
  import { CardGrid } from './components/grid';
  
  let items = [
    { id: 1, title: 'Card 1', description: 'Descrição...' },
    { id: 2, title: 'Card 2', description: 'Descrição...' }
  ];
  
  function handleActivate(e) {
    console.log('Ativou:', e.detail.item);
  }
</script>

<CardGrid 
  {items}
  label="Galeria de cards"
  on:activate={handleActivate}
  let:item
  let:isSelected
  let:isFocused
>
  <h3>{item.title}</h3>
  <p class="text-muted">{item.description}</p>
</CardGrid>
```

## Props

| Prop | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `items` | `array` | `[]` | Lista de items |
| `label` | `string` | `'Grid de itens'` | Label para acessibilidade |
| `getItemId` | `function` | `(item) => item.id` | Função para obter ID do item |
| `selectedIds` | `Set` | `new Set()` | IDs selecionados |
| `multiSelect` | `boolean` | `false` | Permite seleção múltipla |
| `focusedIndex` | `number` | `0` | Índice do item focado |

## Slots

### Slot Padrão

Recebe as seguintes props:

| Prop | Tipo | Descrição |
|------|------|-----------|
| `item` | `object` | O item atual |
| `index` | `number` | Índice do item |
| `isSelected` | `boolean` | Se está selecionado |
| `isFocused` | `boolean` | Se está focado |
| `pos` | `{ row, col }` | Posição visual no grid |

### Slot "empty"

Exibido quando não há items:

```svelte
<CardGrid {items}>
  <!-- slot padrão -->
  
  <div slot="empty">
    <p>Nenhum item encontrado</p>
    <button>Criar novo</button>
  </div>
</CardGrid>
```

## Eventos

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `activate` | `{ item, index }` | Enter/Espaço ou duplo-clique |
| `delete` | `{ item, index }` | Tecla Delete |
| `focusChange` | `{ item, index }` | Foco mudou de card |
| `selectionChange` | `{ selectedIds }` | Seleção mudou |

## Navegação

| Tecla | Ação |
|-------|------|
| `←/→` | Move para card anterior/próximo |
| `↑/↓` | Move para linha acima/abaixo (baseado em colunas visíveis) |
| `Home` | Primeiro card da linha |
| `End` | Último card da linha |
| `Ctrl+Home` | Primeiro card do grid |
| `Ctrl+End` | Último card do grid |
| `PageUp/Down` | Move 3 linhas |
| `Enter/Espaço` | Ativa card |
| `Delete` | Exclui card |
| `Escape` | Limpa seleção |
| `Ctrl+A` | Seleciona todos |

## Layout Responsivo

O CardGrid usa CSS Grid com `auto-fill`:

```css
.card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(var(--card-min-width, 240px), 1fr));
  gap: var(--spacing-md, 16px);
}
```

Personalize com CSS custom properties:

```css
.my-grid {
  --card-min-width: 300px;  /* Largura mínima dos cards */
  --spacing-md: 24px;       /* Gap entre cards */
}
```

---

# Estilização

Ambos os componentes usam CSS custom properties:

```css
/* Cores */
--color-bg-secondary    /* Fundo do cabeçalho */
--color-bg-tertiary     /* Fundo de linhas/cards */
--color-border          /* Bordas */
--color-text-primary    /* Texto principal */
--color-text-muted      /* Texto secundário */
--color-accent          /* Cor de destaque, seleção */

/* Espaçamentos */
--spacing-xs            /* Padding pequeno */
--spacing-sm            /* Padding médio */
--spacing-md            /* Padding grande */
--spacing-lg            /* Padding extra grande */

/* Outros */
--border-radius         /* Arredondamento */
--font-size-sm          /* Fonte pequena */
--font-size-base        /* Fonte padrão */
```

---

# Utilitários (gridUtils.js)

Para uso avançado ou criação de grids customizados:

```javascript
import { 
  createSelectionManager, 
  calculateLinearNavigation,
  calculate2DNavigation 
} from './components/grid';

// Gerenciador de seleção
const selection = createSelectionManager({
  getItemId: (item) => item.id,
  getItems: () => myItems,
  onSelectionChange: (ids) => console.log('Seleção:', ids)
});

selection.selectSingle(0);
selection.selectRange(0, 5);
selection.toggleSelection(2);
selection.selectAll();
selection.clearSelection();

// Navegação 1D (para grids tipo CardGrid)
const { newIndex, handled } = calculateLinearNavigation(
  'ArrowDown',  // tecla
  currentIndex, // índice atual
  itemCount,    // total de items
  columnsCount, // colunas visíveis
  ctrlKey       // Ctrl pressionado
);

// Navegação 2D (para grids tipo DataGrid)
const { newRow, newCol, handled } = calculate2DNavigation(
  'ArrowRight', // tecla
  currentRow,   // linha atual
  currentCol,   // coluna atual
  rowCount,     // total de linhas
  colCount,     // total de colunas
  ctrlKey       // Ctrl pressionado
);
```

---

# Migração

Se você estava importando diretamente:

```javascript
// Antes
import DataGrid from './components/DataGrid.svelte';
import CardGrid from './components/CardGrid.svelte';

// Depois
import { DataGrid, CardGrid } from './components/grid';
```

