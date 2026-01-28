<script>
  import { createEventDispatcher, tick } from 'svelte';
  
  const dispatch = createEventDispatcher();
  
  // Props
  export let icon = '🔧';
  export let label = 'Selecionar';
  export let items = [];              // [{value, label, sublabel?}]
  export let selected = '';           // value selecionado
  export let placeholder = 'Filtrar...';
  export let disabled = false;
  export let maxWidth = '180px';
  
  // Estado
  let isOpen = false;
  let filter = '';
  let highlightIndex = 0;
  let inputElement;
  let buttonElement;
  
  // Filtra items
  $: filteredItems = items.filter(item => 
    item.label.toLowerCase().includes(filter.toLowerCase()) ||
    (item.sublabel && item.sublabel.toLowerCase().includes(filter.toLowerCase()))
  );
  
  // Label do item selecionado
  $: selectedLabel = items.find(i => i.value === selected)?.label || label;
  $: displayLabel = selectedLabel.length > 20 
    ? selectedLabel.substring(0, 17) + '...' 
    : selectedLabel;
  
  export function open() {
    if (disabled) return;
    
    isOpen = true;
    filter = '';
    
    // Inicia no item atual ou no primeiro
    const currentIdx = filteredItems.findIndex(i => i.value === selected);
    highlightIndex = currentIdx >= 0 ? currentIdx : 0;
    
    tick().then(() => {
      inputElement?.focus();
      announce();
    });
    
    dispatch('open');
  }
  
  export function close() {
    isOpen = false;
    filter = '';
    highlightIndex = 0;
    
    tick().then(() => {
      buttonElement?.focus();
    });
    
    dispatch('close');
  }
  
  function select(item) {
    selected = item.value;
    dispatch('select', { value: item.value, item });
    close();
  }
  
  function handleKeyDown(event) {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      event.stopPropagation();
      if (filteredItems.length > 0) {
        highlightIndex = Math.min(highlightIndex + 1, filteredItems.length - 1);
        announce();
        scrollToOption();
      }
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      event.stopPropagation();
      if (highlightIndex > 0) {
        highlightIndex = highlightIndex - 1;
        announce();
        scrollToOption();
      }
    } else if (event.key === 'Enter') {
      event.preventDefault();
      event.stopPropagation();
      if (highlightIndex >= 0 && filteredItems[highlightIndex]) {
        select(filteredItems[highlightIndex]);
      } else if (filteredItems.length === 1) {
        select(filteredItems[0]);
      }
    } else if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      close();
    } else if (event.key === 'Tab') {
      close();
    }
  }
  
  function scrollToOption() {
    tick().then(() => {
      const option = document.getElementById(`${uniqueId}-option-${highlightIndex}`);
      option?.scrollIntoView({ block: 'nearest' });
    });
  }
  
  function announce() {
    if (highlightIndex >= 0 && filteredItems[highlightIndex]) {
      const item = filteredItems[highlightIndex];
      const sublabel = item.sublabel ? `, ${item.sublabel}` : '';
      dispatch('announce', { 
        message: `${item.label}${sublabel}, ${highlightIndex + 1} de ${filteredItems.length}` 
      });
    }
  }
  
  function handleBlur() {
    // Delay para permitir clique nas opções
    setTimeout(() => {
      if (isOpen) {
        close();
      }
    }, 150);
  }
  
  // ID único para acessibilidade
  const uniqueId = `combobox-${Math.random().toString(36).substr(2, 9)}`;
</script>

