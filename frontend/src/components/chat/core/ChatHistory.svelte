<script>
  import { createEventDispatcher, getContext, onDestroy, tick } from 'svelte';
  import MessageNode from './message/MessageNode.svelte';
  import EmptyState from './feedback/EmptyState.svelte';
  import LoadingIndicator from './feedback/LoadingIndicator.svelte';
  import { _ } from 'svelte-i18n';
  import { CHAT_NAVIGATION_KEY } from '../context/navigation.js';
  
  const dispatch = createEventDispatcher();
  
  // Props - Dados
  export let messages = [];
  export let threadedMessages = [];
  export let showInternalMessages = false;
  
  export let expandedPaths = {};
  export let loadingPaths = {};
  export let selectedModel = '';
  export let error = '';
  
  // Props - Estado de interação
  export let hoveredMessageIndex = -1;
  export let focusedMessageIndex = -1;
  export let editingMessageIndex = -1;
  export let editingMessageContent = '';
  export let isTTSDisabled = true;
  export let isLoading = false;
  export let isTyping = false;
  
  // Props - Configuração
  export let config = {};
  $: editable = config.editable ?? true;
  $: deletable = config.deletable ?? true;
  $: pinnable = config.pinnable ?? true;
  $: speakable = config.speakable ?? true;
  $: showHoverActions = config.showHoverActions ?? true;
  $: autoScroll = config.autoScroll ?? true;
  $: lazyLoadChildren = config.lazyLoadChildren ?? true;
  
  // Referência do container
  let messagesContainer = null;
  
  // === Navigation Context ===
  const navigation = getContext(CHAT_NAVIGATION_KEY);
  
  // Reage a solicitações de foco via contexto
  let unsubscribe;
  if (navigation) {
    unsubscribe = navigation.subscribe(async (state) => {
      if (state.focusTarget === 'lastMessage' || state.focusTarget === 'firstMessage') {
        await tick(); // Aguarda DOM atualizar
        const items = messagesContainer?.querySelectorAll(':scope > .messages-list > li');
        if (items && items.length > 0) {
          const targetIndex = state.focusTarget === 'lastMessage' 
            ? items.length - 1 
            : 0;
          items[targetIndex]?.focus();
          focusedMessageIndex = targetIndex;
          dispatch('focus', { index: targetIndex });
        }
        navigation.clearFocusTarget();
      } else if (state.focusTarget === 'message' && state.focusData?.index !== undefined) {
        await tick();
        const items = messagesContainer?.querySelectorAll(':scope > .messages-list > li');
        const targetItem = items?.[state.focusData.index];
        if (targetItem) {
          targetItem.focus();
          focusedMessageIndex = state.focusData.index;
          dispatch('focus', { index: state.focusData.index });
        }
        navigation.clearFocusTarget();
      }
    });
  }
  
  onDestroy(() => {
    unsubscribe?.();
  });
  
  // Expor referência do container
  export function getContainer() {
    return messagesContainer;
  }
  
  // Scroll para baixo
  export function scrollToBottom() {
    if (messagesContainer && autoScroll) {
      messagesContainer.scrollTop = messagesContainer.scrollHeight;
    }
  }
  
  const toNode = (message, index, level = 0) => ({
    message,
    children: [],
    childCount: 0,
    level,
    originalIndex: index
  });

  const ensureNode = (node, index, level = 0) => {
    if (!node) return null;
    if (node.message) {
      const normalizedChildren = Array.isArray(node.children)
        ? node.children
            .map((child, childIndex) => ensureNode(child, childIndex, level + 1))
            .filter(Boolean)
        : [];
      return {
        ...node,
        level: node.level ?? level,
        originalIndex: node.originalIndex ?? index,
        children: normalizedChildren,
        childCount: node.childCount ?? normalizedChildren.length
      };
    }
    return toNode(node, index, level);
  };

  $: normalizedThreads = Array.isArray(threadedMessages)
    ? threadedMessages
        .map((node, index) => ensureNode(node, index))
        .filter(Boolean)
    : [];

  $: baseNodes = normalizedThreads.length > 0
    ? normalizedThreads
    : (messages || []).map((message, index) => ensureNode(message, index) || toNode(message, index));

  // Lista de nodes a exibir
  $: displayNodes = showInternalMessages 
    ? baseNodes 
    : baseNodes.filter(n => !n.message?.internal);
  
  // Verifica se está vazio
  $: isEmpty = displayNodes.length === 0 && !isLoading;
