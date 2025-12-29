<script>
  import { onMount, createEventDispatcher, tick } from 'svelte';
  
  /**
   * DataGrid - Grid de dados acessível estilo Windows Explorer
   * 
   * Navegação:
   * - ↑/↓: Move entre linhas
   * - ←/→: Move entre células na linha
   * - Home: Primeira célula da linha
   * - End: Última célula da linha
   * - Ctrl+Home: Primeira célula do grid
   * - Ctrl+End: Última célula do grid
   * - Enter: Ação primária (abrir/editar item)
   * - Delete: Excluir item
   * - F2: Editar célula inline (quando aplicável)
   * 
   * Seleção (quando multiSelect=true):
   * - Shift+↑/↓: Expande seleção
   * - Ctrl+↑/↓: Navega sem alterar seleção
   * - Ctrl+Espaço: Toggle seleção da linha atual
   * - Ctrl+A: Seleciona todas
   * - Escape: Limpa seleção / Cancela edição
   */
  
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
  let anchorRow = -1; // Âncora para seleção com Shift
  
  // Estado de edição inline
  let editingRow = -1;
  let editingCol = -1;
  let editValue = '';
  let editInputElement;
  
  $: columnCount = columns.length;
  $: rowCount = items.length;
  
  // Encontra o índice da primeira coluna que serve como "título"
  $: titleColumnIndex = columns.findIndex(c => !c.action) ?? 0;
  
  // Encontra a primeira coluna de ação (para Enter como ação primária)
  $: primaryActionIndex = columns.findIndex(c => c.action && c.key !== 'delete');
  
  function handleKeyDown(event) {
    if (items.length === 0 || columns.length === 0) return;
    
    // Se estamos editando, trata teclas de edição
    if (editingRow >= 0) {
      handleEditKeyDown(event);
      return;
    }
    
    const { key, ctrlKey, shiftKey, target } = event;
    
    let newRow = focusedRow;
    let newCol = focusedCol;
    let handled = false;
    let selectionChanged = false;
    
    switch (key) {
      case 'ArrowRight':
        if (focusedCol < columnCount - 1) {
          newCol = focusedCol + 1;
          handled = true;
        }
        break;
        
      case 'ArrowLeft':
        if (focusedCol > 0) {
          newCol = focusedCol - 1;
          handled = true;
        }
        break;
        
      case 'ArrowDown':
        if (focusedRow < rowCount - 1) {
          newRow = focusedRow + 1;
          handled = true;
          
          if (multiSelect) {
            if (shiftKey && !ctrlKey) {
              if (anchorRow === -1) anchorRow = focusedRow;
              selectRange(anchorRow, newRow);
              selectionChanged = true;
            } else if (!ctrlKey && !shiftKey) {
              selectSingle(newRow);
              anchorRow = newRow;
              selectionChanged = true;
            }
          }
        }
        break;
        
      case 'ArrowUp':
        if (focusedRow > 0) {
          newRow = focusedRow - 1;
          handled = true;
          
          if (multiSelect) {
            if (shiftKey && !ctrlKey) {
              if (anchorRow === -1) anchorRow = focusedRow;
              selectRange(anchorRow, newRow);
              selectionChanged = true;
            } else if (!ctrlKey && !shiftKey) {
              selectSingle(newRow);
              anchorRow = newRow;
              selectionChanged = true;
            }
          }
        }
        break;
        
      case 'Home':
        if (ctrlKey) {
          newRow = 0;
          newCol = 0;
        } else {
          newCol = 0;
        }
        handled = true;
        
        if (multiSelect && !ctrlKey && !shiftKey && newRow !== focusedRow) {
          selectSingle(newRow);
          anchorRow = newRow;
          selectionChanged = true;
        }
        break;
        
      case 'End':
        if (ctrlKey) {
          newRow = rowCount - 1;
          newCol = columnCount - 1;
        } else {
          newCol = columnCount - 1;
        }
        handled = true;
        
        if (multiSelect && !ctrlKey && !shiftKey && newRow !== focusedRow) {
          selectSingle(newRow);
          anchorRow = newRow;
          selectionChanged = true;
        }
        break;
        
      case ' ':
        event.preventDefault();
        
        if (ctrlKey && multiSelect) {
          toggleSelection(focusedRow);
          selectionChanged = true;
          handled = true;
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
          handled = true;
        }
        break;
        
      case 'Enter':
        event.preventDefault();
        // Enter sempre aciona a ação primária (abrir/editar o item)
        dispatch('activate', { item: items[focusedRow], rowIndex: focusedRow });
        handled = true;
        break;
        
      case 'Delete':
        event.preventDefault();
        dispatch('delete', { item: items[focusedRow], rowIndex: focusedRow });
        handled = true;
        break;
        
      case 'F2':
        event.preventDefault();
        // F2 inicia edição inline se a coluna atual for editável
        const currentCol = columns[focusedCol];
        if (!currentCol.action && currentCol.editable !== false) {
          startEditing(focusedRow, focusedCol);
        }
        handled = true;
        break;
        
      case 'Escape':
        event.preventDefault();
        if (multiSelect) {
          selectedIds = new Set();
          anchorRow = -1;
          dispatch('selectionChange', { selectedIds });
        }
        handled = true;
        break;
        
      case 'a':
        if (ctrlKey && multiSelect) {
          event.preventDefault();
          selectedIds = new Set(items.map(getItemId));
          dispatch('selectionChange', { selectedIds });
          handled = true;
        }
        break;
        
      case 'PageDown':
        event.preventDefault();
        if (ctrlKey) {
          // Ctrl+PageDown: última linha, mesma coluna
          newRow = rowCount - 1;
        } else {
          // PageDown: 10 linhas para baixo
          newRow = Math.min(focusedRow + 10, rowCount - 1);
        }
        handled = true;
        
        if (multiSelect && !ctrlKey && !shiftKey) {
          selectSingle(newRow);
          anchorRow = newRow;
          selectionChanged = true;
        }
        break;
        
      case 'PageUp':
        event.preventDefault();
        if (ctrlKey) {
          // Ctrl+PageUp: primeira linha, mesma coluna
          newRow = 0;
        } else {
          // PageUp: 10 linhas para cima
          newRow = Math.max(focusedRow - 10, 0);
        }
        handled = true;
        
        if (multiSelect && !ctrlKey && !shiftKey) {
          selectSingle(newRow);
          anchorRow = newRow;
          selectionChanged = true;
        }
        break;
    }
    
    if (handled) {
      event.preventDefault();
      event.stopPropagation();
      
      if (newRow !== focusedRow || newCol !== focusedCol) {
        focusedRow = newRow;
        focusedCol = newCol;
        
        dispatch('focusChange', { 
          item: items[focusedRow], 
          column: columns[focusedCol],
          rowIndex: focusedRow,
          colIndex: focusedCol
        });
        
        focusCell(focusedRow, focusedCol);
      }
      
      if (selectionChanged) {
        dispatch('selectionChange', { selectedIds });
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
  
  function selectSingle(rowIndex) {
    selectedIds = new Set([getItemId(items[rowIndex])]);
  }
  
  function selectRange(fromRow, toRow) {
    const start = Math.min(fromRow, toRow);
    const end = Math.max(fromRow, toRow);
    selectedIds = new Set();
    for (let i = start; i <= end; i++) {
      selectedIds.add(getItemId(items[i]));
    }
  }
  
  function toggleSelection(rowIndex) {
    const id = getItemId(items[rowIndex]);
    if (selectedIds.has(id)) {
      selectedIds.delete(id);
    } else {
      selectedIds.add(id);
    }
    selectedIds = selectedIds;
  }
  
  function handleCellClick(rowIndex, colIndex, event) {
    const { ctrlKey, shiftKey } = event;
    
    focusedRow = rowIndex;
    focusedCol = colIndex;
    
    if (multiSelect) {
      if (shiftKey && !ctrlKey) {
        if (anchorRow === -1) anchorRow = rowIndex;
        selectRange(anchorRow, rowIndex);
      } else if (ctrlKey && !shiftKey) {
        toggleSelection(rowIndex);
      } else {
        selectSingle(rowIndex);
        anchorRow = rowIndex;
      }
      dispatch('selectionChange', { selectedIds });
    }
    
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
  
  function handleRowDoubleClick(rowIndex) {
    dispatch('activate', { item: items[rowIndex], rowIndex });
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
