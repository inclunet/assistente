<script>
  import { createEventDispatcher, getContext, onDestroy } from 'svelte';
  import MediaPreview from './MediaPreview.svelte';
  import { _ } from 'svelte-i18n';
  import { CHAT_NAVIGATION_KEY } from '../../context/navigation.js';
  
  // Props - Dados
  export let inputMessage = '';
  export let inputElement = null;
  export let pendingMedia = [];
  export let mediaError = '';
  
  // Props - Estado
  export let disabled = false;
  export let isLoading = false;
  export let isGeneratingAltText = false;
  export let canSendMessage = true;
  export let isDragging = false;
  
  // Props - Modo de mídia/voz
  export let mediaMode = 'normal'; // 'normal' | 'record_audio' | 'recording'
  export let voiceEnabled = true;
  export let showVoiceButton = false;
  export let isRecording = false; // Estado de gravação
  
  // Props - Configuração
  export let MEDIA_CATEGORIES = { IMAGE: 'image', AUDIO: 'audio' };
  
  // Props - Placeholder dinâmico (opcional, senão usa i18n)
  export let placeholderNormal = '';
  export let placeholderRecordAudio = '';
  export let placeholderWithMedia = '';
  export let hintText = '';
  
  const dispatch = createEventDispatcher();
  
  // === Navigation Context ===
  // Pega o contexto de navegação (se existir - pode ser usado standalone)
  const navigation = getContext(CHAT_NAVIGATION_KEY);
  
  // Reage a solicitações de foco via contexto
  let unsubscribe;
  if (navigation) {
    unsubscribe = navigation.subscribe(state => {
      if (state.focusTarget === 'input') {
        // Foca no input
        inputElement?.focus();
        navigation.clearFocusTarget();
      }
    });
  }
  
  onDestroy(() => {
    unsubscribe?.();
  });
  
  // Placeholder computado
  $: placeholder = mediaMode === 'record_audio' 
    ? (placeholderRecordAudio || $_('chat.placeholder'))
    : pendingMedia.length > 0 
      ? (placeholderWithMedia || $_('chat.placeholder'))
      : (placeholderNormal || $_('chat.placeholder'));
  
  // === Handlers que emitem eventos ===
  
  function handleSubmit(event) {
    event.preventDefault();
    dispatch('submit');
  }
  
  function handleKeyDown(event) {
    // Navegação interna: ↑ com input vazio → última mensagem
    if (event.key === 'ArrowUp' && !inputMessage?.trim() && navigation) {
      event.preventDefault();
      navigation.focusLastMessage();
      return;
    }
    
    dispatch('keydown', { event });
  }
  
  function handlePaste(event) {
    dispatch('paste', { event });
  }
  
  function handleDragEnter(event) {
    dispatch('dragenter', { event });
  }
  
  function handleDragOver(event) {
    dispatch('dragover', { event });
  }
  
  function handleDragLeave(event) {
    dispatch('dragleave', { event });
  }
  
  function handleDrop(event) {
    dispatch('drop', { event });
  }
  
  function handleRemoveMedia(event) {
    dispatch('removeMedia', event.detail);
  }
  
  function clearMediaError() {
    dispatch('clearMediaError');
  }
  
  // === Eventos de gravação de voz ===
  // O componente dispara eventos, quem usa implementa a lógica de gravação
  
  function startRecording() {
    dispatch('recordStart');
  }
  
  function stopRecording() {
    dispatch('recordStop');
  }
  
  function cancelRecording() {
    dispatch('recordCancel');
  }
  
  function toggleRecording() {
    if (isRecording) {
      stopRecording();
    } else {
      startRecording();
    }
  }
  
  // Expor métodos públicos se necessário
  export { startRecording, stopRecording, cancelRecording, toggleRecording };
</script>

<form 
  class="input-area" 
  class:dragging={isDragging}
  on:submit={handleSubmit}
  on:dragenter={handleDragEnter}
  on:dragover={handleDragOver}
  on:dragleave={handleDragLeave}
  on:drop={handleDrop}
