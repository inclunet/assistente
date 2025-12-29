<script context="module">
  /**
   * @typedef {Object} MenuItem
   * @property {string} id - Identificador único
   * @property {string} [label] - Texto exibido
   * @property {string} [icon] - Emoji ou ícone
   * @property {string} [shortcut] - Atalho de teclado (ex: "Ctrl+C")
   * @property {boolean} [disabled] - Item desabilitado
   * @property {boolean} [separator] - É um separador (ignora outros campos)
   * @property {boolean} [danger] - Estilo de ação perigosa (vermelho)
   * @property {MenuItem[]} [submenu] - Submenu (opcional)
   */
  
  // Noop para fechar todos os menus abertos
  let closeAllMenus = () => {};
  
  export function registerCloseAll(fn) {
    const prev = closeAllMenus;
    closeAllMenus = () => {
      prev();
      fn();
    };
    return () => {
      closeAllMenus = prev;
    };
  }
</script>

<script>
  import { onMount, onDestroy, createEventDispatcher, tick } from 'svelte';
  
  const dispatch = createEventDispatcher();
  
  // Props
  /** @type {MenuItem[]} */
  export let items = [];
  export let x = 0;
  export let y = 0;
  export let visible = false;
  export let ariaLabel = 'Menu de contexto';
  
  // Estado interno
  let menuElement;
  let highlightIndex = -1;
  let adjustedX = x;
  let adjustedY = y;
  let unregister;
  let previousFocusElement = null; // Elemento que tinha foco antes de abrir o menu
  
  // Estado de submenu
  let openSubmenuIndex = -1;
  let submenuHighlightIndex = -1;
  
  // Items filtrados (sem separadores para navegação)
  $: navigableItems = items.filter(item => !item.separator && !item.disabled);
  $: navigableIndices = items.reduce((acc, item, idx) => {
    if (!item.separator && !item.disabled) acc.push(idx);
    return acc;
  }, []);
  
  // Atualiza posição quando x/y mudam (para casos de bind:visible)
  $: if (visible) {
    adjustPosition(x, y);
  }
  
  onMount(() => {
    unregister = registerCloseAll(() => {
      if (visible) close();
    });
  });
  
  onDestroy(() => {
    if (unregister) unregister();
  });
  
  async function adjustPosition(posX, posY) {
    await tick();
    
    if (!menuElement) {
      adjustedX = posX;
      adjustedY = posY;
      return;
    }
    
    const rect = menuElement.getBoundingClientRect();
    const viewportWidth = window.innerWidth;
    const viewportHeight = window.innerHeight;
    
    // Ajusta X se sair da tela
    if (posX + rect.width > viewportWidth - 8) {
      adjustedX = Math.max(8, viewportWidth - rect.width - 8);
    } else {
      adjustedX = posX;
    }
    
    // Ajusta Y se sair da tela
    if (posY + rect.height > viewportHeight - 8) {
      adjustedY = Math.max(8, viewportHeight - rect.height - 8);
    } else {
      adjustedY = posY;
    }
  }
  
  export function open(posX, posY) {
    // Salva o elemento que tinha foco para restaurar depois
    previousFocusElement = document.activeElement;
    
    // Fecha outros menus abertos
    closeAllMenus();
    
    x = posX;
    y = posY;
    visible = true;
    
    // Seleciona o primeiro item navegável
    highlightIndex = navigableIndices.length > 0 ? navigableIndices[0] : -1;
    
    tick().then(() => {
      if (menuElement) {
        menuElement.focus();
        adjustPosition(posX, posY);
      }
    });
  }
  
  export function close() {
    if (!visible) return;
    visible = false;
    highlightIndex = -1;
    openSubmenuIndex = -1;
    submenuHighlightIndex = -1;
    
    // Restaura o foco para o elemento anterior
    if (previousFocusElement && typeof previousFocusElement.focus === 'function') {
      tick().then(() => {
        previousFocusElement.focus();
        previousFocusElement = null;
      });
    }
    
    dispatch('close');
  }
  
  function select(item) {
    if (item.disabled) return;
    
    dispatch('select', { id: item.id, item });
    close();
  }
  
  function openSubmenu(index) {
    const item = items[index];
    if (!item?.submenu?.length) return;
    
    openSubmenuIndex = index;
    submenuHighlightIndex = 0;
  }
  
  function closeSubmenu() {
    openSubmenuIndex = -1;
    submenuHighlightIndex = -1;
  }
  
  function toggleSubmenu(index) {
    if (openSubmenuIndex === index) {
      closeSubmenu();
    } else {
      openSubmenu(index);
    }
  }
  
  function handleKeyDown(event) {
    if (!visible) return;
    
    // Se submenu está aberto, navega nele
    if (openSubmenuIndex >= 0) {
      const submenu = items[openSubmenuIndex]?.submenu || [];
      const navigableSub = submenu.filter(s => !s.separator && !s.disabled);
      
      if (navigableSub.length === 0) return;
      
      switch (event.key) {
        case 'ArrowDown':
          event.preventDefault();
          event.stopPropagation();
          submenuHighlightIndex = submenuHighlightIndex < 0 
            ? 0 
            : (submenuHighlightIndex + 1) % navigableSub.length;
          return;
          
        case 'ArrowUp':
          event.preventDefault();
          event.stopPropagation();
          submenuHighlightIndex = submenuHighlightIndex <= 0 
            ? navigableSub.length - 1 
            : submenuHighlightIndex - 1;
          return;
          
        case 'ArrowLeft':
          event.preventDefault();
          event.stopPropagation();
          closeSubmenu();
          return;
          
        case 'Escape':
          event.preventDefault();
          event.stopPropagation();
          closeSubmenu();
          return;
          
        case 'Enter':
        case ' ':
          event.preventDefault();
          event.stopPropagation();
          if (submenuHighlightIndex >= 0 && navigableSub[submenuHighlightIndex]) {
            select(navigableSub[submenuHighlightIndex]);
          }
          return;
      }
    }
    
    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault();
        event.stopPropagation();
        moveHighlight(1);
        break;
        
      case 'ArrowUp':
        event.preventDefault();
        event.stopPropagation();
        moveHighlight(-1);
        break;
        
      case 'ArrowRight':
        event.preventDefault();
        event.stopPropagation();
        // Abre submenu se existir
        if (highlightIndex >= 0 && items[highlightIndex]?.submenu?.length) {
          openSubmenu(highlightIndex);
        }
        break;
        
      case 'Enter':
      case ' ':
        event.preventDefault();
        event.stopPropagation();
        if (highlightIndex >= 0 && items[highlightIndex]) {
          const item = items[highlightIndex];
          if (item.submenu?.length) {
            openSubmenu(highlightIndex);
          } else {
            select(item);
          }
        }
        break;
        
      case 'Escape':
        event.preventDefault();
        event.stopPropagation();
        close();
        break;
        
      case 'Tab':
        event.preventDefault();
        close();
        break;
        
      case 'Home':
        event.preventDefault();
        event.stopPropagation();
        if (navigableIndices.length > 0) {
          highlightIndex = navigableIndices[0];
          closeSubmenu();
          scrollToItem();
        }
        break;
        
      case 'End':
        event.preventDefault();
        event.stopPropagation();
        if (navigableIndices.length > 0) {
          highlightIndex = navigableIndices[navigableIndices.length - 1];
          closeSubmenu();
          scrollToItem();
        }
        break;
        
      default:
        // Type-ahead: primeira letra
        if (event.key.length === 1 && !event.ctrlKey && !event.altKey) {
          const char = event.key.toLowerCase();
          const matchIdx = items.findIndex((item, idx) => 
            !item.separator && 
            !item.disabled && 
            item.label?.toLowerCase().startsWith(char) &&
            idx > highlightIndex
          );
          if (matchIdx >= 0) {
            highlightIndex = matchIdx;
            closeSubmenu();
            scrollToItem();
          } else {
            // Volta ao início
            const firstMatch = items.findIndex(item => 
              !item.separator && 
              !item.disabled && 
              item.label?.toLowerCase().startsWith(char)
            );
            if (firstMatch >= 0) {
              highlightIndex = firstMatch;
              closeSubmenu();
              scrollToItem();
            }
          }
        }
    }
  }
  
  function moveHighlight(direction) {
    if (navigableIndices.length === 0) return;
    
    const currentNavIdx = navigableIndices.indexOf(highlightIndex);
    let newNavIdx;
    
    if (currentNavIdx < 0) {
      // Nenhum selecionado ainda
      newNavIdx = direction > 0 ? 0 : navigableIndices.length - 1;
    } else {
      newNavIdx = currentNavIdx + direction;
      if (newNavIdx < 0) newNavIdx = navigableIndices.length - 1;
      if (newNavIdx >= navigableIndices.length) newNavIdx = 0;
    }
    
    highlightIndex = navigableIndices[newNavIdx];
    scrollToItem();
  }
  
  function scrollToItem() {
    tick().then(() => {
      const item = menuElement?.querySelector(`[data-index="${highlightIndex}"]`);
      item?.scrollIntoView({ block: 'nearest' });
    });
  }
  
  function handleClickOutside(event) {
    if (visible && menuElement && !menuElement.contains(event.target)) {
      close();
    }
  }
  
  function handleBlur(event) {
    // Delay para permitir clique nos itens e navegação
    setTimeout(() => {
      if (visible && menuElement && !menuElement.contains(document.activeElement)) {
        close();
      }
    }, 150);
  }
  
  // Unique ID para ARIA
  const menuId = `context-menu-${Math.random().toString(36).substr(2, 9)}`;
