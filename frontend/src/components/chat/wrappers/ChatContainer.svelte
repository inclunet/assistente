<script>
  import { createEventDispatcher, onMount, tick, setContext } from 'svelte';
  import { _ } from 'svelte-i18n';
  
  // Core components
  import ChatHistory from '../core/ChatHistory.svelte';
  import ChatInput from '../core/input/ChatInput.svelte';
  import StreamingIndicator from '../core/feedback/StreamingIndicator.svelte';
  
  // Context for internal navigation
  import { createNavigationStore, CHAT_NAVIGATION_KEY } from '../context/navigation.js';
  
  const dispatch = createEventDispatcher();
  
  // Cria e provê o contexto de navegação
  const navigation = createNavigationStore();
  setContext(CHAT_NAVIGATION_KEY, navigation);
  
  // ========================================
  // Props - Dados
  // ========================================
  
  export let messages = [];           // Array de MessageNode
  export let threadedMessages = [];   // Alternativa: mensagens já organizadas em árvore
  export let currentUser = { id: 'user', name: 'Você' };
  
  // ========================================
  // Props - Handlers (Sistema de Callbacks)
  // ========================================
  
  /**
   * Sistema de handlers conforme documentação.
   * O ChatContainer chama estes handlers quando eventos ocorrem.
   * Se não fornecido, usa fallback (ação local ou ignora).
   * 
   * @type {import('../types').ChatHandlers}
   */
  export let handlers = {};
  
  // ========================================
  // Props - Configuração
  // ========================================
  
  export let config = {};
  
  // Desestrutura config com defaults
  $: enableTTS = config.enableTTS ?? false;
  $: enableThreads = config.enableThreads ?? true;
  $: enablePinning = config.enablePinning ?? false;
  $: enableEditing = config.enableEditing ?? false;
  $: enableDeleting = config.enableDeleting ?? false;
  $: enableMedia = config.enableMedia ?? true;
  $: lazyLoadChildren = config.lazyLoadChildren ?? true;
  $: autoScroll = config.autoScroll ?? true;
  $: placeholder = config.placeholder ?? '';
  $: showInternalMessages = config.showInternalMessages ?? false;
  
  // ========================================
  // Props - Estado (pode ser controlado externamente)
  // ========================================
  
  // Se passado, usa valor externo. Se não, usa estado interno.
  export let inputMessage = undefined;      // Controle externo do input
  export let pendingMedia = undefined;      // Mídia pendente externa
  export let expandedPaths = undefined;     // Expansão de threads externa
  export let loadingPaths = undefined;      // Paths carregando externa
  export let isLoading = false;             // Estado de loading
  export let isTyping = false;              // Estado de digitação
  export let isStreaming = false;           // Estado de streaming
  export let streamingText = '';            // Texto do streaming
  export let error = '';                    // Mensagem de erro
  export let selectedModel = '';            // Modelo selecionado (para EmptyState)
  export let disabled = false;              // Input desabilitado
  export let hoveredMessageIndex = -1;      // Índice da mensagem hovered
  export let focusedMessageIndex = -1;      // Índice da mensagem focada
  export let editingMessageIndex = -1;      // Índice da mensagem sendo editada
  export let editingMessageContent = '';    // Conteúdo sendo editado
  export let isTTSDisabled = true;          // TTS desabilitado
  
  // Props para ChatInput
  export let mediaError = '';
  export let isGeneratingAltText = false;
  export let canSendMessage = true;
  export let isDragging = false;
  export let mediaMode = 'normal';
  export let voiceEnabled = true;
  export let showVoiceButton = false;
  export let isRecording = false;
  export let MEDIA_CATEGORIES = {};
  export let hintText = '';
  
  // ========================================
  // Estado Interno (fallback se não controlado externamente)
  // ========================================
  
  let _inputValue = '';
  let _pendingMedia = [];
  let _expandedPaths = {};
  let _loadingPaths = {};
  let _hoveredIndex = -1;
  let _focusedIndex = -1;
  let _editingIndex = -1;
  let _editContent = '';
  
  // Use external props if provided, otherwise use internal state
  $: effectiveInputValue = inputMessage !== undefined ? inputMessage : _inputValue;
  $: effectivePendingMedia = pendingMedia !== undefined ? pendingMedia : _pendingMedia;
  $: effectiveExpandedPaths = expandedPaths !== undefined ? expandedPaths : _expandedPaths;
  $: effectiveLoadingPaths = loadingPaths !== undefined ? loadingPaths : _loadingPaths;
  $: effectiveHoveredIndex = hoveredMessageIndex !== undefined && hoveredMessageIndex !== -1 ? hoveredMessageIndex : _hoveredIndex;
  $: effectiveFocusedIndex = focusedMessageIndex !== undefined && focusedMessageIndex !== -1 ? focusedMessageIndex : _focusedIndex;
  $: effectiveEditingIndex = editingMessageIndex !== undefined && editingMessageIndex !== -1 ? editingMessageIndex : _editingIndex;
  $: effectiveEditContent = editingMessageContent !== undefined ? editingMessageContent : _editContent;
  
  // Menu de contexto e modais - 100% externos, apenas propagamos eventos
  
  // Referências
  let chatHistoryRef;
  let liveRegion;
  
  // ========================================
  // Config derivado para ChatHistory
  // ========================================
  
  $: historyConfig = {
    editable: enableEditing,
    deletable: enableDeleting,
    pinnable: enablePinning,
    speakable: enableTTS,
    showHoverActions: true,
    autoScroll,
  };
  
  // ========================================
  // Handlers Padrão (Ações Locais)
  // ========================================
  
  const defaultHandlers = {
    // Ações locais - executam imediatamente
    onCopy: async (text) => {
      await navigator.clipboard.writeText(text);
      return true;
    },
    onOpenLink: (url) => {
      window.open(url, '_blank');
      return true;
    },
  };
  
  $: finalHandlers = { ...defaultHandlers, ...handlers };
  
  // ========================================
  // Event Handlers - Ações
  // ========================================
  
  // --- Submit ---
  async function handleSubmit() {
    const content = (effectiveInputValue || '').trim();
    const media = [...(effectivePendingMedia || [])];
    
    if (!content && media.length === 0) return;
    
    // Dispara evento - quem usa decide se quer bloquear UI
    dispatch('submit', { content, media });
    
    // Se tem handler, chama
    if (finalHandlers.onSend) {
      try {
        await finalHandlers.onSend(content, media);
        
        // Limpa input interno se não controlado externamente
        if (inputMessage === undefined) {
          _inputValue = '';
        }
        if (pendingMedia === undefined) {
          _pendingMedia = [];
        }
        
        dispatch('messageSent', { content, media });
        announce($_('chat.send'));
      } catch (err) {
        console.error('Erro ao enviar:', err);
        dispatch('error', { error: err });
        if (finalHandlers.onError) {
          finalHandlers.onError(err);
        }
      }
    }
  }
  
  // --- Toggle (expandir/recolher) ---
  function handleToggle(event) {
    const { path, expand } = event.detail;
    
    if (expand) {
      expandedPaths = { ...expandedPaths, [path]: true };
    } else {
      expandedPaths = { ...expandedPaths };
      delete expandedPaths[path];
    }
    
    dispatch('toggle', event.detail);
  }
  
  // --- Load Children (lazy loading) ---
  async function handleLoadChildren(event) {
    const { messageId, path } = event.detail;
    
    if (!finalHandlers.onLoadChildren) {
      dispatch('loadChildren', event.detail);
      return;
    }
    
    loadingPaths = { ...loadingPaths, [path]: true };
    
    try {
      const children = await finalHandlers.onLoadChildren(messageId);
      dispatch('childrenLoaded', { messageId, path, children });
    } catch (err) {
      console.error('Erro ao carregar filhos:', err);
      if (finalHandlers.onError) {
        finalHandlers.onError(err);
      }
    } finally {
      loadingPaths = { ...loadingPaths };
      delete loadingPaths[path];
    }
  }
  
  // --- Speak (TTS) ---
  // 100% externo - apenas dispara evento, NUNCA executa TTS internamente
  function handleSpeak(event) {
    dispatch('speak', event.detail);
  }
  
  // --- Copy ---
  async function handleCopy(event) {
    const { message, format } = event.detail;
    const text = format === 'markdown' ? message?.content : (message?.content || '');
    
    try {
      await finalHandlers.onCopy(text);
      announce($_('chat.copied'));
      dispatch('copied', event.detail);
    } catch (err) {
      console.error('Erro ao copiar:', err);
    }
  }
  
  // --- Delete ---
  async function handleDelete(event) {
    const { message } = event.detail;
    
    if (finalHandlers.onDelete) {
      try {
        await finalHandlers.onDelete(message.id);
        announce($_('chat.deleted'));
      } catch (err) {
        console.error('Erro ao excluir:', err);
        if (finalHandlers.onError) finalHandlers.onError(err);
      }
    }
    
    dispatch('delete', event.detail);
  }
  
  // --- Pin ---
  async function handlePin(event) {
    const { message, pinned } = event.detail;
    
    if (finalHandlers.onPin) {
      try {
        await finalHandlers.onPin(message.id, pinned);
        announce(pinned ? $_('chat.pin') : $_('chat.unpin'));
      } catch (err) {
        console.error('Erro ao fixar:', err);
        if (finalHandlers.onError) finalHandlers.onError(err);
      }
    }
    
    dispatch('pin', event.detail);
  }
  
  // --- Resend ---
  async function handleResend(event) {
    const { message } = event.detail;
    
    if (finalHandlers.onResend) {
      try {
        await finalHandlers.onResend(message);
      } catch (err) {
        console.error('Erro ao reenviar:', err);
        if (finalHandlers.onError) finalHandlers.onError(err);
      }
    }
    
    dispatch('resend', event.detail);
  }
  
  // --- Edit ---
  function handleEditStart(event) {
    const { message, index } = event.detail;
    editingIndex = index;
    editContent = message?.content || '';
    dispatch('editStart', event.detail);
  }
  
  async function handleEditSave(event) {
    const { message, newContent } = event.detail;
    
    if (finalHandlers.onEdit) {
      try {
        await finalHandlers.onEdit(message.id, newContent);
        announce($_('chat.saved'));
      } catch (err) {
        console.error('Erro ao salvar:', err);
        if (finalHandlers.onError) finalHandlers.onError(err);
      }
    }
    
    editingIndex = -1;
    editContent = '';
    dispatch('editSave', event.detail);
  }
  
  function handleEditCancel(event) {
    editingIndex = -1;
    editContent = '';
    dispatch('editCancel', event.detail);
  }
  
  // --- Detail Modal ---
  // 100% externo - apenas propaga evento
  function handleDetail(event) {
    dispatch('detail', event.detail);
  }
  
  // --- Image Zoom ---
  // 100% externo - apenas propaga evento
  function handleImageZoom(event) {
    dispatch('imageZoom', event.detail);
  }
  
  // --- Context Menu ---
  // --- Context Menu ---
  // 100% externo - apenas propaga evento com dados
  // Quem usa decide como montar o menu
  function handleContextMenu(event) {
    const { event: domEvent, message, index, level } = event.detail;
    
    // Calcula posição sugerida
    const x = domEvent?.clientX || domEvent?.currentTarget?.getBoundingClientRect().left || 0;
    const y = domEvent?.clientY || domEvent?.currentTarget?.getBoundingClientRect().bottom || 0;
    
    // Apenas propaga - quem usa decide o que fazer
    dispatch('contextMenu', {
      event: domEvent,
      message,
      index,
      level,
      x,
      y,
    });
  }
  
  // --- Announce (Acessibilidade) ---
  function handleAnnounce(event) {
    announce(event.detail.message, event.detail.priority);
  }
  
  function announce(message, priority = 'polite') {
    if (liveRegion) {
      liveRegion.textContent = '';
      setTimeout(() => {
        liveRegion.textContent = message;
      }, 50);
    }
    
    if (finalHandlers.onAnnounce) {
      finalHandlers.onAnnounce(message, priority);
    }
  }
  
  // --- Hover/Focus ---
  function handleHover(event) {
    hoveredIndex = event.detail.hovered ? event.detail.index : -1;
  }
  
  function handleFocus(event) {
    focusedIndex = event.detail.index;
  }
  
  // --- Boundary (navegação atingiu limite) ---
  function handleBoundary(event) {
    const { edge, level } = event.detail;
    
    // Navegação interna: fim das mensagens → foca no input
    if (level === 0 && edge === 'end') {
      navigation.focusInput();
    }
    
    // Sempre propaga o evento para quem quiser tratar adicionalmente
    dispatch('boundary', event.detail);
  }
  
  // --- Media ---
  function handleMediaAdd(event) {
    pendingMedia = [...pendingMedia, event.detail];
    dispatch('mediaAdd', event.detail);
  }
  
  function handleMediaRemove(event) {
    const idx = event.detail.index ?? event.detail;
    pendingMedia = pendingMedia.filter((_, i) => i !== idx);
    dispatch('mediaRemove', event.detail);
  }
  
  // --- Clear Error ---
  function handleClearError() {
    error = '';
    dispatch('clearError');
  }
  
  // ========================================
  // Métodos Públicos
  // ========================================
  
  export function scrollToBottom() {
    if (chatHistoryRef?.scrollToBottom) {
      chatHistoryRef.scrollToBottom();
    }
  }
  
  export function focusInput() {
    // Foca no input
    const input = document.querySelector('#message-input');
    if (input) input.focus();
  }
