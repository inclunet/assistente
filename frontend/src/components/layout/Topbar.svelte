<script>
  import { createEventDispatcher, tick, onMount, onDestroy } from 'svelte';
  
  export let currentPage = 'chat';
  export let hasApiKey = false;
  
  const dispatch = createEventDispatcher();
  
  let menuOpen = false;
  let menuElement;
  let menuButtonElement;
  let focusedIndex = -1;
  
  // Atalhos globais Alt+Key para navegação
  function handleGlobalKeyDown(event) {
    if (!event.altKey) return;
    
    const key = event.key.toLowerCase();
    
    // Alt+M: Menu
    if (key === 'm') {
      event.preventDefault();
      toggleMenu();
    }
    // Alt+1: Chat
    else if (key === '1') {
      event.preventDefault();
      navigate('chat');
    }
    // Alt+2: Histórico
    else if (key === '2' && hasApiKey) {
      event.preventDefault();
      navigate('history');
    }
    // Alt+3: FAQ
    else if (key === '3' && hasApiKey) {
      event.preventDefault();
      navigate('faq');
    }
    // Alt+4: Memórias
    else if (key === '4' && hasApiKey) {
      event.preventDefault();
      navigate('memories');
    }
    // Alt+5: Agentes
    else if (key === '5' && hasApiKey) {
      event.preventDefault();
      navigate('agents');
    }
    // Alt+6: Conexões OAuth
    else if (key === '6' && hasApiKey) {
      event.preventDefault();
      navigate('oauth');
    }
    // Alt+7 ou Alt+,: Configurações
    else if (key === '7' || key === ',') {
      event.preventDefault();
      navigate('settings');
    }
  }
  
  onMount(() => {
    window.addEventListener('keydown', handleGlobalKeyDown);
  });
  
  onDestroy(() => {
    window.removeEventListener('keydown', handleGlobalKeyDown);
  });
  
  const menuItems = [
    { id: 'chat', label: 'Chat', icon: '💬', shortcut: 'Alt+1', requiresAuth: false },
    { id: 'history', label: 'Histórico', icon: '📜', shortcut: 'Alt+2', requiresAuth: true },
    { id: 'faq', label: 'FAQ', icon: '❓', shortcut: 'Alt+3', requiresAuth: true },
    { id: 'memories', label: 'Memórias', icon: '🧠', shortcut: 'Alt+4', requiresAuth: true },
    { id: 'agents', label: 'Agentes', icon: '🤖', shortcut: 'Alt+5', requiresAuth: true },
    { id: 'oauth', label: 'Conexões', icon: '🔐', shortcut: 'Alt+6', requiresAuth: true },
    { id: 'settings', label: 'Configurações', icon: '⚙️', shortcut: 'Alt+7', requiresAuth: false }
  ];
  
  $: availableItems = menuItems.filter(item => !item.requiresAuth || hasApiKey);
  $: currentItem = menuItems.find(m => m.id === currentPage);
  
  async function toggleMenu() {
    menuOpen = !menuOpen;
    
    if (menuOpen) {
      focusedIndex = availableItems.findIndex(item => item.id === currentPage);
      if (focusedIndex === -1) focusedIndex = 0;
      
      await tick();
      focusItem(focusedIndex);
    } else {
      focusedIndex = -1;
    }
  }
  
  function closeMenu() {
    menuOpen = false;
    focusedIndex = -1;
    menuButtonElement?.focus();
  }
  
  function navigate(pageId) {
    dispatch('navigate', pageId);
    closeMenu();
  }
  
  function focusItem(index) {
    const items = menuElement?.querySelectorAll('[role="menuitem"]');
    if (items && items[index]) {
      items[index].focus();
    }
  }
  
  function handleMenuKeyDown(event) {
    if (!menuOpen) return;
    
    const itemCount = availableItems.length;
    
    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault();
        focusedIndex = (focusedIndex + 1) % itemCount;
        focusItem(focusedIndex);
        break;
        
      case 'ArrowUp':
        event.preventDefault();
        focusedIndex = focusedIndex <= 0 ? itemCount - 1 : focusedIndex - 1;
        focusItem(focusedIndex);
        break;
        
      case 'Home':
        event.preventDefault();
        focusedIndex = 0;
        focusItem(focusedIndex);
        break;
        
      case 'End':
        event.preventDefault();
        focusedIndex = itemCount - 1;
        focusItem(focusedIndex);
        break;
        
      case 'Enter':
      case ' ':
        event.preventDefault();
        if (focusedIndex >= 0 && availableItems[focusedIndex]) {
          navigate(availableItems[focusedIndex].id);
        }
        break;
        
      case 'Escape':
        event.preventDefault();
        closeMenu();
        break;
        
      case 'Tab':
        // Fecha menu ao sair com Tab
        closeMenu();
        break;
    }
  }
  
  function handleClickOutside(event) {
    if (menuOpen && menuElement && !menuElement.contains(event.target) && 
        menuButtonElement && !menuButtonElement.contains(event.target)) {
      closeMenu();
    }
  }
