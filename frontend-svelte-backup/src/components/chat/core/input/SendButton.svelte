<script>
  import { createEventDispatcher } from 'svelte';
  import { _ } from 'svelte-i18n';
  
  export let disabled = false;
  export let isLoading = false;
  export let isGeneratingAltText = false;
  export let type = 'submit';
  
  const dispatch = createEventDispatcher();
  
  function handleClick() {
    if (!disabled) {
      dispatch('click');
    }
  }
  
  $: ariaLabel = isLoading 
    ? $_('chat.sending') 
    : isGeneratingAltText 
      ? $_('chat.loading') 
      : $_('chat.send');
</script>

<button 
  {type}
  class="btn-primary send-btn"
  {disabled}
  aria-label={ariaLabel}
  aria-busy={isLoading || isGeneratingAltText}
  title={isGeneratingAltText ? $_('chat.loading') : ''}
  on:click={handleClick}
>
  {#if isLoading}
    <span class="loading-spinner" aria-hidden="true"></span>
  {:else if isGeneratingAltText}
    <span class="generating-indicator" aria-hidden="true">✨</span> {$_('chat.loading')}
  {:else}
    <slot>
      📤 {$_('chat.send')}
    </slot>
  {/if}
</button>

<style>
  .send-btn {
    padding: var(--chat-space-3, 0.75rem) var(--chat-space-4, 1rem);
    border-radius: var(--chat-radius-md, 0.5rem);
    font-size: var(--chat-font-size-base, 1rem);
    min-width: 100px;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--chat-space-1, 0.25rem);
  }
  
  .btn-primary {
    background: var(--chat-btn-primary-bg, #3b82f6);
    color: var(--chat-btn-primary-text, #ffffff);
    border: none;
    cursor: pointer;
    transition: background-color var(--chat-transition-fast, 150ms ease);
  }
  
  .btn-primary:hover:not(:disabled) {
    background: var(--chat-btn-primary-hover, #2563eb);
  }
  
  .btn-primary:focus-visible {
    outline: 2px solid var(--chat-color-border-focus, #3b82f6);
    outline-offset: 2px;
  }
  
  .btn-primary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  
  .loading-spinner {
    width: 16px;
    height: 16px;
    border: 2px solid rgba(255,255,255,0.3);
    border-top-color: white;
    border-radius: var(--chat-radius-full, 9999px);
    animation: spin 0.8s linear infinite;
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
  
  .generating-indicator {
    animation: pulse 1.5s ease-in-out infinite;
  }
  
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
  }
</style>





