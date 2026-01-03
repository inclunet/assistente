<script>
  /**
   * CardGrid - Grid de cards responsivo com navegação acessível
   * 
   * Layout CSS Grid com auto-fill que se adapta ao container.
   * Navegação por setas respeita a estrutura visual de linhas/colunas.
   * 
   * Navegação:
   * - ←/→: Move entre cards adjacentes
   * - ↑/↓: Move entre linhas (baseado em colunas visíveis)
   * - Home: Primeiro card da linha (Ctrl+Home: primeiro do grid)
   * - End: Último card da linha (Ctrl+End: último do grid)
   * - Enter/Espaço: Ativa o card
   * - Delete: Exclui o card
   * - Ctrl+A: Seleciona todos (quando multiSelect=true)
   * - Escape: Limpa seleção
   */
  
  import { onMount, createEventDispatcher, tick } from 'svelte';
  import { 
    createSelectionManager, 
    calculateLinearNavigation, 
    getGridPosition 
  } from './gridUtils.js';
  
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
  
  // Calcula colunas dinamicamente baseado no CSS Grid
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
  
  // Posição no grid para ARIA
  function getPosition(index) {
    const pos = getGridPosition(index, columnsCount);
    return {
      ...pos,
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
    let handled = false;
    let newIndex = focusedIndex;
    
    // Teclas de ação
    switch (key) {
      case 'Enter':
      case ' ':
        if (!target.closest('button, a, input')) {
          event.preventDefault();
          dispatch('activate', { item: items[focusedIndex], index: focusedIndex });
          return;
        }
        break;
        
      case 'Delete':
        event.preventDefault();
        dispatch('delete', { item: items[focusedIndex], index: focusedIndex });
        return;
        
      case 'Escape':
        event.preventDefault();
        if (multiSelect) {
          selection.clearSelection();
        }
        gridElement?.focus();
        return;
        
      case 'a':
        if (ctrlKey && multiSelect) {
          event.preventDefault();
          selection.selectAll();
          return;
        }
        break;
    }
    
    // Navegação usando utilitário compartilhado
    const navResult = calculateLinearNavigation(key, focusedIndex, itemCount, columnsCount, ctrlKey);
    
    if (navResult.handled) {
      event.preventDefault();
      event.stopPropagation();
      
      newIndex = navResult.newIndex;
      
      if (newIndex !== focusedIndex) {
        const oldIndex = focusedIndex;
        focusedIndex = newIndex;
        
        // Seleção durante navegação
        selection.handleNavigationWithModifiers(newIndex, oldIndex, { ctrlKey, shiftKey }, multiSelect);
        
        dispatch('focusChange', { index: focusedIndex, item: items[focusedIndex] });
        focusCell(focusedIndex);
      }
    }
  }
  
  function handleCellClick(index, event) {
    focusedIndex = index;
    selection.handleClickWithModifiers(index, event, multiSelect);
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
  
  .grid-card:focus,
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




