<script>
  import { createEventDispatcher, tick, onMount, onDestroy } from 'svelte';
  
  /**
   * @typedef {Object} Tab
   * @property {string} id - Identificador único da aba
   * @property {string} label - Texto da aba
   * @property {string} [icon] - Emoji ou caractere para ícone
   * @property {boolean} [closable] - Se a aba pode ser fechada (default: true quando closableTabs=true)
   * @property {boolean} [disabled] - Se a aba está desabilitada
   * @property {*} [data] - Dados customizados associados à aba
   */
  
  /** @type {Tab[]} */
  export let tabs = [];
  
  /** @type {string} ID da aba ativa */
  export let activeTab = '';
  
  /** @type {'horizontal' | 'vertical'} Orientação das abas */
  export let orientation = 'horizontal';
  
  /** @type {boolean} Se as abas podem ser fechadas */
  export let closableTabs = false;
  
  /** @type {boolean} Se mostra botão para adicionar nova aba */
  export let addable = false;
  
  /** @type {string} Label do botão de adicionar */
  export let addLabel = 'Nova aba';
  
  /** @type {boolean} Se permite reordenar abas via drag */
  export let reorderable = false;
  
  /** @type {'start' | 'center' | 'end' | 'stretch'} Alinhamento das abas */
  export let align = 'start';
  
  /** @type {string} Tamanho das abas: 'sm' | 'md' | 'lg' */
  export let size = 'md';
  
  /** @type {string} Variante visual: 'default' | 'pills' | 'underline' */
  export let variant = 'default';
  
