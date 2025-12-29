<script context="module">
  /**
   * @typedef {Object} Action
   * @property {string} id - Identificador único
   * @property {string} icon - Emoji ou ícone
   * @property {string} label - Label para aria-label e tooltip
   * @property {boolean} [disabled] - Desabilitado
   * @property {boolean} [hidden] - Oculto
   */
</script>

<script>
  import { createEventDispatcher } from 'svelte';
  
  const dispatch = createEventDispatcher();
  
  // Props
  /** @type {Action[]} */
  export let actions = [];
  
  /** Posição da barra em relação ao container pai */
  export let position = 'top-right'; // 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right'
  
  /** Mostrar ao hover do elemento pai */
  export let showOnHover = true;
  
  /** Mostrar ao focus do elemento pai */
  export let showOnFocus = true;
  
  /** Forçar visibilidade (override de hover/focus) */
  export let forceVisible = false;
  
  /** Delay em ms antes de mostrar (evita flickering) */
  export let showDelay = 0;
  
  // Estado
  let isVisible = false;
  let focusedIndex = -1;
  let showTimeout = null;
  let barElement;
  
  // Actions visíveis
  $: visibleActions = actions.filter(a => !a.hidden);
  
  // Classes de posição
  $: positionClasses = {
    'top-left': 'action-bar--top-left',
    'top-right': 'action-bar--top-right',
    'bottom-left': 'action-bar--bottom-left',
    'bottom-right': 'action-bar--bottom-right'
  }[position] || 'action-bar--top-right';
  
  export function show() {
    if (showDelay > 0) {
      showTimeout = setTimeout(() => {
        isVisible = true;
      }, showDelay);
    } else {
      isVisible = true;
    }
  }
  
  export function hide() {
    if (showTimeout) {
      clearTimeout(showTimeout);
      showTimeout = null;
    }
    isVisible = false;
    focusedIndex = -1;
  }
  
  export function focus() {
    if (visibleActions.length > 0) {
      focusedIndex = 0;
      barElement?.querySelector('button')?.focus();
    }
  }
  
  function handleAction(action) {
    if (action.disabled) return;
    dispatch('action', { id: action.id, action });
  }
  
  function handleKeyDown(event) {
    const enabledActions = visibleActions.filter(a => !a.disabled);
    if (enabledActions.length === 0) return;
    
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        event.preventDefault();
        event.stopPropagation();
        focusedIndex = (focusedIndex + 1) % visibleActions.length;
        focusButton(focusedIndex);
        break;
        
      case 'ArrowLeft':
      case 'ArrowUp':
        event.preventDefault();
        event.stopPropagation();
        focusedIndex = focusedIndex <= 0 ? visibleActions.length - 1 : focusedIndex - 1;
        focusButton(focusedIndex);
        break;
        
      case 'Home':
        event.preventDefault();
        event.stopPropagation();
        focusedIndex = 0;
        focusButton(0);
        break;
        
      case 'End':
        event.preventDefault();
        event.stopPropagation();
        focusedIndex = visibleActions.length - 1;
        focusButton(focusedIndex);
        break;
    }
  }
  
  function focusButton(index) {
    const buttons = barElement?.querySelectorAll('button');
    if (buttons && buttons[index]) {
      buttons[index].focus();
    }
  }
  
  function handleFocus(index) {
    focusedIndex = index;
  }
</script>

{#if visibleActions.length > 0 && (forceVisible || isVisible)}
  <div
    bind:this={barElement}
    class="action-bar {positionClasses}"
    class:visible={forceVisible || isVisible}
    role="toolbar"
    aria-label="Ações disponíveis"
    on:keydown={handleKeyDown}
  >
    {#each visibleActions as action, index}
      <button
        class="action-bar__button"
        class:disabled={action.disabled}
        aria-label={action.label}
        title={action.label}
        disabled={action.disabled}
        tabindex={index === focusedIndex ? 0 : -1}
        on:click={() => handleAction(action)}
        on:focus={() => handleFocus(index)}
      >
        <span class="action-bar__icon" aria-hidden="true">{action.icon}</span>
      </button>
    {/each}
  </div>
{/if}

<style>
  .action-bar {
    position: absolute;
    display: flex;
    gap: 2px;
    padding: 2px;
    background: var(--color-bg-secondary, #1e1e1e);
    border: 1px solid var(--color-border, #3d3d3d);
    border-radius: var(--border-radius, 6px);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
    z-index: 10;
    opacity: 0;
    transform: translateY(-4px);
    transition: opacity 0.15s ease, transform 0.15s ease;
    pointer-events: none;
  }
  
  .action-bar.visible {
    opacity: 1;
    transform: translateY(0);
    pointer-events: auto;
  }
  
  /* Posições */
  .action-bar--top-right {
    top: var(--spacing-xs, 4px);
    right: var(--spacing-xs, 4px);
  }
  
  .action-bar--top-left {
    top: var(--spacing-xs, 4px);
    left: var(--spacing-xs, 4px);
  }
  
  .action-bar--bottom-right {
    bottom: var(--spacing-xs, 4px);
    right: var(--spacing-xs, 4px);
  }
  
  .action-bar--bottom-left {
    bottom: var(--spacing-xs, 4px);
    left: var(--spacing-xs, 4px);
  }
  
  .action-bar__button {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    padding: 0;
    border: none;
    border-radius: calc(var(--border-radius, 6px) - 2px);
    background: transparent;
    color: var(--color-text-primary, #e6e6e6);
    cursor: pointer;
    transition: background-color 0.1s, color 0.1s;
  }
  
  .action-bar__button:hover:not(:disabled) {
    background: var(--color-bg-tertiary, #2d2d2d);
  }
  
  .action-bar__button:focus {
    outline: 2px solid var(--color-accent, #58a6ff);
    outline-offset: -2px;
  }
  
  .action-bar__button:active:not(:disabled) {
    background: var(--color-bg-primary, #121212);
  }
  
  .action-bar__button.disabled {
    color: var(--color-text-muted, #6e7681);
    cursor: not-allowed;
    opacity: 0.5;
  }
  
  .action-bar__icon {
    font-size: 14px;
    line-height: 1;
  }
  
  /* Reduz movimento se preferido */
  @media (prefers-reduced-motion: reduce) {
    .action-bar {
      transition: opacity 0.1s;
      transform: none;
    }
    
    .action-bar.visible {
      transform: none;
    }
  }
</style>



