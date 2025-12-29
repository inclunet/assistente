<script>
  import { createEventDispatcher, onMount, tick } from 'svelte';
  import { GetModels } from '../../wailsjs/go/main/App.js';

  export let value = '';
  export let label = 'Modelo';
  export let placeholder = 'Selecione um modelo';
  export let helpText = '';
  export let disabled = false;
  export let allowCustom = true;

  const dispatch = createEventDispatcher();
  const uniqueId = 'model-' + Math.random().toString(36).substr(2, 9);

  let models = [];
  let loading = false;
  let error = '';
  let isOpen = false;
  let filter = '';
  let highlightedIndex = 0;
  let inputEl;
  let containerEl;

  $: filteredModels = filter 
    ? models.filter(m => m.toLowerCase().includes(filter.toLowerCase()))
    : models;

  // Reset index quando filtro muda
  $: {
    filter;
    highlightedIndex = 0;
  }

  onMount(async () => {
    await loadModels();
  });

  async function loadModels() {
    loading = true;
    error = '';
    try {
      models = await GetModels() || [];
    } catch (e) {
      error = 'Erro ao carregar modelos';
      console.error(e);
    } finally {
      loading = false;
    }
  }

  async function open() {
    if (disabled || loading) return;
    isOpen = true;
    filter = '';
    highlightedIndex = value ? Math.max(0, models.indexOf(value)) : 0;
    
    await tick();
    inputEl?.focus();
  }

  function close() {
    isOpen = false;
    filter = '';
  }

  function select(model) {
    value = model;
    dispatch('change', model);
    close();
  }

  function handleInputKeydown(e) {
    const len = filteredModels.length;
    const pageSize = 10; // Quantidade de itens para Page Up/Down
    
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        e.stopPropagation();
        if (len > 0) {
          highlightedIndex = Math.min(highlightedIndex + 1, len - 1);
          scrollToIndex(highlightedIndex);
        }
        break;
        
      case 'ArrowUp':
        e.preventDefault();
        e.stopPropagation();
        if (len > 0) {
          highlightedIndex = Math.max(highlightedIndex - 1, 0);
          scrollToIndex(highlightedIndex);
        }
        break;
      
      case 'Home':
        e.preventDefault();
        e.stopPropagation();
        if (len > 0) {
          highlightedIndex = 0;
          scrollToIndex(highlightedIndex);
        }
        break;
        
      case 'End':
        e.preventDefault();
        e.stopPropagation();
        if (len > 0) {
          highlightedIndex = len - 1;
          scrollToIndex(highlightedIndex);
        }
        break;
        
      case 'PageDown':
        e.preventDefault();
        e.stopPropagation();
        if (len > 0) {
          highlightedIndex = Math.min(highlightedIndex + pageSize, len - 1);
          scrollToIndex(highlightedIndex);
        }
        break;
        
      case 'PageUp':
        e.preventDefault();
        e.stopPropagation();
        if (len > 0) {
          highlightedIndex = Math.max(highlightedIndex - pageSize, 0);
          scrollToIndex(highlightedIndex);
        }
        break;
        
      case 'Enter':
        e.preventDefault();
        e.stopPropagation();
        if (filteredModels[highlightedIndex]) {
          select(filteredModels[highlightedIndex]);
        } else if (allowCustom && filter.trim()) {
          select(filter.trim());
        }
        break;
        
      case 'Escape':
        e.preventDefault();
        e.stopPropagation();
        close();
        break;
    }
  }

  function scrollToIndex(index) {
    tick().then(() => {
      const item = containerEl?.querySelector(`[data-index="${index}"]`);
      item?.scrollIntoView({ block: 'nearest' });
    });
  }

  function handleBlur(e) {
    // Só fecha se o foco sair do container
    setTimeout(() => {
      if (!containerEl?.contains(document.activeElement)) {
        close();
      }
    }, 100);
  }

  function handleItemClick(model) {
    select(model);
  }
</script>

