<script>
  import { createEventDispatcher } from 'svelte';
  import { _ } from 'svelte-i18n';
  
  // Props
  export let visible = true;
  export let text = '';           // Texto do que está sendo processado
  export let showCancel = false;  // Mostrar botão de cancelar
  
  const dispatch = createEventDispatcher();
  
  $: displayText = text || $_('chat.loading');
  
  function handleCancel() {
    dispatch('cancel');
  }
</script>

{#if visible}
  <div class="streaming-indicator" role="status" aria-live="polite">
    <span class="pulse" aria-hidden="true"></span>
    <span class="streaming-text">{displayText}</span>
    
    {#if showCancel}
      <button 
        type="button"
        class="cancel-btn"
        on:click={handleCancel}
        aria-label={$_('chat.cancel')}
      >
        ✕
      </button>
    {/if}
  </div>
{/if}

<style>
  .streaming-indicator {
    display: inline-flex;
    align-items: center;
    gap: var(--chat-space-2);
    padding: var(--chat-space-1) var(--chat-space-2);
    background: rgba(13, 110, 253, 0.1);
    border-radius: var(--chat-radius-sm);
    font-size: var(--chat-font-size-sm);
    color: var(--chat-color-text);
  }
  
  .pulse {
    width: 10px;
    height: 10px;
    border-radius: var(--chat-radius-full);
    background: var(--chat-color-primary);
    animation: pulse 1.5s ease-in-out infinite;
  }
  
  @keyframes pulse {
    0%, 100% { 
      transform: scale(1); 
      opacity: 1; 
    }
    50% { 
      transform: scale(1.2); 
      opacity: 0.7; 
    }
  }
  
  .streaming-text {
    font-style: italic;
  }
  
  .cancel-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    margin-left: var(--chat-space-1);
    border: none;
    background: transparent;
    color: var(--chat-color-text-muted);
    border-radius: var(--chat-radius-sm);
    cursor: pointer;
    font-size: var(--chat-font-size-xs);
    transition: background-color var(--chat-transition-fast), color var(--chat-transition-fast);
  }
  
  .cancel-btn:hover {
    background: var(--chat-color-hover);
    color: var(--chat-color-error);
  }
  
  .cancel-btn:focus-visible {
    outline: 2px solid var(--chat-color-border-focus);
    outline-offset: 2px;
  }
</style>