</script>

<svelte:window on:click={handleClickOutside} on:contextmenu={handleClickOutside} />

{#if visible}
  <div
    bind:this={menuElement}
    class="context-menu"
    role="menu"
    aria-label={ariaLabel}
    aria-activedescendant={highlightIndex >= 0 ? `${menuId}-item-${highlightIndex}` : undefined}
    tabindex="0"
    id={menuId}
    style="left: {adjustedX}px; top: {adjustedY}px;"
    on:keydown={handleKeyDown}
    on:blur={handleBlur}
    on:contextmenu|preventDefault|stopPropagation
  >
    {#each items as item, index}
      {#if item.separator}
        <div class="context-menu__separator" role="separator" aria-hidden="true"></div>
      {:else}
        {@const itemIndex = navigableItems.indexOf(item)}
        {@const hasSubmenu = item.submenu && item.submenu.length > 0}
        <div class="context-menu__item-wrapper" class:has-submenu={hasSubmenu}>
          <button
            id="{menuId}-item-{index}"
            class="context-menu__item"
            class:highlighted={index === highlightIndex}
            class:disabled={item.disabled}
            class:danger={item.danger}
            class:has-submenu={hasSubmenu}
            role="menuitem"
            tabindex="-1"
            aria-disabled={item.disabled ? 'true' : undefined}
            aria-haspopup={hasSubmenu ? 'menu' : undefined}
            aria-expanded={hasSubmenu && openSubmenuIndex === index ? 'true' : undefined}
            data-index={index}
            on:click={() => hasSubmenu ? toggleSubmenu(index) : select(item)}
            on:mouseenter={() => {
              if (!item.disabled) {
                highlightIndex = index;
                if (hasSubmenu) openSubmenu(index);
                else closeSubmenu();
              }
            }}
          >
            {#if item.icon}
              <span class="context-menu__icon" aria-hidden="true">{item.icon}</span>
            {/if}
            <span class="context-menu__label">{item.label}</span>
            {#if item.shortcut}
              <span class="context-menu__shortcut" aria-hidden="true">{item.shortcut}</span>
            {/if}
            {#if hasSubmenu}
              <span class="context-menu__arrow" aria-hidden="true">▶</span>
            {/if}
          </button>
          
          {#if hasSubmenu && openSubmenuIndex === index}
            <div 
              class="context-menu context-menu--submenu"
              role="menu"
              aria-label={item.label}
            >
              {#each item.submenu as subitem, subindex}
                {#if subitem.separator}
                  <div class="context-menu__separator" role="separator" aria-hidden="true"></div>
                {:else}
                  {@const subNavigable = item.submenu.filter(s => !s.separator && !s.disabled)}
                  {@const subItemIndex = subNavigable.indexOf(subitem)}
                  <button
                    id="{menuId}-sub-{index}-{subindex}"
                    class="context-menu__item"
                    class:highlighted={subItemIndex === submenuHighlightIndex}
                    class:disabled={subitem.disabled}
                    class:danger={subitem.danger}
                    role="menuitem"
                    tabindex="-1"
                    aria-disabled={subitem.disabled ? 'true' : undefined}
                    on:click={() => select(subitem)}
                    on:mouseenter={() => !subitem.disabled && (submenuHighlightIndex = subItemIndex)}
                  >
                    {#if subitem.icon}
                      <span class="context-menu__icon" aria-hidden="true">{subitem.icon}</span>
                    {/if}
                    <span class="context-menu__label">{subitem.label}</span>
                    {#if subitem.shortcut}
                      <span class="context-menu__shortcut" aria-hidden="true">{subitem.shortcut}</span>
                    {/if}
                  </button>
                {/if}
              {/each}
            </div>
          {/if}
        </div>
      {/if}
    {/each}
  </div>
{/if}

<!-- Anúncio para leitores de tela -->
<div 
  class="visually-hidden"
  role="status"
  aria-live="polite"
  aria-atomic="true"
>
  {#if visible}
    {#if openSubmenuIndex >= 0}
      <!-- Submenu aberto: anuncia item do submenu -->
      {@const submenu = items[openSubmenuIndex]?.submenu || []}
      {@const navigableSub = submenu.filter(s => !s.separator)}
      {#if submenuHighlightIndex >= 0 && navigableSub[submenuHighlightIndex]}
        {navigableSub[submenuHighlightIndex].label}
      {/if}
    {:else if highlightIndex >= 0 && items[highlightIndex] && !items[highlightIndex].separator}
      <!-- Menu principal: anuncia item atual -->
      {items[highlightIndex].label}{items[highlightIndex].submenu ? ', submenu' : ''}
    {/if}
  {/if}
</div>

<style>
  .context-menu {
    position: fixed;
    z-index: 9999;
    min-width: 160px;
    max-width: 280px;
    max-height: 400px;
    overflow-y: auto;
    background: var(--color-bg-secondary, #1e1e1e);
    border: 1px solid var(--color-border, #3d3d3d);
    border-radius: var(--border-radius, 6px);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.05);
    padding: var(--spacing-xs, 4px) 0;
    outline: none;
    animation: contextMenuFadeIn 0.1s ease-out;
  }
  
  @keyframes contextMenuFadeIn {
    from {
      opacity: 0;
      transform: scale(0.95);
    }
    to {
      opacity: 1;
      transform: scale(1);
    }
  }
  
  .context-menu__item {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm, 8px);
    width: 100%;
    padding: var(--spacing-sm, 8px) var(--spacing-md, 12px);
    border: none;
    background: transparent;
    color: var(--color-text-primary, #e6e6e6);
    font-size: var(--font-size-sm, 13px);
    font-family: inherit;
    text-align: left;
    cursor: pointer;
    transition: background-color 0.1s;
  }
  
  .context-menu__item:hover:not(.disabled),
  .context-menu__item.highlighted:not(.disabled) {
    background: var(--color-bg-tertiary, #2d2d2d);
  }
  
  .context-menu__item:focus {
    outline: none;
  }
  
  .context-menu__item.disabled {
    color: var(--color-text-muted, #6e7681);
    cursor: not-allowed;
  }
  
  .context-menu__item.danger {
    color: var(--color-error, #f85149);
  }
  
  .context-menu__item.danger:hover:not(.disabled),
  .context-menu__item.danger.highlighted:not(.disabled) {
    background: rgba(248, 81, 73, 0.15);
  }
  
  .context-menu__icon {
    flex-shrink: 0;
    width: 18px;
    text-align: center;
  }
  
  .context-menu__label {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .context-menu__shortcut {
    flex-shrink: 0;
    font-size: var(--font-size-xs, 11px);
    color: var(--color-text-muted, #6e7681);
    background: var(--color-bg-primary, #121212);
    padding: 2px 6px;
    border-radius: 4px;
    margin-left: var(--spacing-md, 12px);
  }
  
  .context-menu__separator {
    height: 1px;
    margin: var(--spacing-xs, 4px) 0;
    background: var(--color-border, #3d3d3d);
  }
  
  /* Wrapper para items com submenu */
  .context-menu__item-wrapper {
    position: relative;
  }
  
  /* Seta indicando submenu */
  .context-menu__arrow {
    flex-shrink: 0;
    font-size: 0.7em;
    color: var(--color-text-muted, #6e7681);
    margin-left: auto;
    padding-left: var(--spacing-sm, 8px);
  }
  
  /* Submenu */
  .context-menu--submenu {
    position: absolute;
    left: 100%;
    top: 0;
    margin-left: -2px;
  }
  
  /* Item com submenu aberto */
  .context-menu__item.has-submenu.highlighted {
    background: var(--color-bg-tertiary, #2d2d2d);
  }
  
  /* Reduz movimento se preferido */
  @media (prefers-reduced-motion: reduce) {
    .context-menu {
      animation: none;
    }
  }
  
  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>