</script>

<div class="chat-container">
  <!-- Aria-live region para anúncios -->
  <div 
    bind:this={liveRegion}
    class="sr-only"
    role="status"
    aria-live="polite"
    aria-atomic="true"
  ></div>
  
  <!-- Slot: header (toolbar customizada fica FORA, aqui é só um slot opcional) -->
  <slot name="header" />
  
  <!-- Indicador de Streaming -->
  {#if isStreaming}
    <div class="streaming-bar">
      <StreamingIndicator 
        text={streamingText} 
        showCancel={true}
        on:cancel={() => {
          finalHandlers.onStreamCancel?.();
          dispatch('streamCancel');
        }}
      />
    </div>
  {/if}
  
  <!-- Slot: before-messages -->
  <slot name="before-messages" />
  
  <!-- Histórico de Mensagens - Pode ser substituído via slot -->
  <slot name="messages-area"
    {messages}
    threadedMessages={threadedMessages.length > 0 ? threadedMessages : messages}
    {showInternalMessages}
    expandedPaths={effectiveExpandedPaths}
    loadingPaths={effectiveLoadingPaths}
    {selectedModel}
    {isTTSDisabled}
    {isLoading}
    {isTyping}
    {error}
  >
    <!-- Default: usa ChatHistory interno -->
    <ChatHistory
      bind:this={chatHistoryRef}
      messages={messages}
      threadedMessages={threadedMessages.length > 0 ? threadedMessages : messages}
      {showInternalMessages}
      expandedPaths={effectiveExpandedPaths}
      loadingPaths={effectiveLoadingPaths}
      {selectedModel}
      hoveredMessageIndex={effectiveHoveredIndex}
      focusedMessageIndex={effectiveFocusedIndex}
      editingMessageIndex={effectiveEditingIndex}
      editingMessageContent={effectiveEditContent}
      {isTTSDisabled}
      {isLoading}
      {isTyping}
      {error}
      config={historyConfig}
      on:toggle={handleToggle}
      on:hover={handleHover}
      on:focus={handleFocus}
      on:boundary={handleBoundary}
      on:keyAction
      on:speak={handleSpeak}
      on:copy={handleCopy}
      on:delete={handleDelete}
      on:pin={handlePin}
      on:resend={handleResend}
      on:detail={handleDetail}
      on:editStart={handleEditStart}
      on:editSave={handleEditSave}
      on:editCancel={handleEditCancel}
      on:announce={handleAnnounce}
      on:contextMenu={handleContextMenu}
      on:loadChildren={handleLoadChildren}
      on:imageZoom={handleImageZoom}
      on:imageDownload
      on:imageCopy
      on:mediaClick
      on:copyCode
      on:copyTable
      on:openLink
      on:clearError={handleClearError}
    >
      <slot name="empty" slot="empty" />
      <slot name="loading" slot="loading" />
    </ChatHistory>
  </slot>
  
  <!-- Slot: after-messages -->
  <slot name="after-messages" />
  
  <!-- Input Area - Pode ser substituída completamente via slot -->
  <slot name="input-area"
    inputMessage={effectiveInputValue}
    pendingMedia={effectivePendingMedia}
    {disabled}
    {isLoading}
    {isGeneratingAltText}
    {canSendMessage}
    {isDragging}
    {mediaMode}
    {voiceEnabled}
    {showVoiceButton}
    {isRecording}
    {placeholder}
    {mediaError}
    {MEDIA_CATEGORIES}
    {hintText}
    onSubmit={handleSubmit}
    onMediaRemove={handleMediaRemove}
  >
    <!-- Default: usa ChatInput interno -->
    <div class="input-wrapper">
      <ChatInput
        inputMessage={effectiveInputValue}
        pendingMedia={effectivePendingMedia}
        {mediaError}
        placeholder={placeholder || $_('chat.placeholder')}
        {disabled}
        {isLoading}
        {isGeneratingAltText}
        {canSendMessage}
        {isDragging}
        {mediaMode}
        {voiceEnabled}
        {showVoiceButton}
        {isRecording}
        {MEDIA_CATEGORIES}
        {hintText}
        on:submit={handleSubmit}
        on:keydown
        on:paste
        on:dragenter
        on:dragover
        on:dragleave
        on:drop
        on:removeMedia={handleMediaRemove}
        on:clearMediaError={() => dispatch('clearMediaError')}
        on:typing={(e) => {
          if (inputMessage === undefined) {
            _inputValue = e.detail.value || _inputValue;
          }
          dispatch('typing', e.detail);
        }}
        on:focus
        on:blur
        on:recordStart
        on:recordStop
        on:recordCancel
      >
        <slot name="input-prefix" slot="prefix" />
        <slot name="input-buttons" slot="buttons" {isRecording} />
        <slot name="input-suffix" slot="suffix" />
      </ChatInput>
    </div>
  </slot>
  
  <!-- Slot: footer (para qualquer coisa extra) -->
  <slot name="footer" />
</div>

<style>
  .chat-container {
    display: flex;
    flex-direction: column;
    height: 100%;
    background: var(--chat-color-bg);
    font-family: var(--chat-font-family);
    font-size: var(--chat-font-size-base);
    line-height: var(--chat-line-height);
    color: var(--chat-color-text);
  }
  
  .streaming-bar {
    padding: var(--chat-space-1) var(--chat-space-4);
    background: rgba(13, 110, 253, 0.1);
    border-bottom: 1px solid var(--chat-color-border);
  }
  
  .input-wrapper {
    display: flex;
    flex-direction: column;
    border-top: 1px solid var(--chat-color-border);
    background: var(--chat-color-bg);
  }
  
  /* Screen reader only */
  .sr-only {
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
</style>