/** @type {string} Label acessível para a lista de abas (opcional, o role já identifica) */
  export let ariaLabel = '';
  
  /** @type {boolean} Se mantém o conteúdo das abas montado mesmo quando não ativas (evita remontagem) */
  export let keepMounted = false;

  const dispatch = createEventDispatcher();
  
  // Referências para elementos
  let containerElement;
  let tabListElement;
  let tabElements = {};
  
  // Live region para anúncios de screen readers
  let liveAnnouncement = '';
  
  // ID único para ARIA
  const panelId = `tabpanel-${Math.random().toString(36).substr(2, 9)}`;
  
  /**
   * Handler global para atalhos de teclado
   * Funciona de qualquer lugar dentro do componente
   */
  function handleGlobalKeyDown(event) {
    // Ctrl+PageDown/PageUp para alternar abas (fallback, já que Ctrl+Tab é bloqueado pelo browser)
    const isTabSwitch = event.ctrlKey && (event.key === 'PageDown' || event.key === 'PageUp');
    
    if (isTabSwitch) {
      event.preventDefault();
      event.stopPropagation();
      
      const enabledTabs = tabs.filter(t => !t.disabled);
      if (enabledTabs.length <= 1) return;
      
      const currentEnabledIndex = enabledTabs.findIndex(t => t.id === activeTab);
      
      let nextTab;
      const goBack = event.key === 'PageUp';
      
      if (goBack) {
        // Ctrl+PageUp: aba anterior
        nextTab = enabledTabs[(currentEnabledIndex - 1 + enabledTabs.length) % enabledTabs.length];
      } else {
        // Ctrl+PageDown: próxima aba
        nextTab = enabledTabs[(currentEnabledIndex + 1) % enabledTabs.length];
      }
      
      if (nextTab) {
        selectTab(nextTab, false); // Não mover foco para a aba
        // Dispara evento para que o pai possa mover o foco para o conteúdo
        dispatch('tabSwitch', { tabId: nextTab.id, tab: nextTab, source: 'keyboard' });
      }
      return;
    }
    
    // Ctrl+T: Nova aba (dispara evento, não executa)
    if (event.ctrlKey && event.key.toLowerCase() === 't') {
      if (addable) {
        event.preventDefault();
        event.stopPropagation();
        dispatch('add');
      }
      return;
    }
    
    // Ctrl+W ou Ctrl+F4: Fechar aba atual (dispara evento, não executa)
    if (event.ctrlKey && (event.key.toLowerCase() === 'w' || event.key === 'F4')) {
      const currentTabObj = tabs.find(t => t.id === activeTab);
      const isClosable = closableTabs && currentTabObj?.closable !== false;
      
      if (currentTabObj && isClosable) {
        event.preventDefault();
        event.stopPropagation();
        
        // Dispara evento de requisição de fechamento
        // O componente pai decide se fecha ou não (ex: confirmar se há alterações)
        dispatch('closeRequest', {
          tabId: currentTabObj.id,
          tab: currentTabObj,
          index: tabs.findIndex(t => t.id === activeTab)
        });
      }
      return;
    }
  }
  
  onMount(() => {
    // Adiciona listener no window com capture=true para pegar Ctrl+Tab antes de outros handlers
    window.addEventListener('keydown', handleGlobalKeyDown, true);
  });
  
  onDestroy(() => {
    window.removeEventListener('keydown', handleGlobalKeyDown, true);
  });
  
  // Rastreia aba anterior para detectar mudanças
  let previousActiveTab = '';
  let isInitialized = false;
  
  // Garante que sempre há uma aba ativa se existirem abas
  $: if (tabs.length > 0 && !tabs.find(t => t.id === activeTab)) {
    const firstEnabled = tabs.find(t => !t.disabled);
    if (firstEnabled) {
      activeTab = firstEnabled.id;
    }
  }
  
  // Aba atual selecionada
  $: currentTab = tabs.find(t => t.id === activeTab);
  
  // Índice da aba atual para navegação
  $: currentIndex = tabs.findIndex(t => t.id === activeTab);
  
  /**
   * Detecta mudanças em activeTab e dispara evento
   * Funciona para qualquer mudança: seleção manual, fechamento, binding externo
   */
  $: if (activeTab && activeTab !== previousActiveTab) {
    const changedTab = tabs.find(t => t.id === activeTab);
    
    // Dispara evento change (exceto na primeira inicialização)
    if (isInitialized && changedTab) {
      dispatch('change', {
        tabId: activeTab,
        tab: changedTab,
        previousTabId: previousActiveTab,
        source: 'reactive' // Indica que veio de mudança reativa
      });
      
      // Nota: Não anunciamos seleção de aba via aria-live.
      // O NVDA já lê a aba quando ela recebe foco (via aria-label)
      // e anuncia o estado "selecionada" (via aria-selected).
    }
    
    previousActiveTab = activeTab;
    isInitialized = true;
  }
  
  // IDs ARIA
  function getTabId(tabId) {
    return `${panelId}-tab-${tabId}`;
  }
  
  function getPanelContentId(tabId) {
    return `${panelId}-content-${tabId}`;
  }
  
  /**
   * Retorna a posição da aba para aria-posinset (1-indexed)
   */
  function getTabPosition(tabId) {
    return tabs.findIndex(t => t.id === tabId) + 1;
  }
  
  /**
   * Gera o label acessível para uma aba
   * Nota: Posição é fornecida pelo NVDA via aria-posinset/aria-setsize
   * Nota: Dica de fechamento usa aria-describedby (lido sob demanda)
   */
  function getTabAriaLabel(tab) {
    let label = tab.label;
    
    // Indica se está desabilitada
    if (tab.disabled) {
      label += ', desabilitada';
    }
    
    return label;
  }
  
  /**
   * ID do elemento de descrição para abas fecháveis
   */
  const closeHintId = `${panelId}-close-hint`;
  
  /**
   * Anuncia mensagem para screen readers via live region
   */
  function announce(message) {
    // Limpa e define nova mensagem para garantir que seja anunciada
    liveAnnouncement = '';
    tick().then(() => {
      liveAnnouncement = message;
    });
  }
  
  /**
   * Seleciona uma aba
   * @param {Tab} tab - A aba a selecionar
   * @param {boolean} [focusTab=true] - Se deve mover o foco para a aba
   */
  function selectTab(tab, focusTab = true) {
    if (tab.disabled) return;
    
    // Não faz nada se já é a aba ativa
    if (activeTab === tab.id) return;
    
    // Atualiza activeTab - o statement reativo cuidará de:
    // - Disparar evento 'change'
    // - Anunciar para screen readers
    activeTab = tab.id;
    
    // Foca na aba selecionada (apenas se solicitado)
    if (focusTab) {
      tick().then(() => {
        tabElements[tab.id]?.focus();
      });
    }
  }
  
  /**
   * Fecha uma aba
   */
  function closeTab(tab, event) {
    event?.stopPropagation();
    event?.preventDefault();
    
    const tabIndex = tabs.findIndex(t => t.id === tab.id);
    const remainingCount = tabs.length - 1;
    
    // Anuncia o fechamento para screen readers
    announce(`Aba ${tab.label} fechada. ${remainingCount} ${remainingCount === 1 ? 'aba restante' : 'abas restantes'}.`);
    
    dispatch('close', { 
      tabId: tab.id, 
      tab,
      index: tabIndex
    });
    
    // Se fechou a aba ativa, seleciona a próxima ou anterior
    if (activeTab === tab.id) {
      const remaining = tabs.filter(t => t.id !== tab.id);
      if (remaining.length > 0) {
        // Tenta selecionar a próxima, ou a anterior se era a última
        const nextIndex = Math.min(tabIndex, remaining.length - 1);
        const nextTab = remaining[nextIndex];
        if (nextTab && !nextTab.disabled) {
          activeTab = nextTab.id;
          // Foca na nova aba ativa após o fechamento
          tick().then(() => {
            tabElements[nextTab.id]?.focus();
          });
        }
      }
    }
  }
  
  /**
   * Adiciona uma nova aba
   */
  function addTab() {
    // O anúncio será feito pelo componente pai após adicionar a aba
    // já que precisamos do nome da nova aba
    dispatch('add');
  }
  
  /**
   * Método público para anunciar criação de nova aba
   * Pode ser chamado pelo componente pai após adicionar
   * @param {string} tabLabel - Nome da aba criada
   */
  export function announceNewTab(tabLabel) {
    announce(`Nova aba ${tabLabel} criada. ${tabs.length} ${tabs.length === 1 ? 'aba total' : 'abas totais'}.`);
  }
  
  /**
   * Navegação por teclado - WAI-ARIA Tabs Pattern
   */
  function handleKeyDown(event, tab) {
    const isVertical = orientation === 'vertical';
    const enabledTabs = tabs.filter(t => !t.disabled);
    const currentEnabledIndex = enabledTabs.findIndex(t => t.id === activeTab);
    
    let nextTab = null;
    let handled = false;
    
    switch (event.key) {
      case 'ArrowRight':
        if (!isVertical) {
          nextTab = enabledTabs[(currentEnabledIndex + 1) % enabledTabs.length];
          handled = true;
        }
        break;
        
      case 'ArrowLeft':
        if (!isVertical) {
          nextTab = enabledTabs[(currentEnabledIndex - 1 + enabledTabs.length) % enabledTabs.length];
          handled = true;
        }
        break;
        
      case 'ArrowDown':
        if (isVertical) {
          nextTab = enabledTabs[(currentEnabledIndex + 1) % enabledTabs.length];
          handled = true;
        }
        break;
        
      case 'ArrowUp':
        if (isVertical) {
          nextTab = enabledTabs[(currentEnabledIndex - 1 + enabledTabs.length) % enabledTabs.length];
          handled = true;
        }
        break;
        
      case 'Home':
        nextTab = enabledTabs[0];
        handled = true;
        break;
        
      case 'End':
        nextTab = enabledTabs[enabledTabs.length - 1];
        handled = true;
        break;
        
      case 'Delete':
        // Delete fecha a aba se for fechável
        if (closableTabs && tab.closable !== false) {
          closeTab(tab, event);
          handled = true;
        }
        break;
        
      case 'Enter':
      case ' ':
        // Ativa a aba focada
        selectTab(tab);
        handled = true;
        break;
    }
    
    if (handled) {
      event.preventDefault();
    }
    
    if (nextTab) {
      selectTab(nextTab);
    }
  }
  
  /**
   * Drag and drop para reordenar
   */
  let draggedTab = null;
  let dragOverTab = null;
  
  function handleDragStart(event, tab) {
    if (!reorderable) return;
    
    draggedTab = tab;
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', tab.id);
    
    // Adiciona classe para feedback visual
    event.target.classList.add('dragging');
  }
  
  function handleDragEnd(event) {
    draggedTab = null;
    dragOverTab = null;
    event.target.classList.remove('dragging');
  }
  
  function handleDragOver(event, tab) {
    if (!reorderable || !draggedTab || draggedTab.id === tab.id) return;
    
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
    dragOverTab = tab;
  }
  
  function handleDragLeave() {
    dragOverTab = null;
  }
  
  function handleDrop(event, targetTab) {
    if (!reorderable || !draggedTab || draggedTab.id === targetTab.id) return;
    
    event.preventDefault();
    
    const fromIndex = tabs.findIndex(t => t.id === draggedTab.id);
    const toIndex = tabs.findIndex(t => t.id === targetTab.id);
    
    dispatch('reorder', {
      fromIndex,
      toIndex,
      tab: draggedTab
    });
    
    draggedTab = null;
    dragOverTab = null;
  }
  
  // Classes dinâmicas
  $: containerClass = [
    'tab-panel',
    `tab-panel--${orientation}`,
    `tab-panel--${variant}`,
    `tab-panel--${size}`,
    `tab-panel--align-${align}`
  ].join(' ');
  
  $: tabListClass = [
    'tab-list',
    `tab-list--${orientation}`
  ].join(' ');
