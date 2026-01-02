<script>
  import { _ } from 'svelte-i18n';
  
  // Props
  export let editContent = '';
  export let onSaveEdit = () => {};
  export let onCancelEdit = () => {};
  
  function handleKeyDown(e) {
    if (e.key === 'Enter' && e.ctrlKey) onSaveEdit();
    if (e.key === 'Escape') onCancelEdit();
  }
</script>

<div class="edit-container">
  <textarea
    class="edit-input"
    bind:value={editContent}
    on:keydown={handleKeyDown}
    rows="3"
    aria-label={$_('chat.editPlaceholder')}
  ></textarea>
  <div class="edit-actions">
    <button class="btn-primary btn-sm" on:click={onSaveEdit}>{$_('chat.saveEdit')}</button>
    <button class="btn-secondary btn-sm" on:click={onCancelEdit}>{$_('chat.cancelEdit')}</button>
  </div>
</div>

<style>
  .edit-container {
    display: flex;
    flex-direction: column;
    gap: var(--chat-space-2);
  }
  
  .edit-input {
    width: 100%;
    padding: var(--chat-space-2);
    background: var(--chat-input-bg);
    color: var(--chat-input-text);
    border: 1px solid var(--chat-input-border);
    border-radius: var(--chat-radius-md);
    font-family: var(--chat-font-family);
    font-size: var(--chat-font-size-base);
    resize: vertical;
  }
  
  .edit-input:focus {
    border-color: var(--chat-input-focus-border);
    box-shadow: 0 0 0 3px var(--chat-input-focus-ring);
    outline: none;
  }
  
  .edit-actions {
    display: flex;
    gap: var(--chat-space-2);
  }
  
  .btn-sm {
    padding: var(--chat-space-1) var(--chat-space-2);
    font-size: var(--chat-font-size-sm);
  }
  
  .btn-primary {
    background: var(--chat-btn-primary-bg);
    color: var(--chat-btn-primary-text);
    border: none;
    border-radius: var(--chat-radius-md);
    cursor: pointer;
    transition: background-color var(--chat-transition-fast);
  }
  
  .btn-primary:hover {
    background: var(--chat-btn-primary-hover);
  }
  
  .btn-primary:focus-visible {
    outline: 2px solid var(--chat-color-border-focus);
    outline-offset: 2px;
  }
  
  .btn-secondary {
    background: var(--chat-color-bg-tertiary);
    color: var(--chat-color-text);
    border: 1px solid var(--chat-color-border);
    border-radius: var(--chat-radius-md);
    cursor: pointer;
    transition: background-color var(--chat-transition-fast);
  }
  
  .btn-secondary:hover {
    background: var(--chat-color-hover);
  }
  
  .btn-secondary:focus-visible {
    outline: 2px solid var(--chat-color-border-focus);
    outline-offset: 2px;
  }
</style>