</script>

<svelte:window on:click={handleClickOutside} />

<header class="topbar">
  <div class="topbar-left">
    <div class="menu-wrapper">
      <button
        bind:this={menuButtonElement}
        class="menu-toggle"
        on:click={toggleMenu}
        aria-expanded={menuOpen}
        aria-haspopup="menu"
        aria-label="Menu de navegação (Alt+M)"
        title="Menu (Alt+M)"
      >
        <span class="menu-icon" aria-hidden="true">☰</span>
      </button>
      
      {#if menuOpen}
        <ul 
          bind:this={menuElement}
          class="nav-list" 
          role="menu" 
          aria-label="Navegação principal"
          on:keydown={handleMenuKeyDown}
        >
          {#each availableItems as item, index}
            {@const isActive = currentPage === item.id}
            <li role="none">
              <button
                role="menuitem"
                class="nav-item"
                class:active={isActive}
                aria-current={isActive ? 'page' : undefined}
                tabindex={focusedIndex === index ? 0 : -1}
                on:click={() => navigate(item.id)}
              >
                <span class="nav-icon" aria-hidden="true">{item.icon}</span>
                <span class="nav-label">{item.label}</span>
                <span class="nav-shortcut">{item.shortcut}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
    
    <h1 class="app-title">🤖 Assistente</h1>
  </div>
  
  <div class="topbar-right">
    {#if currentItem}
      <span class="current-page">
        <span aria-hidden="true">{currentItem.icon}</span>
        {currentItem.label}
      </span>
    {/if}
  </div>
</header>

<style>
  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 56px;
    padding: 0 var(--spacing-md, 16px);
    background: var(--color-bg-secondary, #1a1a1a);
    border-bottom: 1px solid var(--color-border, #404040);
  }
  
  .topbar-left {
    display: flex;
    align-items: center;
    gap: var(--spacing-md, 16px);
  }
  
  .topbar-right {
    display: flex;
    align-items: center;
  }
  
  .menu-wrapper {
    position: relative;
  }
  
  .menu-toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 44px;
    height: 44px;
    background: transparent;
    border: 1px solid transparent;
    border-radius: var(--border-radius, 8px);
    color: var(--color-text-primary, #e0e0e0);
    cursor: pointer;
    font-size: 1.4em;
    transition: all 0.15s;
  }
  
  .menu-toggle:hover {
    background: var(--color-bg-tertiary, #2d2d2d);
    border-color: var(--color-border, #404040);
  }
  
  .menu-toggle:focus {
    outline: 2px solid var(--color-accent, #58a6ff);
    outline-offset: 2px;
  }
  
  .app-title {
    margin: 0;
    font-size: var(--font-size-lg, 18px);
    font-weight: 600;
    color: var(--color-text-primary, #e0e0e0);
  }
  
  .nav-list {
    position: absolute;
    top: calc(100% + var(--spacing-xs, 4px));
    left: 0;
    list-style: none;
    margin: 0;
    padding: var(--spacing-xs, 4px);
    min-width: 220px;
    background: var(--color-bg-secondary, #1a1a1a);
    border: 1px solid var(--color-border, #404040);
    border-radius: var(--border-radius, 8px);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
    z-index: 1000;
  }
  
  .nav-item {
    display: flex;
    align-items: center;
    width: 100%;
    padding: var(--spacing-sm, 8px) var(--spacing-md, 12px);
    background: transparent;
    border: none;
    border-radius: var(--border-radius, 8px);
    color: var(--color-text-secondary, #aaa);
    cursor: pointer;
    text-align: left;
    transition: all 0.15s;
    font-size: var(--font-size-base, 14px);
    gap: var(--spacing-sm, 8px);
  }
  
  .nav-item:hover {
    background: var(--color-bg-tertiary, #2d2d2d);
    color: var(--color-text-primary, #e0e0e0);
  }
  
  .nav-item:focus {
    outline: 2px solid var(--color-accent, #58a6ff);
    outline-offset: -2px;
    background: var(--color-bg-tertiary, #2d2d2d);
    color: var(--color-text-primary, #e0e0e0);
  }
  
  .nav-item.active {
    background: rgba(88, 166, 255, 0.15);
    color: var(--color-accent, #58a6ff);
  }
  
  .nav-icon {
    font-size: 1.1em;
    width: 24px;
    text-align: center;
    flex-shrink: 0;
  }
  
  .nav-label {
    flex: 1;
  }
  
  .nav-shortcut {
    font-size: var(--font-size-xs, 11px);
    color: var(--color-text-muted, #666);
    opacity: 0.7;
  }
  
  .current-page {
    display: flex;
    align-items: center;
    gap: var(--spacing-xs, 4px);
    padding: var(--spacing-xs, 4px) var(--spacing-sm, 8px);
    background: var(--color-bg-tertiary, #2d2d2d);
    border-radius: var(--border-radius, 8px);
    color: var(--color-text-secondary, #aaa);
    font-size: var(--font-size-sm, 13px);
  }
</style>

