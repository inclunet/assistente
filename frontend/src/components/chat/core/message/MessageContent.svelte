<script>
  import { createEventDispatcher } from 'svelte';
  import { Markdown } from '../../../markdown';
  
  // Props
  export let content = '';
  export let media = [];           // Array de mídia anexada
  export let isStreaming = false;
  export let toolsInfo = '';       // Info de tools em execução
  export let truncate = 0;         // 0 = sem truncar, >0 = máx caracteres
  
  const dispatch = createEventDispatcher();
  
  // Detecta se há imagem gerada no conteúdo
  // Formato: [GENERATED_IMAGE:alt_base64:image_base64]
  function hasGeneratedImage(text) {
    if (!text) return false;
    return text.includes('[GENERATED_IMAGE:');
  }
  
  // Extrai dados da imagem gerada
  function extractGeneratedImage(text) {
    if (!text) return null;
    
    const match = text.match(/\[GENERATED_IMAGE:([^:]+):([^\]]+)\]/);
    if (!match) return null;
    
    const altTextBase64 = match[1];
    const imageBase64 = match[2];
    
    // Decodifica alt-text
    let altText = 'Imagem gerada';
    try {
      altText = atob(altTextBase64);
    } catch (e) {
      console.warn('Erro ao decodificar alt-text:', e);
    }
    
    // Extrai texto antes e depois da imagem
    const fullMatch = match[0];
    const startIndex = text.indexOf(fullMatch);
    const textBefore = text.substring(0, startIndex).trim();
    const textAfter = text.substring(startIndex + fullMatch.length).trim();
    
    return {
      id: Date.now(),
      altText,
      imageBase64,
      imageUrl: `data:image/png;base64,${imageBase64}`,
      textBefore,
      textAfter
    };
  }
  
  // Conteúdo possivelmente truncado
  $: displayContent = truncate > 0 && content?.length > truncate 
    ? content.substring(0, truncate) + '...' 
    : content;
  
  // Handlers de eventos para imagem gerada
  function handleImageDownload(imageData) {
    dispatch('imageDownload', imageData);
  }
  
  function handleImageCopy(imageData) {
    dispatch('imageCopy', imageData);
  }
  
  function handleImageZoom(imageData) {
    dispatch('imageZoom', imageData);
  }
  
  function handleMediaClick(mediaItem) {
    dispatch('mediaClick', mediaItem);
  }
</script>

