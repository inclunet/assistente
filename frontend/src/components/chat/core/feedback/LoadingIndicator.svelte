<script>
  import { _ } from 'svelte-i18n';
  
  // Props
  export let visible = true;
  export let size = 'md';    // 'sm' | 'md' | 'lg'
  export let text = '';      // Texto opcional
  
  $: displayText = text || $_('chat.loading');
</script>

{#if visible}
  <div class="loading-indicator {size}" role="status" aria-label={displayText}>
    <span class="spinner" aria-hidden="true"></span>
    {#if text}
      <span class="loading-text">{displayText}</span>
    {/if}
  </div>
{/if}

<style>
  .loading-indicator {
    display: inline-flex;
    align-items: center;
    gap: var(--chat-space-2);
  }
  
  .spinner {
    border-radius: var(--chat-radius-full);
    border-style: solid;
    border-color: var(--chat-color-primary);
    border-top-color: transparent;
    animation: spin 0.8s linear infinite;
  }
  
  /* Tamanhos */
  .loading-indicator.sm .spinner {
    width: 16px;
    height: 16px;
    border-width: 2px;
  }
  
  .loading-indicator.md .spinner {
    width: 24px;
    height: 24px;
    border-width: 3px;
  }
  
  .loading-indicator.lg .spinner {
    width: 32px;
    height: 32px;
    border-width: 4px;
  }
  
  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
  
  .loading-text {
    font-size: var(--chat-font-size-sm);
    color: var(--chat-color-text-muted);
  }
</style>

