<script>
  /**
   * DataGrid - Grid de dados tabulares estilo Windows Explorer
   * 
   * Grid com colunas definidas, navegação 2D (linhas/colunas),
   * edição inline e seleção.
   * 
   * Navegação:
   * - ↑/↓: Move entre linhas
   * - ←/→: Move entre colunas
   * - Home: Primeira coluna (Ctrl+Home: primeira célula do grid)
   * - End: Última coluna (Ctrl+End: última célula do grid)
   * - Enter: Ativa o item (abre/edita)
   * - Delete: Exclui o item
   * - F2: Edita célula inline
   * - Espaço: Ativa botão de ação da célula
   * - Ctrl+A: Seleciona todos (quando multiSelect=true)
   * - Escape: Limpa seleção / Cancela edição
   */
  
  import { createEventDispatcher, tick } from 'svelte';
  import { 
    createSelectionManager, 
    calculate2DNavigation 
  } from './gridUtils.js';
  
  // Props
  export let items = [];
  export let columns = [];
  export let label = 'Grid de dados';
  export let getItemId = (item) => item.id;
  export let selectedIds = new Set();
  export let multiSelect = false;
  
  const dispatch = createEventDispatcher();
  
  let gridElement;
  let focusedRow = 0;
  let focusedCol = 0;
  
  // Estado de edição inline
  let editingRow = -1;
  let editingCol = -1;
  let editValue = '';
  let editInputElement;
  
  $: columnCount = columns.length;
  $: rowCount = items.length;
  
  // Gerenciador de seleção usando utilitário compartilhado
  const selection = createSelectionManager({
    getItemId,
    getItems: () => items,
    onSelectionChange: (ids) => {
      selectedIds = ids;
      dispatch('selectionChange', { selectedIds });
    }
  });
  
  // Sincroniza selectedIds externo com o gerenciador
  $: selection.setSelectedIds(selectedIds);
  
  function handleKeyDown(event) {
    if (items.length === 0 || columns.length === 0) return;
    
    // Se estamos editando, trata teclas de edição
    if (editingRow >= 0) {
      handleEditKeyDown(event);
      return;
    }
    
    const { key, ctrlKey, shiftKey } = event;
    
    let newRow = focusedRow;
    let newCol = focusedCol;
    let handled = false;
    
    // Teclas de ação
    switch (key) {
      case ' ':
        event.preventDefault();
        if (ctrlKey && multiSelect) {
          selection.toggleSelection(focusedRow);
        } else {
          // Espaço ativa a célula atual (botão de ação)
          const col = columns[focusedCol];
          if (col.action) {
            dispatch('cellAction', { 
              item: items[focusedRow], 
              column: col,
              rowIndex: focusedRow,
              colIndex: focusedCol
            });
          }
        }
        return;
        
      case 'Enter':
        event.preventDefault();
        dispatch('activate', { item: items[focusedRow], rowIndex: focusedRow });
        return;
        
      case 'Delete':
        event.preventDefault();
        dispatch('delete', { item: items[focusedRow], rowIndex: focusedRow });
        return;
        
      case 'F2':
        event.preventDefault();
        const currentCol = columns[focusedCol];
        if (!currentCol.action && currentCol.editable !== false) {
          startEditing(focusedRow, focusedCol);
        }
        return;
        
      case 'Escape':
        event.preventDefault();
        if (multiSelect) {
          selection.clearSelection();
        }
        return;
        
      case 'a':
        if (ctrlKey && multiSelect) {
          event.preventDefault();
          selection.selectAll();
          return;
        }
        break;
    }
    
    // Navegação 2D usando utilitário compartilhado
    const navResult = calculate2DNavigation(key, focusedRow, focusedCol, rowCount, columnCount, ctrlKey);
    
    if (navResult.handled) {
      event.preventDefault();
      event.stopPropagation();
      
      newRow = navResult.newRow;
      newCol = navResult.newCol;
      
      if (newRow !== focusedRow || newCol !== focusedCol) {
        const oldRow = focusedRow;
        focusedRow = newRow;
        focusedCol = newCol;
        
        // Seleção durante navegação (apenas quando muda de linha)
        if (newRow !== oldRow) {
          selection.handleNavigationWithModifiers(newRow, oldRow, { ctrlKey, shiftKey }, multiSelect);
        }
        
        dispatch('focusChange', { 
          item: items[focusedRow], 
          column: columns[focusedCol],
          rowIndex: focusedRow,
          colIndex: focusedCol
        });
        
        focusCell(focusedRow, focusedCol);
      }
    }
  }
  
  function handleEditKeyDown(event) {
    const { key } = event;
    
    switch (key) {
      case 'Enter':
        event.preventDefault();
        commitEdit();
        break;
        
      case 'Escape':
        event.preventDefault();
        cancelEdit();
        break;
        
      case 'Tab':
        event.preventDefault();
        commitEdit();
        // Move para próxima célula editável
        let nextCol = editingCol + 1;
        while (nextCol < columnCount && (columns[nextCol].action || columns[nextCol].editable === false)) {
          nextCol++;
        }
        if (nextCol < columnCount) {
          focusedCol = nextCol;
          focusCell(focusedRow, focusedCol);
        }
        break;
    }
  }
  
  async function startEditing(rowIndex, colIndex) {
    const column = columns[colIndex];
    const item = items[rowIndex];
    
    editingRow = rowIndex;
    editingCol = colIndex;
    editValue = item[column.key] ?? '';
    
    await tick();
    editInputElement?.focus();
    editInputElement?.select();
  }
  
  function commitEdit() {
    if (editingRow >= 0 && editingCol >= 0) {
      const column = columns[editingCol];
      const item = items[editingRow];
      const oldValue = item[column.key];
      
      if (editValue !== oldValue) {
        dispatch('edit', {
          item,
          column,
          rowIndex: editingRow,
          colIndex: editingCol,
          oldValue,
          newValue: editValue
        });
      }
      
      cancelEdit();
    }
  }
  
  function cancelEdit() {
    editingRow = -1;
    editingCol = -1;
    editValue = '';
    focusCell(focusedRow, focusedCol);
  }
  
  function handleCellClick(rowIndex, colIndex, event) {
    focusedRow = rowIndex;
    focusedCol = colIndex;
    
    selection.handleClickWithModifiers(rowIndex, event, multiSelect);
    
    dispatch('focusChange', { 
      item: items[rowIndex], 
      column: columns[colIndex],
      rowIndex,
      colIndex
    });
  }
  
  function handleCellDoubleClick(rowIndex, colIndex) {
    const column = columns[colIndex];
    
    if (!column.action && column.editable !== false) {
      startEditing(rowIndex, colIndex);
    } else if (!column.action) {
      dispatch('activate', { item: items[rowIndex], rowIndex });
    }
  }
  
  async function focusCell(rowIndex, colIndex) {
    await tick();
    const cell = gridElement?.querySelector(
      `[data-row="${rowIndex}"][data-col="${colIndex}"]`
    );
    cell?.focus();
  }
  
  export function focus() {
    if (items.length > 0 && columns.length > 0) {
      focusCell(focusedRow, focusedCol);
    }
  }
  
  function getCellValue(item, column) {
    if (column.format) {
      return column.format(item[column.key], item);
    }
    return item[column.key] ?? '';
  }
