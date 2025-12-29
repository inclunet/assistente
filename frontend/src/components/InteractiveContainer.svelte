<script>
  /**
   * InteractiveContainer - Container com ActionBar e ContextMenu integrados
   * 
   * Combina ActionBar (hover) + ContextMenu (click direito) de forma unificada.
   * Use para mensagens, cards, itens de lista, etc.
   * 
   * @example
   * <InteractiveContainer
   *   actions={[{ id: 'copy', icon: '📋', label: 'Copiar' }]}
   *   contextMenuItems={[{ id: 'copy', label: 'Copiar', icon: '📋', shortcut: 'Ctrl+C' }]}
   *   on:action={handleAction}
   *   on:select={handleContextAction}
   * >
   *   <div class="message">Conteúdo aqui</div>
   * </InteractiveContainer>
   */
  
  import { createEventDispatcher } from 'svelte';
  import ContextMenu from './ContextMenu.svelte';
  import ActionBar from './ActionBar.svelte';
  
  const dispatch = createEventDispatcher();
  
  // Props - Actions (ActionBar)
  /** @type {import('./ActionBar.svelte').Action[]} */
  export let actions = [];
  export let actionBarPosition = 'top-right';
  
  // Props - Context Menu
  /** @type {import('./ContextMenu.svelte').MenuItem[]} */
  export let contextMenuItems = [];
  export let contextMenuLabel = 'Menu de contexto';
  
  // Props - Comportamento
  export let disabled = false;
  export let showActionsOnHover = true;
  export let showActionsOnFocus = true;
  export let actionBarDelay = 150; // ms antes de mostrar (evita flickering)
  
  // Estado
  let isHovering = false;
  let isFocused = false;
  let menuVisible = false;
  let menuX = 0;
  let menuY = 0;
  let actionBarComponent;
  let hoverTimeout = null;
  
  // ActionBar visível?
  $: showActionBar = !disabled && (
    (showActionsOnHover && isHovering) || 
    (showActionsOnFocus && isFocused) ||
    menuVisible
  );
  
  function handleMouseEnter() {
    if (actionBarDelay > 0) {
      hoverTimeout = setTimeout(() => {
        isHovering = true;
      }, actionBarDelay);
    } else {
      isHovering = true;
    }
  }
  
  function handleMouseLeave() {
    if (hoverTimeout) {
      clearTimeout(hoverTimeout);
      hoverTimeout = null;
    }
    isHovering = false;
  }
  
  function handleFocusIn() {
    isFocused = true;
  }
  
  function handleFocusOut(event) {
    // Verifica se o foco saiu do container
    if (!event.currentTarget.contains(event.relatedTarget)) {
      isFocused = false;
    }
  }
  
  function handleContextMenu(event) {
    if (disabled || contextMenuItems.length === 0) return;
    
    event.preventDefault();
    event.stopPropagation();
    
    menuX = event.clientX;
    menuY = event.clientY;
    menuVisible = true;
  }
  
  function handleKeyDown(event) {
    if (disabled) return;
    
    // Shift+F10 ou tecla Menu abre o menu de contexto
    if (event.key === 'ContextMenu' || (event.shiftKey && event.key === 'F10')) {
      event.preventDefault();
      event.stopPropagation();
      
      const rect = event.currentTarget.getBoundingClientRect();
      menuX = rect.left + rect.width / 2;
      menuY = rect.top + rect.height / 2;
      menuVisible = true;
    }
  }
  
  function handleAction(event) {
    dispatch('action', event.detail);
  }
  
  function handleContextSelect(event) {
    dispatch('select', event.detail);
    // Também dispara 'action' para unificar handlers se desejado
    dispatch('action', event.detail);
  }
  
  function handleMenuClose() {
    menuVisible = false;
  }
</script>

<div
  class="interactive-container"
  class:disabled
  on:mouseenter={handleMouseEnter}
  on:mouseleave={handleMouseLeave}
  on:focusin={handleFocusIn}
  on:focusout={handleFocusOut}
  on:contextmenu={handleContextMenu}
  on:keydown={handleKeyDown}
>
  {#if actions.length > 0}
    <ActionBar
      bind:this={actionBarComponent}
      {actions}
      position={actionBarPosition}
      forceVisible={showActionBar}
      on:action={handleAction}
    />
  {/if}
  
  <slot />
</div>

{#if contextMenuItems.length > 0}
  <ContextMenu
    items={contextMenuItems}
    ariaLabel={contextMenuLabel}
    bind:visible={menuVisible}
    x={menuX}
    y={menuY}
    on:select={handleContextSelect}
    on:close={handleMenuClose}
  />
{/if}

<style>
  .interactive-container {
    position: relative;
  }
  
  .interactive-container.disabled {
    pointer-events: none;
    opacity: 0.6;
  }
</style>