<div class="model-selector" bind:this={containerEl}>
  <label for={uniqueId}>{label}</label>
  
  {#if isOpen}
    <div class="dropdown-container">
      <input
        bind:this={inputEl}
        id={uniqueId}
        type="text"
        bind:value={filter}
        on:keydown={handleInputKeydown}
        on:blur={handleBlur}
        placeholder="Digite para filtrar..."
        autocomplete="off"
        role="combobox"
        aria-expanded="true"
        aria-controls="{uniqueId}-listbox"
        aria-activedescendant={filteredModels[highlightedIndex] ? `${uniqueId}-option-${highlightedIndex}` : ''}
      />
      
      <ul 
        id="{uniqueId}-listbox" 
        role="listbox"
        aria-label="Modelos disponíveis"
      >
        {#if loading}
          <li class="status">Carregando...</li>
        {:else if error}
          <li class="status error">{error}</li>
        {:else if filteredModels.length === 0}
          {#if allowCustom && filter.trim()}
            <li 
              class="option custom"
              class:highlighted={true}
              on:mousedown|preventDefault={() => select(filter.trim())}
            >
              Usar: "{filter.trim()}"
            </li>
          {:else}
            <li class="status">Nenhum modelo encontrado</li>
          {/if}
        {:else}
          {#each filteredModels as model, i}
            <li
              id="{uniqueId}-option-{i}"
              data-index={i}
              class="option"
              class:highlighted={i === highlightedIndex}
              class:selected={model === value}
              role="option"
              aria-selected={model === value}
              on:mousedown|preventDefault={() => handleItemClick(model)}
              on:mouseenter={() => highlightedIndex = i}
            >
              {#if model === value}<span class="check">✓</span>{/if}
              {model}
            </li>
          {/each}
        {/if}
      </ul>
    </div>
  {:else}
    <button
      type="button"
      class="trigger"
      on:click={open}
      {disabled}
      aria-haspopup="listbox"
      aria-expanded="false"
    >
      <span class="icon">🤖</span>
      <span class="value">{value || placeholder}</span>
      <span class="arrow">▼</span>
    </button>
  {/if}
  
  {#if helpText}
    <small class="help">{helpText}</small>
  {/if}
</div>

<style>
  .model-selector {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  label {
    font-weight: 500;
    color: var(--color-text-primary, #e0e0e0);
  }

  .trigger {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem 1rem;
    min-height: 44px;
    background: var(--color-bg-secondary, #2d2d2d);
    border: 1px solid var(--color-border, #444);
    border-radius: 6px;
    color: var(--color-text-primary, #e0e0e0);
    font-size: 0.95rem;
    cursor: pointer;
    text-align: left;
  }

  .trigger:hover:not(:disabled) {
    border-color: var(--color-accent, #4a9eff);
  }

  .trigger:focus {
    outline: 2px solid var(--color-accent, #4a9eff);
    outline-offset: 2px;
  }

  .trigger:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .trigger .value {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .trigger .arrow {
    font-size: 0.7rem;
    color: var(--color-text-secondary, #888);
  }

  .dropdown-container {
    position: relative;
  }

  input {
    width: 100%;
    padding: 0.75rem 1rem;
    min-height: 44px;
    background: var(--color-bg-secondary, #2d2d2d);
    border: 2px solid var(--color-accent, #4a9eff);
    border-radius: 6px 6px 0 0;
    color: var(--color-text-primary, #e0e0e0);
    font-size: 0.95rem;
    box-sizing: border-box;
  }

  input:focus {
    outline: none;
  }

  ul {
    position: absolute;
    top: 100%;
    left: 0;
    right: 0;
    max-height: 200px;
    overflow-y: auto;
    margin: 0;
    padding: 0;
    list-style: none;
    background: var(--color-bg-secondary, #2d2d2d);
    border: 2px solid var(--color-accent, #4a9eff);
    border-top: 1px solid var(--color-border, #444);
    border-radius: 0 0 6px 6px;
    z-index: 1000;
  }

  .option {
    padding: 0.6rem 1rem;
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .option:hover,
  .option.highlighted {
    background: var(--color-bg-hover, #3a3a3a);
  }

  .option.selected {
    font-weight: 600;
  }

  .option .check {
    color: var(--color-accent, #4a9eff);
  }

  .option.custom {
    color: var(--color-accent, #4a9eff);
    font-style: italic;
  }

  .status {
    padding: 0.75rem 1rem;
    color: var(--color-text-secondary, #888);
    text-align: center;
    font-style: italic;
  }

  .status.error {
    color: var(--color-error, #ff6b6b);
  }

  .help {
    color: var(--color-text-secondary, #888);
    font-size: 0.85rem;
  }
</style>
