<script>
  import { createEventDispatcher } from 'svelte';
  import { get } from 'svelte/store';
  import { _ } from 'svelte-i18n';
  
  // Props - Dados (vindos do MessageNode)
  export let message = {};
  export let level = 0;
  export let isPinned = false;
  export let hasChildren = false;
  export let childCount = 0;
  export let isExpanded = false;
  export let isLoading = false;
  
  const dispatch = createEventDispatcher();
  
  // Helper para usar i18n dentro de funções normais
  const t = (key) => get(_)(key);
  
  // Formata nome do agente (snake_case -> Title Case)
  function formatAgentName(name) {
    if (!name) return t('chat.agent');
    return name.split('_').map(word => 
      word.charAt(0).toUpperCase() + word.slice(1)
    ).join(' ');
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
        if (message.toolCalls) return `🤖 Calling: ${getToolCallNames()}`;
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
    return 'Mensagem:';
  }
  
  function getToolCallNames() {
    const tc = message.toolCalls;
    if (!tc) return '';
    const calls = typeof tc === 'string' ? JSON.parse(tc) : tc;
    return calls.map(c => c.function?.name || c.Function?.Name || '?').join(', ');
  }
  
  function handleToggleClick(event) {
    event.stopPropagation();
    dispatch('toggle');
  }
</script>

<div class="node-header" aria-hidden="true">
  {#if isPinned}
    <span class="pin-indicator" aria-label={$_('chat.pin')} title={$_('chat.pin')}>📌</span>
  {/if}
  <span class="node-icon">{getIcon()}</span>
  <strong class="node-title">{getTitle()}</strong>
  
  {#if hasChildren}
    <button 
      class="expand-btn"
      on:click={handleToggleClick}
      aria-expanded={isExpanded}
      aria-label={isExpanded ? $_('chat.collapse') : $_('chat.expand')}
      tabindex="-1"
      disabled={isLoading}
    >
      {#if isLoading}
        <span class="loading-spinner">⏳</span>
      {:else}
        <span class="expand-arrow" class:expanded={isExpanded}>▶</span>
      {/if}
      <span class="child-count">{$_('chat.interactions', { values: { count: childCount } })}</span>
    </button>
  {/if}
</div>

<style>
  /* Header */
  .node-header {
    display: flex;
    align-items: center;
    gap: var(--chat-space-2);
    flex-wrap: wrap;
    margin-bottom: var(--chat-space-2);
  }
  
  .node-icon {
    font-size: var(--chat-font-size-lg);
  }
  
  .node-title {
    font-weight: var(--chat-font-weight-bold);
    color: inherit;
  }
  
  .pin-indicator {
    margin-right: var(--chat-space-1);
  }
  
  /* Botão de expansão */
  .expand-btn {
    display: inline-flex;
    align-items: center;
    gap: var(--chat-space-1);
    padding: var(--chat-space-1) var(--chat-space-2);
    background: var(--chat-color-bg-tertiary);
    border: 1px solid var(--chat-color-border);
    border-radius: var(--chat-radius-sm);
    font-size: var(--chat-font-size-sm);
    color: inherit;
    cursor: pointer;
    margin-left: auto;
    transition: background-color var(--chat-transition-fast);
  }
  
  .expand-btn:hover:not(:disabled) {
    background: var(--chat-color-hover);
  }
  
  .expand-btn:focus-visible {
    outline: 2px solid var(--chat-color-border-focus);
    outline-offset: 2px;
  }
  
  .expand-btn:disabled {
    opacity: 0.6;
    cursor: wait;
  }
  
  .expand-arrow {
    transition: transform var(--chat-transition-fast);
    display: inline-block;
    font-size: var(--chat-font-size-sm);
  }
  
  .expand-arrow.expanded {
    transform: rotate(90deg);
  }
  
  .loading-spinner {
    animation: spin 1s linear infinite;
  }
  
  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
  
  .child-count {
    color: var(--chat-color-text-muted);
  }
</style>
