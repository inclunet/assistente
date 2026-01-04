<script>
  import { createEventDispatcher, onMount, onDestroy, tick } from 'svelte';
  import { GetConversations, DeleteConversation } from '../../wailsjs/go/main/App.js';
  import { DataGrid } from './grid';

  export let currentConversationId = null;
  export let label = 'Histórico de conversas';

  const dispatch = createEventDispatcher();
  
  // Atalho local Ctrl+N para nova conversa
  function handleLocalKeyDown(event) {
    if (event.ctrlKey && event.key.toLowerCase() === 'n') {
      event.preventDefault();
      handleNew();
    }
  }

  let conversations = [];
  let loading = true;
  let error = '';
  
  // Seleção
  let selectedIds = new Set();
  let gridComponent;
  
  // Região para anúncios
  let announcement = '';

  // Helper para extrair modelo das preferências
  function getModelFromConversation(conv) {
    if (!conv.preferences) return null;
    try {
      const prefs = typeof conv.preferences === 'string' 
        ? JSON.parse(conv.preferences) 
        : conv.preferences;
      return prefs?.model || null;
    } catch (e) {
      return null;
    }
  }

  // Definição das colunas
  const columns = [
    { 
      key: 'title', 
      label: 'Título',
      truncate: true,
      format: (value) => value || 'Sem título'
    },
    { 
      key: 'preferences', 
      label: 'Modelo',
      width: '150px',
      format: (value, item) => getModelFromConversation(item) || 'Padrão'
    },
    { 
      key: 'updated_at', 
      label: 'Data',
      width: '120px',
      format: (value) => formatDate(value)
    },
    { 
      key: 'open', 
      label: 'Abrir',
      width: '80px',
      action: true,
      actionIcon: '📂'
    },
    { 
      key: 'delete', 
      label: 'Excluir',
      width: '80px',
      action: true,
      actionIcon: '🗑️'
    }
  ];

  export async function refresh() {
    await loadConversations();
  }
  
  export function focusList() {
    const tryFocus = () => {
      if (loading) {
        setTimeout(tryFocus, 50);
        return;
      }
      gridComponent?.focus();
    };
    tryFocus();
  }

  async function loadConversations() {
    loading = true;
    error = '';
    try {
      conversations = await GetConversations() || [];
      selectedIds = new Set();
    } catch (err) {
      error = 'Erro ao carregar conversas';
      console.error(err);
    } finally {
      loading = false;
    }
  }

  async function deleteConversation(conv) {
    if (!confirm(`Excluir a conversa "${conv.title || 'Sem título'}"?`)) return;
    
    try {
      await DeleteConversation(conv.id);
      if (currentConversationId === conv.id) {
        dispatch('select', null);
      }
      await loadConversations();
      announce('Conversa excluída');
    } catch (err) {
      error = 'Erro ao excluir conversa';
    }
  }

  async function deleteSelected() {
    if (selectedIds.size === 0) return;
    
    const count = selectedIds.size;
    const msg = count === 1 
      ? 'Excluir a conversa selecionada?' 
      : `Excluir ${count} conversas selecionadas?`;
    
    if (!confirm(msg)) return;
    
    try {
      for (const id of selectedIds) {
        await DeleteConversation(id);
        if (currentConversationId === id) {
          dispatch('select', null);
        }
      }
      await loadConversations();
      announce(`${count} conversa${count > 1 ? 's' : ''} excluída${count > 1 ? 's' : ''}`);
    } catch (err) {
      error = 'Erro ao excluir conversas';
    }
  }

  function handleNew() {
    dispatch('new');
  }

  function handleSelect(conv) {
    dispatch('select', conv);
  }
  
  function announce(message) {
    announcement = '';
    tick().then(() => {
      announcement = message;
    });
  }

  function formatDate(dateStr) {
    const date = new Date(dateStr);
    const now = new Date();
    const diff = now - date;
    
    if (diff < 60000) return 'Agora';
    if (diff < 3600000) return `${Math.floor(diff / 60000)} min`;
    if (diff < 86400000) return `${Math.floor(diff / 3600000)} h`;
    if (diff < 604800000) return `${Math.floor(diff / 86400000)} d`;
    
    return date.toLocaleDateString('pt-BR');
  }

  function handleActivate(event) {
    handleSelect(event.detail.item);
  }

  function handleCellAction(event) {
    const { item, column } = event.detail;
    
    if (column.key === 'open') {
      handleSelect(item);
    } else if (column.key === 'delete') {
      deleteConversation(item);
    }
  }

  function handleDeleteEvent(event) {
    deleteConversation(event.detail.item);
  }

  function handleSelectionChange(event) {
    selectedIds = event.detail.selectedIds;
  }

  onMount(() => {
    window.addEventListener('keydown', handleLocalKeyDown);
    loadConversations();
  });
  
  onDestroy(() => {
    window.removeEventListener('keydown', handleLocalKeyDown);
  });