{#if isStreaming && !content}
  <!-- Indicador de streaming -->
  {#if toolsInfo}
    <span class="tools-indicator" aria-hidden="true">
      {toolsInfo}
    </span>
  {:else}
    <span aria-hidden="true" class="typing-indicator">
      <span></span><span></span><span></span>
    </span>
  {/if}
{:else}
  <!-- Mídia anexada -->
  {#if media && media.length > 0}
    <div class="message-media">
      {#each media as mediaItem, idx}
        {#if mediaItem.type === 'image' || mediaItem.type === 'screenshot' || mediaItem.type === 'webcam'}
          {@const imageDesc = mediaItem.altText || mediaItem.file?.name || 'Imagem enviada'}
          <figure 
            class="message-image" 
            role="img" 
            aria-label={imageDesc}
            on:click={() => handleMediaClick(mediaItem)}
            on:keydown={(e) => e.key === 'Enter' && handleMediaClick(mediaItem)}
          >
            <img 
              src={mediaItem.preview} 
              alt={imageDesc}
              loading="lazy"
            />
          </figure>
        {:else if mediaItem.type === 'audio'}
          <div class="message-audio">
            <span class="media-icon" aria-hidden="true">🎵</span>
            <span>{mediaItem.file?.name || 'Áudio'}</span>
          </div>
        {:else if mediaItem.type === 'document'}
          <div class="message-document">
            <span class="media-icon" aria-hidden="true">📄</span>
            <span>{mediaItem.file?.name || 'Documento'}</span>
          </div>
        {/if}
      {/each}
    </div>
  {/if}
  
  <!-- Conteúdo de texto -->
  {#if content}
    {#if hasGeneratedImage(content)}
      {@const imageData = extractGeneratedImage(content)}
      
      <!-- Texto antes da imagem -->
      {#if imageData?.textBefore}
        <Markdown content={imageData.textBefore} />
      {/if}
      
      <!-- Imagem gerada -->
      {#if imageData}
        <div class="generated-image" role="figure" aria-labelledby="img-desc-{imageData.id}">
          <img 
            src={imageData.imageUrl} 
            alt={imageData.altText}
            class="generated-image__img"
            loading="lazy"
          />
          
          <details class="generated-image__description">
            <summary>📖 Descrição da imagem</summary>
            <p id="img-desc-{imageData.id}">{imageData.altText}</p>
          </details>
          
          <div class="generated-image__actions">
            <button 
              class="btn-secondary"
              on:click={() => handleImageDownload(imageData)} 
              aria-label="Download da imagem"
            >
              💾 Download
            </button>
            <button 
              class="btn-secondary"
              on:click={() => handleImageCopy(imageData)} 
              aria-label="Copiar imagem"
            >
              📋 Copiar
            </button>
            <button 
              class="btn-secondary"
              on:click={() => handleImageZoom(imageData)} 
              aria-label="Ver em tamanho maior"
            >
              🔍 Ampliar
            </button>
          </div>
        </div>
      {/if}
      
      <!-- Texto depois da imagem -->
      {#if imageData?.textAfter}
        <Markdown content={imageData.textAfter} />
      {/if}
    {:else}
      <!-- Conteúdo normal (markdown ou texto) -->
      {#if truncate > 0}
        <pre class="content-text">{displayContent}</pre>
      {:else}
        <Markdown content={content} />
      {/if}
    {/if}
  {/if}
{/if}

<style>
  .message-media {
    display: flex;
    flex-wrap: wrap;
    gap: var(--chat-space-2);
    margin-bottom: var(--chat-space-2);
  }
  
  .message-image {
    max-width: 300px;
    border-radius: var(--chat-radius-lg);
    overflow: hidden;
    cursor: pointer;
    margin: 0;
  }
  
  .message-image img {
    width: 100%;
    height: auto;
    display: block;
  }
  
  .message-image:hover {
    opacity: 0.9;
  }
  
  .message-audio,
  .message-document {
    display: flex;
    align-items: center;
    gap: var(--chat-space-2);
    padding: var(--chat-space-2);
    background: var(--chat-color-bg-secondary);
    border-radius: var(--chat-radius-sm);
  }
  
  .media-icon {
    font-size: var(--chat-font-size-xl);
  }
  
  .typing-indicator {
    display: inline-flex;
    gap: 4px;
    padding: var(--chat-space-2);
  }
  
  .typing-indicator span {
    width: 8px;
    height: 8px;
    border-radius: var(--chat-radius-full);
    background: var(--chat-color-text-muted);
    animation: bounce 1.4s infinite ease-in-out both;
  }
  
  .typing-indicator span:nth-child(1) { animation-delay: -0.32s; }
  .typing-indicator span:nth-child(2) { animation-delay: -0.16s; }
  .typing-indicator span:nth-child(3) { animation-delay: 0s; }
  
  @keyframes bounce {
    0%, 80%, 100% { transform: scale(0); }
    40% { transform: scale(1); }
  }
  
  .tools-indicator {
    color: var(--chat-color-text-muted);
    font-style: italic;
  }
  
  .generated-image {
    margin: var(--chat-space-4) 0;
    max-width: 100%;
  }
  
  .generated-image__img {
    max-width: 100%;
    height: auto;
    border-radius: var(--chat-radius-lg);
    box-shadow: var(--chat-shadow-md);
  }
  
  .generated-image__description {
    margin-top: var(--chat-space-2);
    font-size: var(--chat-font-size-sm);
  }
  
  .generated-image__description summary {
    cursor: pointer;
    color: var(--chat-color-primary);
  }
  
  .generated-image__actions {
    display: flex;
    gap: var(--chat-space-2);
    margin-top: var(--chat-space-2);
    flex-wrap: wrap;
  }
  
  .generated-image__actions .btn-secondary {
    padding: var(--chat-space-1) var(--chat-space-2);
    font-size: var(--chat-font-size-sm);
    background: var(--chat-color-bg-tertiary);
    color: var(--chat-color-text);
    border: 1px solid var(--chat-color-border);
    border-radius: var(--chat-radius-md);
    cursor: pointer;
  }
  
  .generated-image__actions .btn-secondary:hover {
    background: var(--chat-color-hover);
  }
  
  .content-text {
    margin: 0;
    white-space: pre-wrap;
    word-break: break-word;
    font-family: var(--chat-font-family);
    font-size: var(--chat-font-size-base);
    line-height: var(--chat-line-height);
  }
</style>
