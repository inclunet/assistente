<script>
  import { createEventDispatcher } from 'svelte';
  import { _ } from 'svelte-i18n';
  
  // Props
  export let childCount = 0;
  export let isExpanded = false;
  export let isLoading = false;
  
  const dispatch = createEventDispatcher();
  
  $: hasChildren = childCount > 0;
  
  function handleClick(event) {
    event.stopPropagation();
    if (!isLoading) {
      dispatch('toggle', { expand: !isExpanded });
    }
  }
</script>

{#if hasChildren}
  <button 
    class="thread-indicator"
    class:expanded={isExpanded}
    on:click={handleClick}
    aria-expanded={isExpanded}
    aria-label={isExpanded ? $_('chat.collapse') : $_('chat.expand')}
    tabindex="-1"
    disabled={isLoading}
  >
    {#if isLoading}
      <span class="loading-spinner">⏳</span>
    {:else}
      <span class="expand-arrow" class:expanded={isExpanded}>▶</span>
    {/if}
    <span class="child-count">{childCount} {$_('chat.interactions')}</span>
  </button>
{/if}

<style>
  .thread-indicator {
    display: inline-flex;
    align-items: center;
    gap: var(--chat-space-1);
    padding: var(--chat-space-1) var(--chat-space-2);
    background: var(--chat-color-bg-tertiary);
    border: 1px solid var(--chat-color-border);
    border-radius: var(--chat-radius-sm);
    font-size: var(--chat-font-size-sm);
    color: var(--chat-color-text);
    cursor: pointer;
    margin-left: auto;
    transition: background-color var(--chat-transition-fast);
  }
  
  .thread-indicator:hover:not(:disabled) {
    background: var(--chat-color-hover);
  }
  
  .thread-indicator:focus-visible {
    outline: 2px solid var(--chat-color-border-focus);
    outline-offset: 2px;
  }
  
  .thread-indicator:disabled {
    opacity: 0.6;
    cursor: wait;
  }
  
  .expand-arrow {
    transition: transform var(--chat-transition-fast);
    display: inline-block;
    font-size: var(--chat-font-size-sm);
  }
  
  .expand-arrow.expanded {
    transform: rotate(90deg);
  }
  
  .loading-spinner {
    animation: spin 1s linear infinite;
  }
  
  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
  
  .child-count {
    color: var(--chat-color-text-muted);
  }
</style>