</script>

{#if items.length > 0}
  <div
    bind:this={gridElement}
    class="data-grid"
    role="grid"
    aria-label={label}
    aria-rowcount={rowCount}
    aria-colcount={columnCount}
    aria-multiselectable={multiSelect || undefined}
    on:keydown={handleKeyDown}
  >
    <!-- Cabeçalho -->
    <div role="rowgroup" class="grid-header-group">
      <div role="row" class="grid-header">
        {#each columns as column, colIndex}
          <div 
            role="columnheader" 
            class="grid-header-cell"
            aria-colindex={colIndex + 1}
            style={column.width ? `width: ${column.width}; flex: none;` : ''}
          >
            {column.label}
          </div>
        {/each}
      </div>
    </div>
    
    <!-- Corpo -->
    <div role="rowgroup" class="grid-body">
      {#each items as item, rowIndex (getItemId(item))}
        {@const isSelected = selectedIds.has(getItemId(item))}
        {@const isRowFocused = rowIndex === focusedRow}
        <div 
          role="row" 
          class="grid-row"
          class:selected={isSelected}
          class:focused={isRowFocused}
          aria-rowindex={rowIndex + 1}
          aria-selected={multiSelect ? isSelected : undefined}
        >
          {#each columns as column, colIndex}
            {@const isCellFocused = isRowFocused && colIndex === focusedCol}
            {@const isEditing = editingRow === rowIndex && editingCol === colIndex}
            <div
              role="gridcell"
              class="grid-cell"
              class:cell-focused={isCellFocused}
              class:cell-action={column.action}
              class:cell-editing={isEditing}
              aria-colindex={colIndex + 1}
              data-row={rowIndex}
              data-col={colIndex}
              tabindex={isCellFocused ? 0 : -1}
              on:click={(e) => handleCellClick(rowIndex, colIndex, e)}
              on:dblclick={() => handleCellDoubleClick(rowIndex, colIndex)}
              style={column.width ? `width: ${column.width}; flex: none;` : ''}
            >
              {#if isEditing}
                <input
                  bind:this={editInputElement}
                  type="text"
                  class="cell-edit-input"
                  bind:value={editValue}
                  on:blur={commitEdit}
                />
              {:else if column.action}
                {#if !column.showIf || column.showIf(item)}
                  <button
                    class="cell-button"
                    tabindex="-1"
                    aria-label={column.label}
                    on:click|stopPropagation={() => dispatch('cellAction', { item, column, rowIndex, colIndex })}
                  >
                    {column.actionIcon || column.label}
                  </button>
                {/if}
              {:else}
                <span class="cell-content" class:cell-truncate={column.truncate}>
                  {getCellValue(item, column)}
                </span>
              {/if}
            </div>
          {/each}
        </div>
      {/each}
    </div>
  </div>
{:else}
  <slot name="empty">
    <p class="empty-message">Nenhum item encontrado.</p>
  </slot>
{/if}

<style>
  .data-grid {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--color-border, #404040);
    border-radius: var(--border-radius, 8px);
    overflow: hidden;
    outline: none;
  }
  
  .grid-header-group {
    display: contents;
  }
  
  .grid-header {
    display: flex;
    background: var(--color-bg-secondary, #252525);
    border-bottom: 1px solid var(--color-border, #404040);
  }
  
  .grid-header-cell {
    flex: 1;
    padding: var(--spacing-sm, 8px) var(--spacing-md, 12px);
    font-weight: 600;
    font-size: var(--font-size-sm, 13px);
    color: var(--color-text-muted, #888);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  
  .grid-body {
    display: flex;
    flex-direction: column;
    max-height: 400px;
    overflow-y: auto;
  }
  
  .grid-row {
    display: flex;
    border-bottom: 1px solid var(--color-border, #404040);
    transition: background-color 0.15s;
  }
  
  .grid-row:last-child {
    border-bottom: none;
  }
  
  .grid-row:hover {
    background: var(--color-bg-tertiary, #2d2d2d);
  }
  
  .grid-row.selected {
    background: rgba(88, 166, 255, 0.15);
  }
  
  .grid-row.focused {
    background: var(--color-bg-tertiary, #2d2d2d);
  }
  
  .grid-row.selected.focused {
    background: rgba(88, 166, 255, 0.25);
  }
  
  .grid-cell {
    flex: 1;
    padding: var(--spacing-sm, 8px) var(--spacing-md, 12px);
    display: flex;
    align-items: center;
    outline: none;
    min-height: 44px;
  }
  
  .grid-cell.cell-focused {
    outline: 2px solid var(--color-accent, #58a6ff);
    outline-offset: -2px;
    background: rgba(88, 166, 255, 0.1);
  }
  
  .grid-cell.cell-action {
    flex: 0 0 auto;
    justify-content: center;
  }
  
  .grid-cell.cell-editing {
    padding: 4px;
  }
  
  .cell-content {
    color: var(--color-text-primary, #e0e0e0);
    font-size: var(--font-size-base, 14px);
  }
  
  .cell-truncate {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: block;
    max-width: 100%;
  }
  
  .cell-edit-input {
    width: 100%;
    padding: var(--spacing-xs, 4px) var(--spacing-sm, 8px);
    border: 2px solid var(--color-accent, #58a6ff);
    border-radius: var(--border-radius, 8px);
    background: var(--color-bg-input, #1a1a1a);
    color: var(--color-text-primary, #e0e0e0);
    font-size: var(--font-size-base, 14px);
    outline: none;
  }
  
  .cell-button {
    background: var(--color-bg-secondary, #252525);
    border: 1px solid var(--color-border, #404040);
    border-radius: var(--border-radius, 8px);
    padding: var(--spacing-xs, 4px) var(--spacing-sm, 8px);
    cursor: pointer;
    font-size: var(--font-size-base, 14px);
    color: var(--color-text-primary, #e0e0e0);
    transition: all 0.15s;
  }
  
  .cell-button:hover {
    background: var(--color-accent, #58a6ff);
    border-color: var(--color-accent, #58a6ff);
    color: white;
  }
  
  .cell-button:focus {
    outline: 2px solid var(--color-accent, #58a6ff);
    outline-offset: 2px;
  }
  
  .empty-message {
    text-align: center;
    padding: var(--spacing-lg, 20px);
    color: var(--color-text-muted, #888);
  }
</style>

