<script>
  import { createEventDispatcher, onMount, onDestroy } from 'svelte';
  import ContextMenu from './ContextMenu.svelte';
  
  const dispatch = createEventDispatcher();
  
  // Props
  /** @type {import('./ContextMenu.svelte').MenuItem[]} */
  export let items = [];
  export let disabled = false;
  export let ariaLabel = 'Menu de contexto';
  
  // Estado
  let menuComponent;
  let triggerElement;
  
  /**
   * Abre o menu em uma posição específica
   * @param {number} x 
   * @param {number} y 
   */
  export function openAt(x, y) {
    if (disabled || !menuComponent) return;
    menuComponent.open(x, y);
  }
  
  /**
   * Fecha o menu
   */
  export function close() {
    if (menuComponent) {
      menuComponent.close();
    }
  }
  
  function handleContextMenu(event) {
    if (disabled) return;
    
    // Bloqueia o menu de contexto nativo
    event.preventDefault();
    event.stopPropagation();
    
    openAt(event.clientX, event.clientY);
  }
  
  function handleKeyDown(event) {
    if (disabled) return;
    
    // Shift+F10 ou tecla Menu/Applications abre o menu de contexto
    const isContextMenuKey = event.key === 'ContextMenu' || event.code === 'ContextMenu';
    const isShiftF10 = event.shiftKey && (event.key === 'F10' || event.code === 'F10');
    
    if (isContextMenuKey || isShiftF10) {
      event.preventDefault();
      event.stopPropagation();
      
      // Posiciona perto do elemento focado
      const target = event.target;
      const rect = target.getBoundingClientRect();
      openAt(rect.left + rect.width / 2, rect.top);
    }
  }
  
  function handleSelect(event) {
    dispatch('select', event.detail);
  }
  
  function handleClose() {
    dispatch('close');
  }
  
  // Captura contextmenu no modo capture para garantir bloqueio do menu nativo
  function captureContextMenu(event) {
    if (disabled || !triggerElement) return;
    
    if (triggerElement.contains(event.target)) {
      event.preventDefault();
      event.stopPropagation();
      openAt(event.clientX, event.clientY);
    }
  }
  
  onMount(() => {
    // Usa capture: true para interceptar antes do menu nativo
    window.addEventListener('contextmenu', captureContextMenu, true);
  });
  
  onDestroy(() => {
    window.removeEventListener('contextmenu', captureContextMenu, true);
  });
</script>

<!-- 
  Wrapper transparente que adiciona menu de contexto
  Bloqueia o menu nativo e abre o nosso customizado
-->
<div 
  bind:this={triggerElement}
  class="context-menu-trigger"
  on:keydown={handleKeyDown}
>
  <slot />
</div>

<ContextMenu
  bind:this={menuComponent}
  {items}
  {ariaLabel}
  on:select={handleSelect}
  on:close={handleClose}
/>

<style>
  .context-menu-trigger {
    display: block;
    width: 100%;
  }
</style>
