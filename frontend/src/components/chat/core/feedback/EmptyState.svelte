<script>
  import { _ } from 'svelte-i18n';
  
  // Props
  export let selectedModel = '';
  export let greeting = '';
  
  $: displayGreeting = greeting || $_('chat.emptyChat');
</script>

<div class="empty-state" role="status">
  <p>{displayGreeting}</p>
  <p class="empty-state-hint">
    {#if selectedModel}
      Model: {selectedModel}. {$_('chat.emptyDescription')}
    {:else}
      {$_('chat.loading')}
    {/if}
  </p>
</div>

<style>
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: var(--chat-space-8);
    text-align: center;
    color: var(--chat-color-text-secondary);
    flex: 1;
  }
  
  .empty-state p {
    margin: var(--chat-space-2) 0;
  }
  
  .empty-state-hint {
    color: var(--chat-color-text-muted);
    font-size: var(--chat-font-size-sm);
  }
</style>