<div class="combobox-picker" style="--max-width: {maxWidth}">
  {#if !isOpen}
    <button 
      bind:this={buttonElement}
      class="picker-button"
      on:click={open}
      {disabled}
      aria-haspopup="listbox"
      aria-expanded={isOpen}
      aria-label="{label}: {selectedLabel}"
      title="{label}: {selectedLabel}"
    >
      <!-- Texto visual escondido do leitor (aria-label já fornece o contexto) -->
      <span class="picker-icon" aria-hidden="true">{icon}</span>
      <span class="picker-label" aria-hidden="true">{displayLabel}</span>
      <span class="picker-arrow" aria-hidden="true">▼</span>
    </button>
  {:else}
    <div class="picker-dropdown">
      <input 
        bind:this={inputElement}
        type="text"
        bind:value={filter}
        on:keydown={handleKeyDown}
        on:blur={handleBlur}
        {placeholder}
        role="combobox"
        aria-expanded="true"
        aria-controls="{uniqueId}-listbox"
        aria-activedescendant={highlightIndex >= 0 ? `${uniqueId}-option-${highlightIndex}` : ''}
        aria-autocomplete="list"
      />
      <ul 
        id="{uniqueId}-listbox" 
        role="listbox" 
        aria-label="{label} disponíveis"
      >
        {#each filteredItems as item, i}
          <li 
            id="{uniqueId}-option-{i}"
            role="option"
            aria-selected={i === highlightIndex}
            class:highlighted={i === highlightIndex}
            class:selected={item.value === selected}
            on:click={() => select(item)}
            on:mouseenter={() => highlightIndex = i}
          >
            <span class="option-label">{item.label}</span>
            {#if item.sublabel}
              <span class="option-sublabel">{item.sublabel}</span>
            {/if}
          </li>
        {/each}
        {#if filteredItems.length === 0}
          <li class="no-results" role="option" aria-disabled="true">
            Nenhum resultado
          </li>
        {/if}
      </ul>
    </div>
  {/if}
</div>

<style>
  .combobox-picker {
    position: relative;
  }

  .picker-button {
    display: flex;
    align-items: center;
    gap: var(--spacing-xs);
    padding: var(--spacing-xs) var(--spacing-sm);
    max-width: var(--max-width, 180px);
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    color: var(--color-text-primary);
    font-size: var(--font-size-sm);
    cursor: pointer;
    transition: all 0.15s;
    min-height: 36px;
  }

  .picker-button:hover:not(:disabled) {
    background: var(--color-bg-primary);
    border-color: var(--color-accent);
  }

  .picker-button:focus {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
  }

  .picker-button:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .picker-icon {
    flex-shrink: 0;
  }

  .picker-label {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    text-align: left;
  }

  .picker-arrow {
    flex-shrink: 0;
    font-size: 0.7em;
    opacity: 0.7;
  }

  .picker-dropdown {
    position: absolute;
    top: 0;
    left: 0;
    z-index: 100;
    min-width: 100%;
    width: max-content;
    max-width: 320px;
    max-height: 300px;
    background-color: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    display: flex;
    flex-direction: column;
  }

  .picker-dropdown input {
    margin: var(--spacing-sm);
    padding: var(--spacing-sm);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    background-color: var(--color-bg-input);
    color: var(--color-text-primary);
    font-size: var(--font-size-sm);
  }

  .picker-dropdown input:focus {
    border-color: var(--color-accent);
    outline: none;
  }

  .picker-dropdown ul {
    list-style: none;
    margin: 0;
    padding: 0;
    overflow-y: auto;
    max-height: 220px;
  }

  .picker-dropdown li {
    padding: var(--spacing-sm) var(--spacing-md);
    cursor: pointer;
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: var(--spacing-sm);
  }

  .picker-dropdown li:hover,
  .picker-dropdown li.highlighted {
    background-color: var(--color-bg-tertiary);
  }

  .picker-dropdown li.selected {
    background-color: rgba(88, 166, 255, 0.15);
  }

  .picker-dropdown li.highlighted.selected {
    background-color: var(--color-accent);
    color: var(--color-bg-primary);
  }

  .picker-dropdown li.no-results {
    color: var(--color-text-muted);
    cursor: default;
    justify-content: center;
  }

  .option-label {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .option-sublabel {
    flex-shrink: 0;
    font-size: var(--font-size-xs);
    color: var(--color-text-muted);
    background: var(--color-bg-tertiary);
    padding: 2px 6px;
    border-radius: var(--border-radius);
  }

  .highlighted .option-sublabel {
    background: var(--color-bg-secondary);
  }

  .highlighted.selected .option-sublabel {
    background: rgba(255, 255, 255, 0.2);
    color: inherit;
  }
</style>

