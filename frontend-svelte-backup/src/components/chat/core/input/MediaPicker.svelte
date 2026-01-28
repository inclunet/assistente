<script>
  import { createEventDispatcher } from 'svelte';
  
  // Props - Configuração
  export let allowImages = true;
  export let allowAudio = true;
  export let allowDocuments = true;
  export let allowScreenshot = false;
  export let allowWebcam = false;
  export let maxFiles = 5;
  export let currentCount = 0;
  export let disabled = false;
  
  // Props - Labels (i18n)
  export let labels = {};
  
  const dispatch = createEventDispatcher();
  
  let fileInputElement;
  let showMenu = false;
  
  $: canAddMore = currentCount < maxFiles;
  
  // Tipos de arquivo aceitos
  $: acceptedTypes = buildAcceptedTypes();
  
  function buildAcceptedTypes() {
    const types = [];
    if (allowImages) types.push('image/*');
    if (allowAudio) types.push('audio/*');
    if (allowDocuments) types.push('.pdf,.doc,.docx,.txt,.md,.csv,.xlsx,.xls');
    return types.join(',');
  }
  
  // === Menu Items ===
  
  $: menuItems = buildMenuItems();
  
  function buildMenuItems() {
    const items = [];
    
    if (allowImages || allowDocuments) {
      items.push({
        id: 'file',
        label: labels.uploadFile || 'Enviar arquivo',
        icon: '📎',
        action: () => openFilePicker()
      });
    }
    
    if (allowScreenshot) {
      items.push({
        id: 'screenshot',
        label: labels.captureScreen || 'Capturar tela',
        icon: '📸',
        action: () => dispatch('captureScreen')
      });
    }
    
    if (allowWebcam) {
      items.push({
        id: 'webcam',
        label: labels.captureWebcam || 'Capturar webcam',
        icon: '📷',
        action: () => dispatch('captureWebcam')
      });
    }
    
    if (allowAudio) {
      items.push({
        id: 'audio',
        label: labels.recordAudio || 'Gravar áudio',
        icon: '🎤',
        action: () => dispatch('recordAudio')
      });
    }
    
    return items;
  }
  
  // === Event Handlers ===
  
  function toggleMenu() {
    showMenu = !showMenu;
  }
  
  function closeMenu() {
    showMenu = false;
  }
  
  function handleMenuAction(item) {
    closeMenu();
    item.action();
  }
  
  function openFilePicker() {
    fileInputElement?.click();
  }
  
  function handleFileChange(event) {
    const files = event.target.files;
    if (!files || files.length === 0) return;
    
    dispatch('filesSelected', { files: Array.from(files) });
    
    // Limpa input
    if (fileInputElement) {
      fileInputElement.value = '';
    }
  }
  
  function handleKeyDown(event) {
    if (event.key === 'Escape' && showMenu) {
      event.preventDefault();
      closeMenu();
    }
  }
</script>

<div class="media-picker" class:disabled on:keydown={handleKeyDown}>
  <!-- Botão principal -->
  <button
    type="button"
    class="picker-btn"
    on:click={toggleMenu}
    disabled={disabled || !canAddMore}
    aria-haspopup="menu"
    aria-expanded={showMenu}
    aria-label={labels.addMedia || 'Anexar mídia'}
    title={labels.addMedia || 'Anexar mídia'}
  >
    📎
  </button>
  
  <!-- Menu dropdown -->
  {#if showMenu}
    <div class="picker-menu" role="menu">
      {#each menuItems as item (item.id)}
        <button
          type="button"
          class="menu-item"
          role="menuitem"
          on:click={() => handleMenuAction(item)}
        >
          <span class="menu-icon" aria-hidden="true">{item.icon}</span>
          <span class="menu-label">{item.label}</span>
        </button>
      {/each}
    </div>
    
    <!-- Backdrop para fechar menu -->
    <button 
      type="button"
      class="picker-backdrop" 
      on:click={closeMenu}
      aria-label="Fechar menu"
      tabindex="-1"
    ></button>
  {/if}
  
  <!-- Input de arquivo oculto -->
  <input
    bind:this={fileInputElement}
    type="file"
    multiple
    accept={acceptedTypes}
    class="file-input-hidden"
    on:change={handleFileChange}
  />
</div>

<style>
  .media-picker {
    position: relative;
    display: inline-block;
  }
  
  .media-picker.disabled {
    opacity: 0.5;
    pointer-events: none;
  }
  
  .picker-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    padding: 0;
    border: none;
    background: var(--chat-action-bg);
    border-radius: var(--chat-radius-sm);
    cursor: pointer;
    font-size: var(--chat-font-size-lg);
    transition: background-color var(--chat-transition-fast);
  }
  
  .picker-btn:hover:not(:disabled) {
    background: var(--chat-action-hover-bg);
  }
  
  .picker-btn:focus-visible {
    outline: 2px solid var(--chat-color-border-focus);
    outline-offset: 2px;
  }
  
  .picker-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .picker-menu {
    position: absolute;
    bottom: 100%;
    left: 0;
    min-width: 180px;
    margin-bottom: var(--chat-space-1);
    padding: var(--chat-space-1);
    background: var(--chat-color-bg);
    border: 1px solid var(--chat-color-border);
    border-radius: var(--chat-radius-md);
    box-shadow: var(--chat-shadow-lg);
    z-index: 100;
  }
  
  .menu-item {
    display: flex;
    align-items: center;
    gap: var(--chat-space-2);
    width: 100%;
    padding: var(--chat-space-2) var(--chat-space-3);
    border: none;
    background: transparent;
    color: var(--chat-color-text);
    text-align: left;
    font-size: var(--chat-font-size-sm);
    cursor: pointer;
    border-radius: var(--chat-radius-sm);
    transition: background-color var(--chat-transition-fast);
  }
  
  .menu-item:hover,
  .menu-item:focus {
    background: var(--chat-color-hover);
    outline: none;
  }
  
  .menu-icon {
    font-size: var(--chat-font-size-lg);
  }
  
  .menu-label {
    flex: 1;
  }
  
  .picker-backdrop {
    position: fixed;
    inset: 0;
    background: transparent;
    border: none;
    z-index: 99;
  }
  
  .file-input-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    border: 0;
  }
</style>

