<script>
  import { createEventDispatcher } from 'svelte';
  import { _ } from 'svelte-i18n';
  
  export let disabled = false;
  export let hasMedia = false; // Indica se já tem mídia anexada
  
  const dispatch = createEventDispatcher();
  
  function handleClick() {
    dispatch('click');
  }
  
  function handleContextMenu(event) {
    dispatch('contextmenu', { event });
  }
</script>

<button 
  type="button"
  class="media-btn"
  class:has-media={hasMedia}
  {disabled}
  aria-label={$_('chat.addMedia')}
  title={$_('chat.addMedia')}
  on:click={handleClick}
  on:contextmenu={handleContextMenu}
>
  <slot>
    📎
  </slot>
</button>

<style>
  .media-btn {
    padding: var(--chat-space-2, 0.5rem);
    border-radius: var(--chat-radius-md, 0.5rem);
    font-size: 1.25rem;
    min-width: 40px;
    min-height: 40px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--chat-btn-secondary-bg, #e5e7eb);
    color: var(--chat-btn-secondary-text, #374151);
    border: none;
    cursor: pointer;
    transition: background-color var(--chat-transition-fast, 150ms ease);
  }
  
  .media-btn:hover:not(:disabled) {
    background: var(--chat-btn-secondary-hover, #d1d5db);
  }
  
  .media-btn:focus-visible {
    outline: 2px solid var(--chat-color-border-focus, #3b82f6);
    outline-offset: 2px;
  }
  
  .media-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  
  .media-btn.has-media {
    background: var(--chat-btn-primary-bg, #3b82f6);
    color: var(--chat-btn-primary-text, #ffffff);
  }
</style>