</script>

<div bind:this={containerElement} class={containerClass}>
  <!-- Live region para anúncios de screen readers -->
  <div 
    class="sr-only" 
    role="status" 
    aria-live="polite" 
    aria-atomic="true"
  >
    {liveAnnouncement}
  </div>
  
  <!-- Dica de fechamento (lida sob demanda via aria-describedby) -->
  {#if closableTabs}
    <div id={closeHintId} class="sr-only">
      Pressione Delete para fechar
    </div>
  {/if}
  
  <!-- Toolbar contendo lista de abas e botão adicionar -->
  <div class="tab-toolbar">
    <!-- Lista de abas -->
    <div 
      bind:this={tabListElement}
      class={tabListClass}
      role="tablist"
      aria-label={ariaLabel || undefined}
      aria-orientation={orientation}
    >
      {#each tabs as tab, index (tab.id)}
        {@const isActive = activeTab === tab.id}
        {@const isClosable = closableTabs && tab.closable !== false}
        {@const isDragOver = dragOverTab?.id === tab.id}
        {@const position = index + 1}
        {@const total = tabs.length}
        
        <button
          bind:this={tabElements[tab.id]}
          id={getTabId(tab.id)}
          class="tab"
          class:tab--active={isActive}
          class:tab--disabled={tab.disabled}
          class:tab--closable={isClosable}
          class:tab--drag-over={isDragOver}
          role="tab"
          type="button"
          tabindex={isActive ? 0 : -1}
          aria-selected={isActive ? 'true' : 'false'}
          aria-controls={getPanelContentId(tab.id)}
          aria-posinset={position}
          aria-setsize={total}
          aria-label={getTabAriaLabel(tab)}
          aria-describedby={isClosable ? closeHintId : undefined}
          disabled={tab.disabled || undefined}
          draggable={reorderable}
          on:click={() => selectTab(tab)}
          on:keydown={(e) => handleKeyDown(e, tab)}
          on:dragstart={(e) => handleDragStart(e, tab)}
          on:dragend={handleDragEnd}
          on:dragover={(e) => handleDragOver(e, tab)}
          on:dragleave={handleDragLeave}
          on:drop={(e) => handleDrop(e, tab)}
        >
          {#if tab.icon}
            <span class="tab__icon" aria-hidden="true">{tab.icon}</span>
          {/if}
          
          <span class="tab__label" aria-hidden="true">{tab.label}</span>
          
          {#if isClosable}
            <span 
              class="tab__close"
              role="button"
              aria-hidden="true"
              on:click={(e) => closeTab(tab, e)}
            >×</span>
          {/if}
        </button>
      {/each}
    </div>
    
    <!-- Botão adicionar (fora do tablist para semântica correta) -->
    {#if addable}
      <button
        class="tab-add-btn"
        type="button"
        aria-label={addLabel}
        on:click={addTab}
      >
        <span aria-hidden="true">+</span>
      </button>
    {/if}
  </div>
  
  <!-- Conteúdo da aba ativa -->
  <div class="tab-content">
    {#each tabs as tab (tab.id)}
      {@const isActive = activeTab === tab.id}
      <div
        id={getPanelContentId(tab.id)}
        class="tab-pane"
        class:tab-pane--active={isActive}
        role="tabpanel"
        aria-labelledby={getTabId(tab.id)}
        tabindex="-1"
        hidden={!isActive}
        aria-hidden={!isActive ? 'true' : undefined}
      >
        {#if keepMounted || isActive}
          <slot name="tab-content" {tab} tabId={tab.id} />
        {/if}
      </div>
    {/each}
    
    <!-- Slot padrão para conteúdo simples baseado na aba ativa -->
    {#if currentTab}
      <slot tab={currentTab} tabId={currentTab.id} />
    {/if}
  </div>
</div>

<style>
  /* ===== Screen Reader Only ===== */
  .sr-only {
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
  
  /* ===== Container ===== */
  .tab-panel {
    display: flex;
    height: 100%;
    width: 100%;
    overflow: hidden;
  }
  
  .tab-panel--horizontal {
    flex-direction: column;
  }
  
  .tab-panel--vertical {
    flex-direction: row;
  }
  
  /* ===== Tab Toolbar ===== */
  .tab-toolbar {
    display: flex;
    align-items: stretch;
    background: var(--color-bg-secondary, #161b22);
    border: 1px solid var(--color-border, #30363d);
    flex-shrink: 0;
  }
  
  .tab-panel--horizontal .tab-toolbar {
    flex-direction: row;
    border-radius: var(--border-radius, 8px) var(--border-radius, 8px) 0 0;
    border-bottom: none;
  }
  
  .tab-panel--vertical .tab-toolbar {
    flex-direction: column;
    border-radius: var(--border-radius, 8px) 0 0 var(--border-radius, 8px);
    border-right: none;
  }
  
  /* ===== Tab List ===== */
  .tab-list {
    display: flex;
    gap: var(--spacing-xs, 4px);
    flex: 1;
    overflow-x: auto;
    overflow-y: hidden;
    scrollbar-width: thin;
  }
  
  .tab-list--horizontal {
    flex-direction: row;
    padding: var(--spacing-xs, 4px) var(--spacing-xs, 4px) 0;
  }
  
  .tab-list--vertical {
    flex-direction: column;
    padding: var(--spacing-xs, 4px) 0 var(--spacing-xs, 4px) var(--spacing-xs, 4px);
    min-width: 160px;
    max-width: 240px;
    overflow-x: hidden;
    overflow-y: auto;
  }
  
  /* Alinhamentos */
  .tab-panel--align-start .tab-list--horizontal {
    justify-content: flex-start;
  }
  
  .tab-panel--align-center .tab-list--horizontal {
    justify-content: center;
  }
  
  .tab-panel--align-end .tab-list--horizontal {
    justify-content: flex-end;
  }
  
  .tab-panel--align-stretch .tab-list--horizontal .tab {
    flex: 1;
  }
  
  /* ===== Tab Button ===== */
  .tab {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm, 8px);
    padding: var(--spacing-sm, 8px) var(--spacing-md, 16px);
    background: transparent;
    border: none;
    border-radius: var(--border-radius, 8px) var(--border-radius, 8px) 0 0;
    color: var(--color-text-muted, #8b949e);
    font-family: inherit;
    font-size: var(--font-size-sm, 0.875rem);
    font-weight: 500;
    cursor: pointer;
    transition: all var(--transition-fast, 150ms ease);
    white-space: nowrap;
    min-height: 40px;
    position: relative;
  }
  
  /* Vertical tabs */
  .tab-list--vertical .tab {
    border-radius: var(--border-radius, 8px) 0 0 var(--border-radius, 8px);
    justify-content: flex-start;
    width: 100%;
  }
  
  .tab:hover:not(:disabled) {
    background: var(--color-bg-tertiary, #21262d);
    color: var(--color-text-secondary, #c9d1d9);
  }
  
  .tab:focus-visible {
    outline: 2px solid var(--color-accent, #58a6ff);
    outline-offset: -2px;
    z-index: 1;
  }
  
  .tab--active {
    background: var(--color-bg-primary, #0d1117);
    color: var(--color-text-primary, #f0f6fc);
    border: 1px solid var(--color-border, #30363d);
  }
  
  .tab-list--horizontal .tab--active {
    border-bottom-color: var(--color-bg-primary, #0d1117);
    margin-bottom: -1px;
  }
  
  .tab-list--vertical .tab--active {
    border-right-color: var(--color-bg-primary, #0d1117);
    margin-right: -1px;
  }
  
  .tab--disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .tab--drag-over {
    background: var(--color-accent-dark, #1f6feb);
    opacity: 0.7;
  }
  
  .tab.dragging {
    opacity: 0.5;
  }
  
  /* ===== Tab Icon ===== */
  .tab__icon {
    font-size: 1.1em;
    line-height: 1;
    flex-shrink: 0;
  }
  
  /* ===== Tab Label ===== */
  .tab__label {
    overflow: hidden;
    text-overflow: ellipsis;
  }
  
  /* ===== Close Button (inside tab) ===== */
  .tab__close {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    margin-left: var(--spacing-xs, 4px);
    background: transparent;
    border: none;
    border-radius: 4px;
    color: inherit;
    font-size: 16px;
    line-height: 1;
    cursor: pointer;
    opacity: 0.6;
    transition: all var(--transition-fast, 150ms ease);
    flex-shrink: 0;
  }
  
  .tab__close:hover {
    opacity: 1;
    background: var(--color-error, #f85149);
    color: white;
  }
  
  /* ===== Add Button (outside tablist) ===== */
  .tab-add-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    min-width: 40px;
    min-height: 40px;
    padding: var(--spacing-sm, 8px);
    background: transparent;
    border: none;
    border-left: 1px solid var(--color-border, #30363d);
    color: var(--color-text-muted, #8b949e);
    font-size: 1.25em;
    font-weight: bold;
    cursor: pointer;
    transition: all var(--transition-fast, 150ms ease);
  }
  
  .tab-panel--vertical .tab-add-btn {
    border-left: none;
    border-top: 1px solid var(--color-border, #30363d);
  }
  
  .tab-add-btn:hover {
    color: var(--color-accent, #58a6ff);
    background: var(--color-bg-tertiary, #21262d);
  }
  
  .tab-add-btn:focus-visible {
    outline: 2px solid var(--color-accent, #58a6ff);
    outline-offset: -2px;
    z-index: 1;
  }
  
  /* ===== Tab Content ===== */
  .tab-content {
    flex: 1;
    background: var(--color-bg-primary, #0d1117);
    border: 1px solid var(--color-border, #30363d);
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  
  .tab-panel--horizontal .tab-content {
    border-radius: 0 0 var(--border-radius, 8px) var(--border-radius, 8px);
    border-top: none;
  }
  
  .tab-panel--vertical .tab-content {
    border-radius: 0 var(--border-radius, 8px) var(--border-radius, 8px) 0;
    border-left: none;
  }
  
  /* ===== Tab Pane ===== */
  .tab-pane {
    display: none;
    height: 100%;
    overflow: auto;
  }
  
  .tab-pane--active {
    display: flex;
    flex-direction: column;
  }
  
  /* ===== Variantes ===== */
  
  /* Pills */
  .tab-panel--pills .tab-list {
    background: transparent;
    border: none;
    padding: var(--spacing-sm, 8px);
    gap: var(--spacing-sm, 8px);
  }
  
  .tab-panel--pills .tab {
    border-radius: var(--border-radius-lg, 12px);
    border: none;
    background: var(--color-bg-tertiary, #21262d);
  }
  
  .tab-panel--pills .tab--active {
    background: var(--color-accent-dark, #1f6feb);
    color: white;
  }
  
  .tab-panel--pills .tab-content {
    border-radius: var(--border-radius, 8px);
    border: 1px solid var(--color-border, #30363d);
  }
  
  /* Underline */
  .tab-panel--underline .tab-list {
    background: transparent;
    border: none;
    border-bottom: 1px solid var(--color-border, #30363d);
    padding: 0;
    gap: 0;
  }
  
  .tab-panel--underline .tab {
    border: none;
    border-radius: 0;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
    padding: var(--spacing-sm, 8px) var(--spacing-lg, 24px);
  }
  
  .tab-panel--underline .tab--active {
    background: transparent;
    border-bottom-color: var(--color-accent, #58a6ff);
    color: var(--color-accent, #58a6ff);
  }
  
  .tab-panel--underline .tab-content {
    border: none;
    border-radius: 0;
  }
  
  /* ===== Tamanhos ===== */
  
  /* Small */
  .tab-panel--sm .tab {
    padding: var(--spacing-xs, 4px) var(--spacing-sm, 8px);
    font-size: var(--font-size-sm, 0.75rem);
    min-height: 32px;
  }
  
  .tab-panel--sm .tab__close {
    width: 16px;
    height: 16px;
    font-size: 12px;
  }
  
  /* Large */
  .tab-panel--lg .tab {
    padding: var(--spacing-md, 16px) var(--spacing-lg, 24px);
    font-size: var(--font-size-base, 1rem);
    min-height: 48px;
  }
  
  
  /* ===== Scrollbar ===== */
  .tab-list::-webkit-scrollbar {
    width: 4px;
    height: 4px;
  }
  
  .tab-list::-webkit-scrollbar-track {
    background: transparent;
  }
  
  .tab-list::-webkit-scrollbar-thumb {
    background: var(--color-border, #30363d);
    border-radius: 2px;
  }
  
  /* ===== Reduced Motion ===== */
  @media (prefers-reduced-motion: reduce) {
    .tab,
    .tab__close {
      transition: none;
    }
  }
</style>

