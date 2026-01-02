<script>
  import { createEventDispatcher } from 'svelte';
  import { _ } from 'svelte-i18n';
  
  const dispatch = createEventDispatcher();
  
  // Props - Estado
  export let isHovered = false;
  export let isStreaming = false;
  export let isTTSDisabled = true;
  export let level = 0;
  export let showHoverActions = false;
  export let speakable = true;
  
  $: shouldShow = showHoverActions && isHovered && !isStreaming && level === 0;
</script>

{#if shouldShow}
  <div class="message-actions" aria-hidden="true">
    {#if speakable && !isTTSDisabled}
      <button 
        class="action-btn"
        on:click={() => dispatch('speak')}
        aria-label={$_('chat.speak')}
        title="{$_('chat.speak')} (Espaço)"
        tabindex="-1"
      >🔊</button>
    {/if}
    <button 
      class="action-btn"
      on:click={() => dispatch('copy')}
      aria-label={$_('chat.copy')}
      title="{$_('chat.copy')} (Ctrl+C)"
      tabindex="-1"
    >📋</button>
    <button 
      class="action-btn menu-btn"
      on:click={(e) => dispatch('contextMenu', { event: e })}
      aria-label={$_('chat.moreActions')}
      aria-haspopup="menu"
      title={$_('chat.moreActions')}
      tabindex="-1"
    >⋮</button>
  </div>
{/if}

<style>
  /* Ações no hover */
  .message-actions {
    position: absolute;
    top: var(--chat-space-2);
    right: var(--chat-space-2);
    display: flex;
    gap: var(--chat-space-1);
    background: var(--chat-color-bg);
    border-radius: var(--chat-radius-sm);
    padding: var(--chat-space-1);
    box-shadow: var(--chat-shadow-md);
  }
  
  .action-btn {
    padding: var(--chat-space-1) var(--chat-space-2);
    background: var(--chat-action-bg);
    color: var(--chat-action-text);
    border: none;
    cursor: pointer;
    font-size: var(--chat-font-size-base);
    border-radius: var(--chat-radius-sm);
    transition: background-color var(--chat-transition-fast);
  }
  
  .action-btn:hover {
    background: var(--chat-action-hover-bg);
  }
  
  .action-btn:focus-visible {
    outline: 2px solid var(--chat-color-border-focus);
    outline-offset: 2px;
  }
</style>
