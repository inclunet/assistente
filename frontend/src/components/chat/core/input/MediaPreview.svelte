<script>
  import { createEventDispatcher } from 'svelte';
  import { _ } from 'svelte-i18n';
  
  // Props
  export let media = [];       // Array de { type, category, file, preview, altText, generatingAlt, icon, sizeFormatted }
  export let MEDIA_CATEGORIES = { IMAGE: 'image', AUDIO: 'audio' }; // Categorias de mídia
  
  const dispatch = createEventDispatcher();
  
  function removeMedia(index) {
    dispatch('remove', { index });
  }
</script>

{#if media.length > 0}
  <div class="pending-media" role="list" aria-label={$_('chat.addMedia')}>
    {#each media as item, index}
      <div class="media-preview" role="listitem" data-category={item.category}>
        
        <!-- Imagem: thumbnail com indicador de geração de alt -->
        {#if item.category === MEDIA_CATEGORIES.IMAGE && item.preview}
          <div class="media-thumbnail-wrapper">
            <img 
              src={item.preview} 
              alt={item.altText || item.file?.name || $_('chat.imageAlt')} 
              class="media-thumbnail"
              title={item.altText || item.file?.name || $_('chat.imageAlt')}
            />
            {#if item.generatingAlt}
              <span class="alt-generating" aria-label={$_('chat.loading')}>✨</span>
            {/if}
          </div>
        
        <!-- Áudio: mini player -->
        {:else if item.category === MEDIA_CATEGORIES.AUDIO && item.preview}
          <div class="media-audio-preview">
            <span class="media-icon" aria-hidden="true">{item.icon || '🎵'}</span>
            <audio 
              src={item.preview} 
              controls 
              class="audio-mini-player"
              title={item.file?.name || 'Audio'}
            >
              Your browser does not support audio.
            </audio>
          </div>
        
        <!-- Outros: ícone baseado na categoria -->
        {:else}
          <span class="media-icon" aria-hidden="true">
            {item.icon || '📎'}
          </span>
        {/if}
        
        <!-- Nome e info do arquivo -->
        <div class="media-info">
          <span class="media-name" title={item.altText || item.file?.name}>
            {#if item.generatingAlt}
              ✨ {$_('chat.loading')}
            {:else if item.altText && item.altText !== item.file?.name}
              {item.altText.substring(0, 40)}{item.altText.length > 40 ? '...' : ''}
            {:else}
              {item.file?.name || 'File'}
            {/if}
          </span>
          {#if item.sizeFormatted}
            <span class="media-size">{item.sizeFormatted}</span>
          {/if}
        </div>
        
        <button 
          type="button"
          class="media-remove"
          on:click={() => removeMedia(index)}
          aria-label="{$_('chat.removeMedia')} {item.altText || item.file?.name || ''}"
          title={$_('chat.removeMedia')}
        >✕</button>
      </div>
    {/each}
  </div>
{/if}

<style>
  .pending-media {
    display: flex;
    flex-wrap: wrap;
    gap: var(--chat-space-2);
    margin-bottom: var(--chat-space-2);
    padding: var(--chat-space-2);
    background: var(--chat-color-bg-tertiary);
    border-radius: var(--chat-radius-md);
  }
  
  .media-preview {
    display: flex;
    align-items: center;
    gap: var(--chat-space-2);
    padding: var(--chat-space-1) var(--chat-space-2);
    background: var(--chat-color-bg-secondary);
    border-radius: var(--chat-radius-sm);
    max-width: 200px;
  }
  
  .media-thumbnail-wrapper {
    position: relative;
    width: 40px;
    height: 40px;
    flex-shrink: 0;
  }
  
  .media-thumbnail {
    width: 100%;
    height: 100%;
    object-fit: cover;
    border-radius: var(--chat-radius-sm);
  }
  
  .alt-generating {
    position: absolute;
    top: -4px;
    right: -4px;
    font-size: var(--chat-font-size-xs);
    animation: pulse 1.5s ease-in-out infinite;
  }
  
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
  }
  
  .media-audio-preview {
    display: flex;
    align-items: center;
    gap: var(--chat-space-1);
  }
  
  .audio-mini-player {
    height: 24px;
    max-width: 100px;
  }
  
  .media-icon {
    font-size: var(--chat-font-size-xl);
    flex-shrink: 0;
  }
  
  .media-info {
    display: flex;
    flex-direction: column;
    min-width: 0;
    flex: 1;
  }
  
  .media-name {
    font-size: var(--chat-font-size-sm);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--chat-color-text);
  }
  
  .media-size {
    font-size: var(--chat-font-size-xs);
    color: var(--chat-color-text-muted);
  }
  
  .media-remove {
    padding: var(--chat-space-1);
    background: transparent;
    border: none;
    cursor: pointer;
    font-size: var(--chat-font-size-sm);
    color: var(--chat-color-text-muted);
    border-radius: var(--chat-radius-sm);
    flex-shrink: 0;
    transition: background-color var(--chat-transition-fast), color var(--chat-transition-fast);
  }
  
  .media-remove:hover {
    background: rgba(220, 53, 69, 0.1);
    color: var(--chat-color-error);
  }

  .media-remove:focus-visible {
    outline: 2px solid var(--chat-color-border-focus);
    outline-offset: 2px;
  }
</style>
