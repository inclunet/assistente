<script>
  import { createEventDispatcher, tick } from 'svelte';
  import { get } from 'svelte/store';
  import MessageContent from './MessageContent.svelte';
  import MessageHeader from './MessageHeader.svelte';
  import MessageActions from './MessageActions.svelte';
  import { _ } from 'svelte-i18n';
  
  // Helper para usar i18n dentro de funções normais
  const t = (key) => get(_)(key);
  
  // Props - Dados
  export let node;                    // MessageNode do backend
  export let level = 0;               // Nível de profundidade
  export let path = '';               // Caminho para identificar (ex: "0", "0-1")
  export let siblingCount = 1;        // Total de irmãos
  export let siblingIndex = 0;        // Índice entre irmãos
  export let parentPath = '';         // Path do pai para voltar
  
  // Props - Estado de expansão (controlado pelo parent)
  export let expandedPaths = {};
  export let loadingPaths = {};       // Paths em loading
  
  // Props - Funcionalidades (controlado por nível)
  export let showHoverActions = false;  // Mostrar ações no hover (nível 0)
  export let editable = false;          // Permitir edição (nível 0)
  export let deletable = false;         // Permitir exclusão
  export let pinnable = false;          // Permitir fixar
  export let speakable = true;          // Permitir TTS
  export let showContextMenu = false;   // Menu de contexto (nível 0)
  export let isHovered = false;         // Estado de hover (nível 0)
  export let isFocused = false;         // Estado de foco (nível 0)
  export let isEditing = false;         // Modo edição (nível 0)
  export let editContent = '';          // Conteúdo sendo editado
  export let isPinned = false;          // Mensagem fixada
  export let isTTSDisabled = true;      // TTS desativado
  export let truncateContent = 0;       // Truncar conteúdo (0 = não truncar)
  
  const dispatch = createEventDispatcher();
  
  // Dados da mensagem
  $: message = node?.message || {};
  $: children = node?.children || [];
  $: childCount = node?.childCount || children.length || 0;
  $: hasChildren = childCount > 0;
  $: isExpanded = !!expandedPaths[path];
  $: isLoading = !!loadingPaths[path];
  
  // Formata nome do agente (snake_case -> Title Case)
  function formatAgentName(name) {
    if (!name) return t('chat.agent');
    return name.split('_').map(word => 
      word.charAt(0).toUpperCase() + word.slice(1)
    ).join(' ');
  }
  
  // Gera label para acessibilidade - conteúdo completo para leitores de tela
  function getLabel() {
    const role = message.role;
    const agentName = message.agent_name || message.agentName || t('chat.agent');
    const content = message.content || '';
    const position = `${siblingIndex + 1} ${t('chat.messageOf').replace('{n}', siblingIndex + 1).replace('{total}', siblingCount)}`;
    
    if (level === 0) {
      if (role === 'user') {
        return `${t('chat.you')}: ${content}. ${position}.${hasChildren ? ` ${childCount} ${t('chat.interactions', { values: { count: childCount } })}.` : ''}`;
      }
      return `${t('chat.assistant')}: ${content}. ${position}.${hasChildren ? ` ${childCount} ${t('chat.interactions', { values: { count: childCount } })}.` : ''}`;
    }
    
    // Níveis internos
    if (role === 'assistant') {
      const label = `${t('chat.assistant')} → ${formatAgentName(agentName)}: ${content}. ${position}.`;
      return hasChildren ? `${label} ${childCount} ${t('chat.interactions', { values: { count: childCount } })}.` : label;
    }
    if (role === 'agent') {
      return `${formatAgentName(agentName)}: ${content}. ${position}.`;
    }
    if (role === 'tool') {
      const toolName = message.toolName || message.tool_name || agentName;
      return `${t('chat.tool')} ${formatAgentName(toolName)}: ${content}. ${position}.`;
    }
    return `${position}. ${content}`;
  }
  
  // Ícone baseado no role e nível
  function getIcon() {
    const role = message.role;
    if (level === 0) {
      return role === 'user' ? '👤' : '🤖';
    }
    if (role === 'assistant') return '🤖';
    if (role === 'agent') return '🔧';
    if (role === 'tool') return '📥';
    return '💬';
  }
  
  // Título baseado no role
  function getTitle() {
    const role = message.role;
    const agentName = message.agent_name || message.agentName;
    
    if (level === 0) {
      if (message.internal) {
        if (role === 'tool') return `🔧 ${message.toolName || agentName || 'Tool'}:`;
        if (message.toolCalls) return `🤖 ${t('chat.calling')}: ${getToolCallNames()}`;
        return `🔧 ${agentName || 'Internal'}:`;
      }
      return role === 'user' ? `${t('chat.you')}:` : `${t('chat.assistant')}:`;
    }
    
    if (role === 'assistant') {
      return `${t('chat.assistant')} → ${formatAgentName(agentName)}:`;
    }
    if (role === 'agent') {
      return `${formatAgentName(agentName)}:`;
    }
    if (role === 'tool') {
      return `📥 ${formatAgentName(message.toolName || message.tool_name || agentName)}:`;
    }
    return `${t('chat.system')}:`;
  }
  
  function getToolCallNames() {
    const tc = message.toolCalls;
    if (!tc) return '';
    const calls = typeof tc === 'string' ? JSON.parse(tc) : tc;
    return calls.map(c => c.function?.name || c.Function?.Name || '?').join(', ');
  }
  
  // ==================== EVENT DISPATCHERS ====================
  // Todas as ações disparam eventos - NENHUM callback
  
  function emitSpeak() {
    dispatch('speak', { message, text: message.content });
  }
  
  function emitCopy(format = 'text') {
    dispatch('copy', { message, format });
  }
  
  function emitDelete() {
    dispatch('delete', { message, index: siblingIndex });
  }
  
  function emitPin() {
    dispatch('pin', { message, pinned: !isPinned });
  }
  
  function emitResend() {
    dispatch('resend', { message });
  }
  
  function emitDetail() {
    dispatch('detail', { message, index: siblingIndex, path });
  }
  
  function emitEditStart() {
    dispatch('editStart', { message, index: siblingIndex, path });
  }
  
  function emitEditSave() {
    dispatch('editSave', { message, newContent: editContent, index: siblingIndex });
  }
  
  function emitEditCancel() {
    dispatch('editCancel', { message, index: siblingIndex });
  }
  
  function emitAnnounce(text, priority = 'polite') {
    dispatch('announce', { message: text, priority });
  }
  
  function emitContextMenu(svelteEvent) {
    // Pode vir de um evento DOM direto ou de um CustomEvent do Svelte (MessageActions)
    const domEvent = svelteEvent?.detail?.event || svelteEvent;
    dispatch('contextMenu', { 
      event: domEvent, 
      message, 
      index: siblingIndex, 
      level,
      path 
    });
  }
  
  function emitLoadChildren() {
    const messageId = message.id || message.ID;
    if (messageId) {
      dispatch('loadChildren', { messageId, path });
    }
  }
  
  // ==================== KEYBOARD NAVIGATION ====================
  // Navegação é tratada internamente (setas, Home, End)
  // Ações são disparadas como eventos genéricos para o app decidir
  
  async function handleKeyDown(event) {
    const key = event.key;
    
    // === Tab/Shift+Tab: deixa o navegador lidar (foco entre elementos) ===
    if (key === 'Tab') {
      // Não previne, não dispara keyAction - deixa o comportamento padrão
      return;
    }
    
    // === Navegação (tratada internamente) ===
    if (key === 'ArrowDown') {
      event.preventDefault();
      event.stopPropagation();
      if (siblingIndex < siblingCount - 1) {
        focusSibling(siblingIndex + 1);
      } else {
        dispatch('boundary', { edge: 'end', level, path });
      }
      return;
    }
    
    if (key === 'ArrowUp') {
      event.preventDefault();
      event.stopPropagation();
      if (siblingIndex > 0) {
        focusSibling(siblingIndex - 1);
      } else {
        dispatch('boundary', { edge: 'start', level, path });
      }
      return;
    }
    
    if (key === 'ArrowRight') {
      event.preventDefault();
      event.stopPropagation();
      if (hasChildren) {
        await expandAndFocusFirst();
      }
      return;
    }
    
    if (key === 'ArrowLeft' || key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      if (isExpanded) {
        dispatch('toggle', { path, expand: false });
        emitAnnounce(t('chat.collapse'));
      } else if (level > 0) {
        focusParent();
      }
      return;
    }
    
    if (key === 'Home') {
      event.preventDefault();
      event.stopPropagation();
      focusSibling(0);
      return;
    }
    
    if (key === 'End') {
      event.preventDefault();
      event.stopPropagation();
      focusSibling(siblingCount - 1);
      return;
    }
    
    // === Ações - dispara evento genérico por tecla ===
    // O app decide o que fazer com cada tecla
    
    // Monta o identificador da tecla (ex: "Ctrl+C", "Shift+Enter", "Space", "e")
    const keyId = getKeyIdentifier(event);
    
    // Dispara evento genérico - o app decide a ação
    dispatch('keyAction', {
      key: keyId,
      originalKey: key,
      ctrlKey: event.ctrlKey,
      shiftKey: event.shiftKey,
      altKey: event.altKey,
      metaKey: event.metaKey,
      message,
      index: siblingIndex,
      level,
      path,
      originalEvent: event,
    });
    
    // Não previne default aqui - o app decide se quer prevenir
  }
  
  // Gera identificador legível da tecla (ex: "Ctrl+Shift+C", "Enter", "Space")
  function getKeyIdentifier(event) {
    const parts = [];
    if (event.ctrlKey || event.metaKey) parts.push('Ctrl');
    if (event.shiftKey) parts.push('Shift');
    if (event.altKey) parts.push('Alt');
    
    // Normaliza nome da tecla
    let keyName = event.key;
    if (keyName === ' ') keyName = 'Space';
    if (keyName.length === 1) keyName = keyName.toUpperCase();
    
    // Não duplica modificadores
    if (!['Control', 'Shift', 'Alt', 'Meta'].includes(keyName)) {
      parts.push(keyName);
    }
    
    return parts.join('+') || keyName;
  }
  
  function focusSibling(idx) {
    const siblingPath = level === 0 ? String(idx) : (() => {
      const parts = path.split('-');
      parts[parts.length - 1] = idx.toString();
      return parts.join('-');
    })();
    
    const el = document.querySelector(`[data-message-path="${siblingPath}"]`);
    if (el) el.focus();
  }
  
  function focusParent() {
    if (parentPath) {
      const parentEl = document.querySelector(`[data-message-path="${parentPath}"]`);
      if (parentEl) {
        parentEl.focus();
        return;
      }
    }
    dispatch('focusRoot');
  }
  
  async function expandAndFocusFirst() {
    if (!hasChildren) return;
    
    if (!isExpanded) {
      emitAnnounce(`${t('chat.loading')} ${childCount} ${t('chat.interactions', { values: { count: childCount } })}...`);
      
      // Solicita carregamento de filhos se necessário
      if (children.length === 0 && childCount > 0) {
        emitLoadChildren();
      }
      
      dispatch('toggle', { path, expand: true });
      
      await tick();
      setTimeout(() => {
        const firstChildPath = `${path}-0`;
        const firstChild = document.querySelector(`[data-message-path="${firstChildPath}"]`);
        if (firstChild) {
          firstChild.focus();
        }
      }, 150);
    } else {
      const firstChildPath = `${path}-0`;
      const firstChild = document.querySelector(`[data-message-path="${firstChildPath}"]`);
      if (firstChild) firstChild.focus();
    }
  }
  
  async function handleToggleClick() {
    if (!hasChildren) return;
    
    if (!isExpanded && children.length === 0 && childCount > 0) {
      emitLoadChildren();
    }
    
    dispatch('toggle', { path, expand: !isExpanded });
  }
  
  // ==================== CHILD EVENT HANDLERS ====================
  // Propagam eventos dos filhos para cima
  
  function handleChildEvent(event) {
    // Propaga qualquer evento do filho
    dispatch(event.type, event.detail);
  }
  
  function handleContextMenuEvent(event) {
    if (showContextMenu) {
      event.preventDefault();
      event.stopPropagation();
      emitContextMenu(event);
    }
  }
  
  // ==================== CONTENT EVENTS ====================
  // Eventos disparados pelo MessageContent
  
  function handleImageZoom(event) {
    dispatch('imageZoom', event.detail);
  }
  
  function handleImageDownload(event) {
    dispatch('imageDownload', event.detail);
  }
  
  function handleImageCopy(event) {
    dispatch('imageCopy', event.detail);
  }
  
  function handleMediaClick(event) {
    dispatch('mediaClick', event.detail);
  }
  
  function handleCopyCode(event) {
    dispatch('copyCode', event.detail);
  }
  
  function handleCopyTable(event) {
    dispatch('copyTable', event.detail);
  }
  
  function handleOpenLink(event) {
    dispatch('openLink', event.detail);
  }
  
  // CSS classes
  $: cssClasses = [
    'message-node',
    `level-${level}`,
    message.role || 'unknown',
    isExpanded ? 'expanded' : '',
    hasChildren ? 'has-children' : '',
    isHovered && level === 0 ? 'hovered' : '',
    isPinned ? 'pinned' : '',
    message.internal ? 'internal' : '',
    message.isStreaming ? 'streaming' : ''
  ].filter(Boolean).join(' ');