</script>

<div 
  class="messages-container" 
  bind:this={messagesContainer}
  aria-busy={isLoading ? 'true' : undefined}
>
  <!-- Slot: header (conteúdo antes das mensagens) -->
  <slot name="header">
    <!-- Header padrão vazio -->
  </slot>
  
  <!-- Slot: loading (indicador de carregamento customizado) -->
  {#if isLoading && displayNodes.length === 0}
    <slot name="loading">
      <LoadingIndicator text={$_('chat.loading')} />
    </slot>
  {/if}
  
  <!-- Slot: empty (estado vazio customizado) -->
  {#if isEmpty}
    <slot name="empty" {selectedModel}>
      <EmptyState {selectedModel} />
    </slot>
  {:else}
    <ul 
      class="messages-list" 
      role="list" 
      aria-label={$_('chat.messageHistory') || 'Message history'}
    >
      {#each displayNodes as node, nodeIndex (node.message?.id || nodeIndex)}
        <MessageNode
          {node}
          level={0}
          path={String(node.originalIndex ?? nodeIndex)}
          siblingCount={displayNodes.length}
          siblingIndex={nodeIndex}
          parentPath=""
          {expandedPaths}
          {loadingPaths}
          {showHoverActions}
          lazyLoadChildren={lazyLoadChildren}
          editable={editable && node.message?.role === 'user'}
          {deletable}
          {pinnable}
          {speakable}
          showContextMenu={true}
          isHovered={hoveredMessageIndex === (node.originalIndex ?? nodeIndex)}
          isFocused={focusedMessageIndex === nodeIndex || (focusedMessageIndex < 0 && nodeIndex === 0)}
          isEditing={editingMessageIndex === (node.originalIndex ?? nodeIndex)}
          editContent={editingMessageContent}
          isPinned={node.message?.pinned || node.isPinned}
          {isTTSDisabled}
          on:toggle
          on:hover
          on:focus
          on:focusRoot
          on:boundary
          on:keyAction
          on:speak
          on:copy
          on:delete
          on:pin
          on:resend
          on:detail
          on:editStart
          on:editSave
          on:editCancel
          on:announce
          on:contextMenu
          on:loadChildren
          on:imageZoom
          on:imageDownload
          on:imageCopy
          on:mediaClick
          on:copyCode
          on:copyTable
          on:openLink
        />
      {/each}
    </ul>
  {/if}

  <!-- Error display -->
  {#if error}
    <div class="error-message" role="alert">
      <strong>{$_('chat.error')}:</strong> {error}
      <button 
        class="btn-secondary retry-btn"
        on:click={() => dispatch('clearError')}
      >
        {$_('chat.resend')}
      </button>
    </div>
  {/if}
  
  <!-- Slot: footer (conteúdo após as mensagens) -->
  <slot name="footer">
    <!-- Footer padrão vazio -->
  </slot>
</div>

<style>
  .messages-container {
    flex: 1;
    overflow-y: auto;
    padding: var(--chat-space-4);
    scroll-behavior: smooth;
    background: var(--chat-color-bg);
  }
  
  .messages-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--chat-messages-gap);
  }
  
  .error-message {
    display: flex;
    align-items: center;
    gap: var(--chat-space-4);
    padding: var(--chat-space-4);
    margin: var(--chat-space-4) 0;
    background: rgba(220, 53, 69, 0.1);
    color: var(--chat-color-error);
    border-radius: var(--chat-radius-lg);
    border-left: 4px solid var(--chat-color-error);
  }
  
  .retry-btn {
    margin-left: auto;
    padding: var(--chat-space-2) var(--chat-space-4);
    background: var(--chat-color-bg-tertiary);
    border: 1px solid var(--chat-color-border);
    border-radius: var(--chat-radius-sm);
    color: var(--chat-color-text);
    cursor: pointer;
    transition: background-color var(--chat-transition-fast);
  }
  
  .retry-btn:hover {
    background: var(--chat-color-hover);
  }
  
  .retry-btn:focus-visible {
    outline: 2px solid var(--chat-color-border-focus);
    outline-offset: 2px;
  }
</style>