>
  <!-- Slot: prefix (conteúdo antes do input, ex: MediaPicker) -->
  <slot name="prefix">
    <!-- Prefix padrão vazio -->
  </slot>
  
  <!-- Slot: mediaPreview (preview de mídia customizado) -->
  <slot name="mediaPreview" media={pendingMedia} {MEDIA_CATEGORIES}>
    <MediaPreview 
      media={pendingMedia}
      {MEDIA_CATEGORIES}
      on:remove={handleRemoveMedia}
    />
  </slot>
  
  {#if mediaError}
    <div class="media-error" role="alert">
      ⚠️ {mediaError}
      <button type="button" class="media-error-close" on:click={clearMediaError}>✕</button>
    </div>
  {/if}
  
  <div class="input-row">
    <label for="message-input" class="visually-hidden">
      {$_('chat.placeholder')}
    </label>
    <textarea
      id="message-input"
      bind:this={inputElement}
      bind:value={inputMessage}
      on:keydown={handleKeyDown}
      on:paste={handlePaste}
      on:focus={() => dispatch('focus')}
      on:blur={() => dispatch('blur')}
      on:input={() => dispatch('typing', { isTyping: inputMessage.length > 0 })}
      {placeholder}
      {disabled}
      rows="2"
    ></textarea>
    
    <!-- Slot: buttons (botões customizados, VoiceButton ou Send) -->
    <slot name="buttons" {isRecording} {toggleRecording} {startRecording} {stopRecording} {cancelRecording}>
      <!-- Default: mostra botão de voz OU botão de enviar -->
      {#if showVoiceButton && !inputMessage.trim() && pendingMedia.length === 0}
        <button 
          type="button" 
          class="btn-primary voice-btn"
          class:recording={isRecording}
          on:click={toggleRecording}
          disabled={disabled}
          aria-label={isRecording ? $_('chat.stopRecording') : $_('chat.startRecording')}
          aria-pressed={isRecording}
        >
          {#if isRecording}
            ⏹️
          {:else}
            🎤
          {/if}
        </button>
      {:else}
        <button 
          type="submit" 
          class="btn-primary send-btn"
          disabled={!canSendMessage}
          aria-label={isLoading ? $_('chat.sending') : isGeneratingAltText ? $_('chat.loading') : $_('chat.send')}
          aria-busy={isLoading || isGeneratingAltText}
          title={isGeneratingAltText ? $_('chat.loading') : ''}
        >
          {#if isLoading}
            <span class="loading-spinner" aria-hidden="true"></span>
          {:else if isGeneratingAltText}
            <span class="generating-indicator" aria-hidden="true">✨</span> {$_('chat.loading')}
          {:else}
            📤 {$_('chat.send')}
          {/if}
        </button>
      {/if}
    </slot>
  </div>
  
  <!-- Slot: suffix (conteúdo após o input, ex: dicas) -->
  <slot name="suffix">
    {#if voiceEnabled && hintText}
      <div class="input-hint" aria-hidden="true">
        <span class="hint-text">{hintText}</span>
      </div>
    {/if}
  </slot>
</form>

<style>
  .input-area {
    display: flex;
    flex-direction: column;
    gap: var(--chat-space-2);
    padding: var(--chat-space-3);
    background: var(--chat-input-bg);
    border-top: 1px solid var(--chat-color-border);
  }
  
  .input-area.dragging {
    background: var(--chat-color-hover);
    border: 2px dashed var(--chat-btn-primary-bg);
  }
  
  .input-row {
    display: flex;
    gap: var(--chat-space-2);
    align-items: flex-end;
  }
  
  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
  
  textarea {
    flex: 1;
    padding: var(--chat-space-3);
    border: 1px solid var(--chat-input-border);
    border-radius: var(--chat-radius-md);
    font-family: var(--chat-font-family);
    font-size: var(--chat-font-size-base);
    resize: none;
    min-height: 44px;
    background: var(--chat-input-bg);
    color: var(--chat-input-text);
  }
  
  textarea::placeholder {
    color: var(--chat-input-placeholder);
  }
  
  textarea:focus {
    border-color: var(--chat-input-focus-border);
    box-shadow: 0 0 0 3px var(--chat-input-focus-ring);
    outline: none;
  }
  
  textarea:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  
  .send-btn {
    padding: var(--chat-space-3) var(--chat-space-4);
    border-radius: var(--chat-radius-md);
    font-size: var(--chat-font-size-base);
    min-width: 100px;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--chat-space-1);
  }
  
  .btn-primary {
    background: var(--chat-btn-primary-bg);
    color: var(--chat-btn-primary-text);
    border: none;
    cursor: pointer;
    transition: background-color var(--chat-transition-fast);
  }
  
  .btn-primary:hover:not(:disabled) {
    background: var(--chat-btn-primary-hover);
  }
  
  .btn-primary:focus-visible {
    outline: 2px solid var(--chat-color-border-focus);
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
    border-radius: var(--chat-radius-full);
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
  
  .media-error {
    display: flex;
    align-items: center;
    gap: var(--chat-space-2);
    padding: var(--chat-space-2);
    background: rgba(220, 53, 69, 0.1);
    color: var(--chat-color-error);
    border-radius: var(--chat-radius-sm);
    font-size: var(--chat-font-size-sm);
  }
  
  .media-error-close {
    margin-left: auto;
    padding: var(--chat-space-1);
    background: transparent;
    border: none;
    cursor: pointer;
    font-size: var(--chat-font-size-base);
    color: inherit;
    border-radius: var(--chat-radius-sm);
  }
  
  .media-error-close:hover {
    background: rgba(0,0,0,0.1);
  }
  
  .voice-btn {
    padding: var(--chat-space-3);
    border-radius: var(--chat-radius-full);
    font-size: 1.25rem;
    min-width: 44px;
    min-height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  .voice-btn.recording {
    background: var(--chat-color-error);
    animation: pulse-recording 1s ease-in-out infinite;
  }
  
  @keyframes pulse-recording {
    0%, 100% { 
      box-shadow: 0 0 0 0 rgba(220, 53, 69, 0.4);
    }
    50% { 
      box-shadow: 0 0 0 8px rgba(220, 53, 69, 0);
    }
  }
  
  .input-hint {
    font-size: var(--chat-font-size-xs);
    color: var(--chat-color-text-muted);
    text-align: center;
  }
</style>
