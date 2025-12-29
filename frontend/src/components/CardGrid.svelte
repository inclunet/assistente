<script>
  import { onMount, createEventDispatcher, tick } from 'svelte';
  
  // Props
  export let items = [];
  export let label = 'Grid de itens';
  export let getItemId = (item) => item.id;
  export let selectedIds = new Set();
  export let multiSelect = false;
  export let focusedIndex = 0;
  
  const dispatch = createEventDispatcher();
  
  let gridElement;
  let columnsCount = 1;
  
  // Calcula colunas dinamicamente
  function updateColumnsCount() {
    if (!gridElement) return;
    const style = window.getComputedStyle(gridElement);
    const cols = style.getPropertyValue('grid-template-columns');
    const newCount = cols.split(' ').filter(c => c.trim()).length || 1;
    if (newCount !== columnsCount) {
      columnsCount = newCount;
    }
  }
  
  onMount(() => {
    setTimeout(updateColumnsCount, 100);
    
    const observer = new ResizeObserver(() => {
      updateColumnsCount();
    });
    observer.observe(gridElement);
    return () => observer.disconnect();
  });
  
  // Posição no grid
  function getPosition(index) {
    return {
      row: Math.floor(index / columnsCount) + 1,
      col: (index % columnsCount) + 1,
      totalRows: Math.ceil(items.length / columnsCount),
      totalCols: columnsCount
    };
  }
  
  // Navegação por teclado
  function handleGridKeyDown(event) {
    if (items.length === 0) return;
    
    const { key, shiftKey, ctrlKey, target } = event;
    
    // Se estamos em um elemento interativo, permite comportamento padrão (exceto Escape)
    if ((target.tagName === 'BUTTON' || target.tagName === 'A' || target.tagName === 'INPUT') && key !== 'Escape') {
      return;
    }
    
    const itemCount = items.length;
    let newIndex = focusedIndex;
    let handled = false;
    
    switch (key) {
      case 'ArrowRight':
        if (focusedIndex < itemCount - 1) {
          newIndex = focusedIndex + 1;
          handled = true;
        }
        break;
        
      case 'ArrowLeft':
        if (focusedIndex > 0) {
          newIndex = focusedIndex - 1;
          handled = true;
        }
        break;
        
      case 'ArrowDown':
        {
          const targetIndex = focusedIndex + columnsCount;
          if (targetIndex < itemCount) {
            newIndex = targetIndex;
          } else if (focusedIndex < itemCount - 1) {
            newIndex = itemCount - 1;
          }
          handled = true;
        }
        break;
        
      case 'ArrowUp':
        {
          const targetIndex = focusedIndex - columnsCount;
          if (targetIndex >= 0) {
            newIndex = targetIndex;
          } else if (focusedIndex > 0) {
            newIndex = 0;
          }
          handled = true;
        }
        break;
        
      case 'Home':
        if (ctrlKey) {
          newIndex = 0;
        } else {
          newIndex = Math.floor(focusedIndex / columnsCount) * columnsCount;
        }
        handled = true;
        break;
        
      case 'End':
        if (ctrlKey) {
          newIndex = itemCount - 1;
        } else {
          const rowStart = Math.floor(focusedIndex / columnsCount) * columnsCount;
          newIndex = Math.min(rowStart + columnsCount - 1, itemCount - 1);
        }
        handled = true;
        break;
        
      case 'Enter':
      case ' ':
        if (!target.closest('button, a, input')) {
          event.preventDefault();
          dispatch('activate', { item: items[focusedIndex], index: focusedIndex });
          handled = true;
        }
        break;
        
      case 'Delete':
        event.preventDefault();
        dispatch('delete', { item: items[focusedIndex], index: focusedIndex });
        handled = true;
        break;
        
      case 'Escape':
        event.preventDefault();
        if (multiSelect) {
          selectedIds = new Set();
          dispatch('selectionChange', { selectedIds });
        }
        gridElement?.focus();
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
    }
    
    if (handled) {
      event.preventDefault();
      event.stopPropagation();
      
      if (newIndex !== focusedIndex) {
        focusedIndex = newIndex;
        
        if (multiSelect) {
          if (shiftKey) {
            toggleSelection(focusedIndex, true);
          } else if (!ctrlKey) {
            selectedIds = new Set([getItemId(items[focusedIndex])]);
            dispatch('selectionChange', { selectedIds });
          }
        }
        
        dispatch('focusChange', { index: focusedIndex, item: items[focusedIndex] });
        focusCell(focusedIndex);
      }
    }
  }
  
  function toggleSelection(index, addToSelection = false) {
    const id = getItemId(items[index]);
    if (addToSelection) {
      if (selectedIds.has(id)) {
        selectedIds.delete(id);
      } else {
        selectedIds.add(id);
      }
    } else {
      selectedIds = new Set([id]);
    }
    selectedIds = selectedIds;
    dispatch('selectionChange', { selectedIds });
  }
  
  function handleCellClick(index, event) {
    focusedIndex = index;
    
    if (multiSelect) {
      if (event.shiftKey || event.ctrlKey) {
        toggleSelection(index, true);
      } else {
        selectedIds = new Set([getItemId(items[index])]);
        dispatch('selectionChange', { selectedIds });
      }
    }
    
    dispatch('focusChange', { index, item: items[index] });
  }
  
  function handleCellDoubleClick(index) {
    dispatch('activate', { item: items[index], index });
  }
  
  function handleCellKeyDown(index, event) {
    if (event.key === 'Enter' || event.key === ' ') {
      if (!event.target.closest('button, a, input')) {
        event.preventDefault();
        dispatch('activate', { item: items[index], index });
      }
    }
  }
  
  async function focusCell(index) {
    await tick();
    const cells = gridElement?.querySelectorAll('[role="gridcell"]');
    cells?.[index]?.focus();
  }
  
  export function focus() {
    if (items.length > 0) {
      focusCell(focusedIndex);
    }
  }
