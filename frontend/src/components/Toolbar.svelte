<script>
  import { onMount, tick } from 'svelte';
  
  // Props
  export let label = 'Barra de ferramentas';
  
  // Estado
  let toolbarElement;
  let focusedIndex = 0;
  
  // Obtém todos os elementos focáveis na toolbar
  function getFocusableItems() {
    if (!toolbarElement) return [];
    // Seleciona botões e elementos com role="button" que não estão desabilitados
    return Array.from(toolbarElement.querySelectorAll(
      'button:not([disabled]), [role="button"]:not([aria-disabled="true"]), .picker-button:not([disabled])'
    ));
  }
  
  // Atualiza tabindex de todos os itens
  function updateTabIndexes() {
    const items = getFocusableItems();
    items.forEach((item, index) => {
      item.setAttribute('tabindex', index === focusedIndex ? '0' : '-1');
    });
  }
  
  // Foca no item atual
  function focusCurrentItem() {
    const items = getFocusableItems();
    if (items[focusedIndex]) {
      items[focusedIndex].focus();
    }
  }
  
  function handleKeyDown(event) {
    const items = getFocusableItems();
    if (items.length === 0) return;
    
    let handled = false;
    
    if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
      event.preventDefault();
      focusedIndex = (focusedIndex + 1) % items.length;
      handled = true;
    } else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
      event.preventDefault();
      focusedIndex = (focusedIndex - 1 + items.length) % items.length;
      handled = true;
    } else if (event.key === 'Home') {
      event.preventDefault();
      focusedIndex = 0;
      handled = true;
    } else if (event.key === 'End') {
      event.preventDefault();
      focusedIndex = items.length - 1;
      handled = true;
    }
    
    if (handled) {
      updateTabIndexes();
      focusCurrentItem();
    }
  }
  
  // Quando um item recebe foco (via clique ou programático)
  function handleFocusIn(event) {
    const items = getFocusableItems();
    const index = items.indexOf(event.target);
    if (index >= 0) {
      focusedIndex = index;
      updateTabIndexes();
    }
  }
  
  onMount(() => {
    // Inicializa tabindexes
    tick().then(updateTabIndexes);
    
    // Observer para detectar mudanças nos itens
    const observer = new MutationObserver(() => {
      tick().then(updateTabIndexes);
    });
    
    observer.observe(toolbarElement, { 
      childList: true, 
      subtree: true,
      attributes: true,
      attributeFilter: ['disabled']
    });
    
    return () => observer.disconnect();
  });
</script>

<div 
  bind:this={toolbarElement}
  class="toolbar"
  role="toolbar"
  aria-label={label}
  on:keydown={handleKeyDown}
  on:focusin={handleFocusIn}
>
  <slot />
</div>

<style>
  .toolbar {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm) var(--spacing-lg);
    background: var(--color-bg-tertiary, #1e1e1e);
    border-bottom: 1px solid var(--color-border);
    flex-wrap: wrap;
  }
  
  /* Estilos para elementos dentro da toolbar via slot */
  .toolbar :global(.toolbar-separator) {
    width: 1px;
    height: 24px;
    background: var(--color-border);
    flex-shrink: 0;
  }
  
  .toolbar :global(.toolbar-btn) {
    display: flex;
    align-items: center;
    gap: var(--spacing-xs);
    padding: var(--spacing-xs) var(--spacing-sm);
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    color: var(--color-text-primary);
    font-size: var(--font-size-sm);
    cursor: pointer;
    transition: all 0.15s;
    min-height: 36px;
  }
  
  .toolbar :global(.toolbar-btn:hover:not(:disabled)) {
    background: var(--color-bg-primary);
    border-color: var(--color-accent);
  }
  
  .toolbar :global(.toolbar-btn:focus) {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
  }
  
  .toolbar :global(.toolbar-btn:disabled) {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .toolbar :global(.toolbar-spacer) {
    flex: 1;
  }
  
  .toolbar :global(.loading-status) {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm);
    color: var(--color-text-muted);
    font-size: var(--font-size-sm);
  }
</style>