</script>

<div class="conversation-list">
  <header class="list-header">
    <h2 id="conv-list-heading">{label}</h2>
    <div class="header-actions">
      {#if selectedIds.size > 0}
        <button 
          class="btn-danger btn-sm" 
          on:click={deleteSelected}
          aria-label="Excluir {selectedIds.size} conversa{selectedIds.size > 1 ? 's' : ''} selecionada{selectedIds.size > 1 ? 's' : ''}"
        >
          🗑️ Excluir ({selectedIds.size})
        </button>
      {/if}
      <button class="btn-primary btn-sm" on:click={handleNew}>
        + Nova
      </button>
    </div>
  </header>

  {#if loading}
    <div class="loading" role="status" aria-live="polite">
      <span class="loading-spinner" aria-hidden="true"></span>
      Carregando conversas...
    </div>
  {:else if error}
    <div class="error" role="alert">{error}</div>
  {:else if conversations.length === 0}
    <p class="empty">Nenhuma conversa ainda. Clique em "+ Nova" para começar.</p>
  {:else}
    <p class="sr-only">
      Use setas verticais para navegar entre conversas e setas horizontais para navegar entre os campos.
      Enter para ativar, Delete para excluir. Shift+setas para seleção múltipla.
    </p>
    
    <DataGrid
      bind:this={gridComponent}
      items={conversations}
      {columns}
      {label}
      getItemId={(c) => c.id}
      bind:selectedIds
      multiSelect={true}
      on:activate={handleActivate}
      on:delete={handleDeleteEvent}
      on:cellAction={handleCellAction}
      on:selectionChange={handleSelectionChange}
    >
      <p slot="empty" class="empty">Nenhuma conversa encontrada.</p>
    </DataGrid>
  {/if}
</div>

<!-- Região para anúncios de acessibilidade -->
<div 
  class="sr-only"
  role="status"
  aria-live="assertive"
  aria-atomic="true"
>{announcement}</div>

<style>
  .conversation-list {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 300px;
    gap: var(--spacing-md);
  }

  .list-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-bottom: var(--spacing-md);
    border-bottom: 1px solid var(--color-border);
  }

  .list-header h2 {
    margin: 0;
    font-size: var(--font-size-lg);
    color: var(--color-text-primary);
  }
  
  .header-actions {
    display: flex;
    gap: var(--spacing-sm);
  }

  .btn-sm {
    padding: var(--spacing-xs) var(--spacing-sm);
    font-size: var(--font-size-sm);
  }
  
  .btn-primary {
    background-color: var(--color-accent);
    color: white;
    border: none;
    border-radius: var(--border-radius);
    cursor: pointer;
  }
  
  .btn-danger {
    background-color: var(--color-error);
    color: white;
    border: none;
    border-radius: var(--border-radius);
    cursor: pointer;
  }
  
  .btn-primary:hover, .btn-danger:hover {
    opacity: 0.9;
  }

  .loading, .error, .empty {
    padding: var(--spacing-lg);
    text-align: center;
    color: var(--color-text-muted);
  }

  .error {
    color: var(--color-error);
  }
  
  .loading-spinner {
    display: inline-block;
    width: 16px;
    height: 16px;
    border: 2px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 0.75s linear infinite;
    margin-right: var(--spacing-xs);
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }

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