</script>

{#if items.length > 0}
  <!--
    Grid structure for screen readers:
    - role="grid" on container
    - role="rowgroup" wraps all rows (implicit)
    - role="row" for each visual row
    - role="gridcell" for each card
    
    The grid uses CSS Grid for layout with auto-fill columns.
    Navigation: Arrow keys move between cells, respecting row/column structure.
  -->
  <div
    bind:this={gridElement}
    class="card-grid"
    role="grid"
    aria-label={label}
    aria-rowcount={Math.ceil(items.length / columnsCount)}
    aria-colcount={columnsCount}
    aria-multiselectable={multiSelect || undefined}
    on:keydown={handleGridKeyDown}
  >
    {#each items as item, index (getItemId(item))}
      {@const pos = getPosition(index)}
      {@const isSelected = selectedIds.has(getItemId(item))}
      {@const isFocused = index === focusedIndex}
      <article
        role="gridcell"
        class="grid-card"
        class:selected={isSelected}
        class:focused={isFocused}
        aria-rowindex={pos.row}
        aria-colindex={pos.col}
        aria-selected={multiSelect ? isSelected : undefined}
        aria-label="{pos.row}, {pos.col}"
        tabindex={isFocused ? 0 : -1}
        on:click={(e) => handleCellClick(index, e)}
        on:dblclick={() => handleCellDoubleClick(index)}
        on:keydown={(e) => handleCellKeyDown(index, e)}
      >
        <slot {item} {index} {isSelected} {isFocused} {pos} />
      </article>
    {/each}
  </div>
{:else}
  <slot name="empty">
    <p class="empty-message">Nenhum item encontrado.</p>
  </slot>
{/if}

<style>
  .card-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(var(--card-min-width, 240px), 1fr));
    gap: var(--spacing-md, 16px);
    padding: var(--spacing-xs, 4px);
    outline: none;
  }
  
  .grid-card {
    position: relative;
    display: flex;
    flex-direction: column;
    padding: var(--spacing-lg, 20px) var(--spacing-md, 16px);
    background: var(--color-bg-tertiary, #2d2d2d);
    border: 2px solid var(--color-border, #404040);
    border-radius: 12px;
    cursor: pointer;
    transition: all 0.2s ease;
    outline: none;
  }
  
  .grid-card:hover {
    border-color: var(--color-accent, #58a6ff);
    transform: translateY(-2px);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  }
  
  .grid-card:focus {
    border-color: var(--color-accent, #58a6ff);
    box-shadow: 0 0 0 3px rgba(88, 166, 255, 0.3);
  }
  
  .grid-card.focused {
    border-color: var(--color-accent, #58a6ff);
    box-shadow: 0 0 0 3px rgba(88, 166, 255, 0.3);
  }
  
  .grid-card.selected {
    background: linear-gradient(135deg, var(--color-accent, #58a6ff) 0%, #7c3aed 100%);
    border-color: transparent;
    color: white;
  }
  
  .grid-card.selected :global(.text-muted) {
    color: rgba(255, 255, 255, 0.8);
  }
  
  .empty-message {
    text-align: center;
    padding: var(--spacing-lg, 20px);
    color: var(--color-text-muted, #888);
  }
</style>