</script>

{#if node}
<li 
  class={cssClasses}
  tabindex={isFocused ? 0 : -1}
  role="listitem"
  aria-label={getLabel()}
  data-message-path={path}
  data-level={level}
  aria-hidden={!isTTSDisabled && message.isStreaming ? 'true' : undefined}
  on:keydown={handleKeyDown}
  on:contextmenu={handleContextMenuEvent}
  on:mouseenter={() => level === 0 && dispatch('hover', { index: siblingIndex, hovered: true })}
  on:mouseleave={() => level === 0 && dispatch('hover', { index: siblingIndex, hovered: false })}
  on:focus={() => dispatch('focus', { index: siblingIndex, path })}
>
  <!-- Slot: avatar (customização do avatar) -->
  <slot name="avatar">
    <!-- Avatar padrão -->
  </slot>
  
  <!-- Slot: actions (ações customizadas) - ou ações padrão -->
  <slot name="actions" message={message} {isHovered} {isTTSDisabled}>
    <MessageActions
      {isHovered}
      isStreaming={message.isStreaming}
      {isTTSDisabled}
      {level}
      {showHoverActions}
      {speakable}
      on:speak={emitSpeak}
      on:copy={() => emitCopy()}
      on:contextMenu={emitContextMenu}
    />
  </slot>
  
  <!-- Slot: header (header customizado) - ou header padrão -->
  <slot name="header" {message} {level} {isPinned} {hasChildren} {childCount} {isExpanded} {isLoading}>
    <MessageHeader
      {message}
      {level}
      {isPinned}
      {hasChildren}
      {childCount}
      {isExpanded}
      {isLoading}
      on:toggle={handleToggleClick}
    />
  </slot>
  
  <!-- Slot: content (conteúdo customizado) - ou conteúdo padrão -->
  <slot name="content" {message} {isEditing} {editContent}>
    <div class="node-body">
      {#if isEditing && editable}
        <!-- Modo edição -->
        <div class="edit-container">
          <textarea
            class="edit-input"
            bind:value={editContent}
            on:keydown={(e) => {
              if (e.key === 'Enter' && e.ctrlKey) emitEditSave();
              if (e.key === 'Escape') emitEditCancel();
            }}
            rows="3"
            aria-label={$_('chat.editPlaceholder')}
          ></textarea>
          <div class="edit-actions">
            <button class="btn-primary btn-sm" on:click={emitEditSave}>{$_('chat.saveEdit')}</button>
            <button class="btn-secondary btn-sm" on:click={emitEditCancel}>{$_('chat.cancelEdit')}</button>
          </div>
        </div>
      {:else}
        <MessageContent
          content={message.content}
          media={message.media}
          isStreaming={message.isStreaming}
          toolsInfo={message.toolsInfo}
          truncate={truncateContent}
          on:imageDownload={handleImageDownload}
          on:imageCopy={handleImageCopy}
          on:imageZoom={handleImageZoom}
          on:mediaClick={handleMediaClick}
          on:copyCode={handleCopyCode}
          on:copyTable={handleCopyTable}
          on:openLink={handleOpenLink}
        />
      {/if}
    </div>
  </slot>
  
  <!-- Tool calls info -->
  {#if message.toolCalls && level > 0}
    {@const tcalls = typeof message.toolCalls === 'string' ? JSON.parse(message.toolCalls) : message.toolCalls}
    {#if tcalls && tcalls.length > 0}
      <div class="node-tool-calls" aria-hidden="true">
        <span class="tool-call-info">
          🔧 {$_('chat.calling')}: {tcalls.map(tc => tc.function?.name || tc.Function?.Name || '?').join(', ')}
        </span>
      </div>
    {/if}
  {/if}
  
  <!-- Slot: footer (rodapé customizado) -->
  <slot name="footer" {message} {level}>
    <!-- Footer padrão vazio -->
  </slot>
  
  <!-- Filhos (recursivo) -->
  {#if isExpanded && children.length > 0}
    <ul class="node-children" role="list" aria-label={`${$_('chat.interactions', { values: { count: children.length } })} ${$_('chat.level')} ${level + 1}`}>
      {#each children as child, childIdx (child.message?.id || child.message?.ID || childIdx)}
        <svelte:self
          node={child}
          level={level + 1}
          path={`${path}-${childIdx}`}
          siblingCount={children.length}
          siblingIndex={childIdx}
          parentPath={path}
          {expandedPaths}
          {loadingPaths}
          truncateContent={500}
          showContextMenu={true}
          showHoverActions={true}
          {speakable}
          {isTTSDisabled}
          on:toggle
          on:focusRoot
          on:boundary
          on:hover
          on:focus
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
</li>
{/if}

<style>
  .message-node {
    list-style: none;
    padding: var(--chat-message-padding, 0.75rem 1rem);
    margin: var(--chat-space-2) 0;
    border-radius: var(--chat-message-radius, 0.5rem);
    background: var(--chat-color-bg-secondary);
    border-left: 4px solid var(--chat-color-border);
    position: relative;
    transition: background-color var(--chat-transition-fast), border-color var(--chat-transition-fast);
    font-family: var(--chat-font-family);
  }
  
  .message-node:focus {
    outline: 2px solid var(--chat-color-border-focus);
    outline-offset: 2px;
  }
  
  /* Cores por role */
  .message-node.user {
    border-left-color: var(--chat-color-user-border);
    background: var(--chat-color-user-bg);
    color: var(--chat-color-user-text);
  }
  
  .message-node.assistant {
    border-left-color: var(--chat-color-assistant-border);
    background: var(--chat-color-assistant-bg);
    color: var(--chat-color-assistant-text);
  }
  
  .message-node.agent {
    border-left-color: var(--chat-color-agent-border);
    background: var(--chat-color-agent-bg);
    color: var(--chat-color-agent-text);
  }
  
  .message-node.tool {
    border-left-color: var(--chat-color-tool-border);
    background: var(--chat-color-tool-bg);
    color: var(--chat-color-tool-text);
  }
  
  .message-node.internal {
    opacity: 0.85;
    font-size: var(--chat-font-size-sm);
  }
  
  .message-node.pinned {
    border-left-color: var(--chat-color-warning);
  }
  
  .message-node.hovered {
    background: var(--chat-color-hover);
  }
  
  /* Níveis de profundidade */
  .message-node.level-1,
  .message-node.level-2,
  .message-node.level-3,
  .message-node.level-4,
  .message-node.level-5 {
    margin-left: 1.5rem;
    padding: 0.5rem 0.75rem;
  }
  
  .message-node.level-2,
  .message-node.level-3,
  .message-node.level-4,
  .message-node.level-5 {
    font-size: 0.95em;
  }
  
  /* Corpo */
  .node-body {
    margin-top: var(--chat-space-1);
  }
  
  /* Edição */
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
  }
  
  .btn-primary:hover {
    background: var(--chat-btn-primary-hover);
  }
  
  .btn-secondary {
    background: var(--chat-color-bg-tertiary);
    color: var(--chat-color-text);
    border: 1px solid var(--chat-color-border);
    border-radius: var(--chat-radius-md);
    cursor: pointer;
  }
  
  .btn-secondary:hover {
    background: var(--chat-color-hover);
  }
  
  /* Tool calls */
  .node-tool-calls {
    margin-top: var(--chat-space-2);
    padding: var(--chat-space-1) var(--chat-space-2);
    background: var(--chat-color-warning);
    border-radius: var(--chat-radius-sm);
    font-size: var(--chat-font-size-sm);
    opacity: 0.2;
  }
  
  .tool-call-info {
    color: inherit;
  }
  
  /* Filhos */
  .node-children {
    margin: var(--chat-space-2) 0 0 0;
    padding: 0;
  }
</style>
