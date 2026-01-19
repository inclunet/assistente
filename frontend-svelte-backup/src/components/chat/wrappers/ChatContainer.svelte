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
  export let autoFocusInput = false; // Prop para foco automático
  
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
  let _isDragging = false;
  
  // Use external props if provided, otherwise use internal state
  $: effectiveInputValue = inputMessage !== undefined ? inputMessage : _inputValue;
  $: effectivePendingMedia = pendingMedia !== undefined ? pendingMedia : _pendingMedia;
  $: effectiveExpandedPaths = expandedPaths !== undefined ? expandedPaths : _expandedPaths;
  $: effectiveLoadingPaths = loadingPaths !== undefined ? loadingPaths : _loadingPaths;
  $: effectiveHoveredIndex = hoveredMessageIndex !== undefined && hoveredMessageIndex !== -1 ? hoveredMessageIndex : _hoveredIndex;
  $: effectiveFocusedIndex = focusedMessageIndex !== undefined && focusedMessageIndex !== -1 ? focusedMessageIndex : _focusedIndex;
  $: effectiveEditingIndex = editingMessageIndex !== undefined && editingMessageIndex !== -1 ? editingMessageIndex : _editingIndex;
  $: effectiveEditContent = editingMessageContent !== undefined ? editingMessageContent : _editContent;
  $: effectiveIsDragging = isDragging !== undefined && isDragging !== false ? isDragging : _isDragging;

  // ========================================
  // Funções de Drag & Drop (Agnósticas)
  // ========================================

  /**
   * Handler para drag enter - ativa estado de dragging
   */
  function handleDragEnter(event) {
    event.preventDefault();
    event.stopPropagation();
    
    // Verifica se tem arquivos sendo arrastados
    if (event.dataTransfer?.types?.includes('Files')) {
      if (isDragging !== undefined) {
        dispatch('dragStateChange', { isDragging: true });
      } else {
        _isDragging = true;
      }
    }
  }

  /**
   * Handler para drag over (necessário para permitir drop)
   */
  function handleDragOver(event) {
    event.preventDefault();
    event.stopPropagation();
  }

  /**
   * Handler para drag leave - desativa estado de dragging
   */
  function handleDragLeave(event) {
    event.preventDefault();
    event.stopPropagation();
    
    // Só desativa se saiu da área de input (não de um filho)
    const rect = event.currentTarget?.getBoundingClientRect();
    if (!rect) return;
    
    const x = event.clientX;
    const y = event.clientY;
    
    if (x < rect.left || x > rect.right || y < rect.top || y > rect.bottom) {
      if (isDragging !== undefined) {
        dispatch('dragStateChange', { isDragging: false });
      } else {
        _isDragging = false;
      }
    }
  }

  /**
   * Handler para drop de arquivos
   * Extrai os arquivos e dispara evento para o host processar
   */
  async function handleFileDrop(event) {
    event.preventDefault();
    event.stopPropagation();
    
    // Desativa dragging
    if (isDragging !== undefined) {
      dispatch('dragStateChange', { isDragging: false });
    } else {
      _isDragging = false;
    }
    
    const files = event.dataTransfer?.files;
    if (!files || files.length === 0) return;
    
    // Converte FileList para Array
    const fileArray = Array.from(files);
    
    // Dispara evento com os arquivos para o host processar
    dispatch('filesDropped', { files: fileArray });
  }

  /**
   * Handler para paste de imagens/arquivos
   * Extrai os arquivos e dispara evento para o host processar
   */
  function handleFilePaste(event) {
    const items = event.clipboardData?.items;
    if (!items) return;
    
    const files = [];
    for (const item of items) {
      if (item.kind === 'file') {
        const file = item.getAsFile();
        if (file) files.push(file);
      }
    }
    
    if (files.length > 0) {
      event.preventDefault();
      dispatch('filesDropped', { files, source: 'paste' });
    }
  }

  // ========================================
  // Funções de Navegação de Threads (Agnósticas)
  // ========================================

  /**
   * Verifica se um path está expandido
   * @param {string} path - Ex: "0", "0-1", "0-1-2"
   * @returns {boolean}
   */
  function isPathExpanded(path) {
    return !!effectiveExpandedPaths[path];
  }

  /**
   * Toggle expansão de um path
   * @param {string} path
   * @param {boolean} [shouldExpand] - Se não passado, inverte o estado atual
   */
  function togglePath(path, shouldExpand) {
    if (typeof shouldExpand === 'undefined') {
      shouldExpand = !effectiveExpandedPaths[path];
    }
    
    if (expandedPaths !== undefined) {
      // Controlado externamente - dispara evento
      dispatch('pathToggle', { path, expand: shouldExpand });
    } else {
      // Controlado internamente
      if (shouldExpand) {
        _expandedPaths[path] = true;
      } else {
        delete _expandedPaths[path];
      }
      _expandedPaths = { ..._expandedPaths };
    }
  }

  /**
   * Encontra um node na árvore pelo path
   * Ex: "0" = primeiro root, "0-1" = segundo filho do primeiro root
   * @param {string} path
   * @returns {object|null}
   */
  function findNodeByPath(path) {
    const msgs = threadedMessages.length > 0 ? threadedMessages : messages;
    if (!path || !msgs?.length) return null;
    
    const indices = path.split('-').map(Number);
    let current = msgs[indices[0]];
    
    for (let i = 1; i < indices.length && current; i++) {
      current = current?.children?.[indices[i]];
    }
    
    return current;
  }

  /**
   * Handler genérico de expansão de thread
   * @param {string} path
   * @param {boolean} shouldExpand
   */
  async function handleNodeExpand(path, shouldExpand) {
    const node = findNodeByPath(path);
    
    if (shouldExpand && node) {
      // Verifica se precisa carregar filhos (lazy loading)
      const needsLoading = node.message?.id && 
                          (!node.children || node.children.length === 0);
      
      if (needsLoading) {
        // Marca como carregando
        if (loadingPaths !== undefined) {
          dispatch('loadingChange', { path, loading: true });
        } else {
          _loadingPaths[path] = true;
          _loadingPaths = { ..._loadingPaths };
        }
        
        // Dispara evento para o host carregar os filhos
        dispatch('loadChildren', { 
          messageId: node.message.id, 
          path,
          node
        });
        return; // O host vai chamar completeChildrenLoad quando terminar
      }
    }
    
    togglePath(path, shouldExpand);
    
    // Foca no primeiro filho após expandir
    if (shouldExpand) {
      await tick();
      const firstChildPath = `${path}-0`;
      const firstChild = document.querySelector(`[data-message-path="${firstChildPath}"]`);
      if (firstChild) {
        firstChild.focus();
      }
    }
  }

  /**
   * Chamado pelo host após carregar filhos com sucesso
   * @param {string} path
   * @param {boolean} success
   */
  export function completeChildrenLoad(path, success = true) {
    // Remove do loading
    if (loadingPaths !== undefined) {
      dispatch('loadingChange', { path, loading: false });
    } else {
      delete _loadingPaths[path];
      _loadingPaths = { ..._loadingPaths };
    }
    
    if (success) {
      togglePath(path, true);
      
      // Foca no primeiro filho - usa múltiplos frames para garantir renderização
      tick().then(() => {
        requestAnimationFrame(() => {
          requestAnimationFrame(() => {
            const firstChildPath = `${path}-0`;
            const firstChild = document.querySelector(`[data-message-path="${firstChildPath}"]`);
            if (firstChild) {
              firstChild.focus();
            }
          });
        });
      });
    }
  }
  
  // Menu de contexto e modais - 100% externos, apenas propagamos eventos
  
  // Referências
  let chatHistoryRef;
  let chatInputRef;
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

  // Removido: clones e logs temporários; reatividade agora depende dos stores unificados
  
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
  async function handleToggle(event) {
    const { path, expand } = event.detail;
    
    // Usa lógica interna de navegação de threads
    await handleNodeExpand(path, expand);
    
    // Propaga evento para quem quiser ouvir
    dispatch('toggle', event.detail);
  }
  
  // --- Load Children (lazy loading) ---
  // NOTA: Este evento agora é tratado internamente via handleNodeExpand
  // O evento on:loadChildren é disparado automaticamente quando lazy loading é necessário
  async function handleLoadChildren(event) {
    const { messageId, path, node } = event.detail;
    
    // Se tem handler externo, usa ele
    if (finalHandlers.onLoadChildren) {
      try {
        const children = await finalHandlers.onLoadChildren(messageId, node);
        dispatch('childrenLoaded', { messageId, path, children });
        completeChildrenLoad(path, true);
      } catch (err) {
        console.error('Erro ao carregar filhos:', err);
        completeChildrenLoad(path, false);
        if (finalHandlers.onError) {
          finalHandlers.onError(err);
        }
      }
      return;
    }
    
    // Caso contrário, apenas propaga o evento para o host tratar
    dispatch('loadChildren', event.detail);
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
    hoveredMessageIndex = event.detail.hovered ? event.detail.index : -1;
  }
  
  function handleFocus(event) {
    focusedMessageIndex = event.detail.index;
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
    // Foca no input usando a referência do componente
    if (chatInputRef?.focus) {
      chatInputRef.focus();
    }
  }

  /**
   * Expande uma thread pelo índice (nível 0)
   * @param {number} index
   */
  export async function expandThread(index) {
    const path = String(index);
    if (!isPathExpanded(path)) {
      await handleNodeExpand(path, true);
    }
  }

  /**
   * Recolhe uma thread pelo índice (nível 0)
   * @param {number} index
   */
  export function collapseThread(index) {
    const path = String(index);
    if (isPathExpanded(path)) {
      togglePath(path, false);
    }
  }

  /**
   * Verifica se uma thread está expandida
   * @param {number} index
   * @returns {boolean}
   */
  export function isThreadExpanded(index) {
    return isPathExpanded(String(index));
  }

  /**
   * Expande um path específico
   * @param {string} path
   */
  export async function expandPath(path) {
    if (!isPathExpanded(path)) {
      await handleNodeExpand(path, true);
    }
  }

  /**
   * Recolhe um path específico
   * @param {string} path
   */
  export function collapsePath(path) {
    if (isPathExpanded(path)) {
      togglePath(path, false);
    }
  }

  /**
   * Encontra um node pelo path (exposição pública)
   * @param {string} path
   * @returns {object|null}
   */
  export function getNodeByPath(path) {
    return findNodeByPath(path);
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
    messages={messages}
    threadedMessages={threadedMessages}
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
      threadedMessages={threadedMessages}
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
    isDragging={effectiveIsDragging}
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
    onDragEnter={handleDragEnter}
    onDragOver={handleDragOver}
    onDragLeave={handleDragLeave}
    onFileDrop={handleFileDrop}
    onFilePaste={handleFilePaste}
  >
    <!-- Default: usa ChatInput interno -->
    <div class="input-wrapper">
      <ChatInput
        bind:this={chatInputRef}
        autoFocus={autoFocusInput}
        inputMessage={effectiveInputValue}
        pendingMedia={effectivePendingMedia}
        {mediaError}
        placeholder={placeholder || $_('chat.placeholder')}
        {disabled}
        {isLoading}
        {isGeneratingAltText}
        {canSendMessage}
        isDragging={effectiveIsDragging}
        {mediaMode}
        {voiceEnabled}
        {showVoiceButton}
        {isRecording}
        {MEDIA_CATEGORIES}
        {hintText}
        on:submit={handleSubmit}
        on:keydown
        on:paste={(e) => handleFilePaste(e.detail?.event || e)}
        on:dragenter={(e) => handleDragEnter(e.detail?.event || e)}
        on:dragover={(e) => handleDragOver(e.detail?.event || e)}
        on:dragleave={(e) => handleDragLeave(e.detail?.event || e)}
        on:drop={(e) => handleFileDrop(e.detail?.event || e)}
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
