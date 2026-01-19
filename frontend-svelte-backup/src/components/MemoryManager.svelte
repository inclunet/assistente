<script>
  import { onMount, onDestroy } from 'svelte';
  import { GetAllMemories, CreateMemory, UpdateMemory, DeleteMemory, SearchMemories } from '../../wailsjs/go/main/App.js';
  import { DataGrid } from './grid';

  export let label = 'Memórias salvas';
  
  // Atalho local Ctrl+N para nova memória
  function handleLocalKeyDown(event) {
    if (event.ctrlKey && event.key.toLowerCase() === 'n') {
      event.preventDefault();
      openNewForm();
    }
  }

  let memories = [];
  let loading = true;
  let error = '';
  let searchQuery = '';
  
  // Estado do formulário
  let showForm = false;
  let editingId = null;
  let formTitle = '';
  let formContent = '';
  let formCategory = 'geral';
  let formError = '';
  let saving = false;
  
  // Grid
  let gridComponent;

  const categories = [
    { value: 'core', label: 'Core (sempre no contexto)' },
    { value: 'usuario', label: 'Usuário' },
    { value: 'preferencia', label: 'Preferência' },
    { value: 'projeto', label: 'Projeto' },
    { value: 'contexto', label: 'Contexto' },
    { value: 'geral', label: 'Geral' }
  ];

  // Definição das colunas
  const columns = [
    { 
      key: 'title', 
      label: 'Título',
      truncate: true
    },
    { 
      key: 'content', 
      label: 'Conteúdo',
      truncate: true,
      format: (value) => value?.length > 100 ? value.substring(0, 100) + '...' : value
    },
    { 
      key: 'category', 
      label: 'Categoria',
      width: '120px',
      format: (value) => getCategoryLabel(value)
    },
    { 
      key: 'edit', 
      label: 'Editar',
      width: '80px',
      action: true,
      actionIcon: '✏️'
    },
    { 
      key: 'delete', 
      label: 'Excluir',
      width: '80px',
      action: true,
      actionIcon: '🗑️'
    }
  ];

  onMount(async () => {
    window.addEventListener('keydown', handleLocalKeyDown);
    await loadMemories();
  });
  
  onDestroy(() => {
    window.removeEventListener('keydown', handleLocalKeyDown);
  });

  export function focusList() {
    setTimeout(() => {
      gridComponent?.focus();
    }, 50);
  }

  async function loadMemories() {
    loading = true;
    error = '';
    try {
      const result = await GetAllMemories();
      memories = result || [];
    } catch (err) {
      error = 'Erro ao carregar memórias: ' + (err.message || err);
    } finally {
      loading = false;
    }
  }

  async function handleSearch() {
    if (!searchQuery.trim()) {
      await loadMemories();
      return;
    }
    
    loading = true;
    error = '';
    try {
      memories = await SearchMemories(searchQuery) || [];
    } catch (err) {
      error = 'Erro ao buscar: ' + err;
    } finally {
      loading = false;
    }
  }

  function openNewForm() {
    editingId = null;
    formTitle = '';
    formContent = '';
    formCategory = 'geral';
    formError = '';
    showForm = true;
  }

  function openEditForm(memory) {
    editingId = memory.id;
    formTitle = memory.title;
    formContent = memory.content;
    formCategory = memory.category || 'geral';
    formError = '';
    showForm = true;
  }

  function closeForm() {
    showForm = false;
    editingId = null;
    formTitle = '';
    formContent = '';
    formCategory = 'geral';
    formError = '';
  }

  async function handleSubmit() {
    if (!formTitle.trim() || !formContent.trim()) {
      formError = 'Título e conteúdo são obrigatórios';
      return;
    }

    saving = true;
    formError = '';

    try {
      if (editingId) {
        await UpdateMemory(editingId, formTitle, formContent, formCategory);
      } else {
        await CreateMemory(formTitle, formContent, formCategory);
      }
      closeForm();
      await loadMemories();
    } catch (err) {
      formError = 'Erro ao salvar: ' + err;
    } finally {
      saving = false;
    }
  }

  async function handleDelete(memory) {
    if (!confirm(`Tem certeza que deseja excluir a memória "${memory.title}"?`)) return;

    try {
      await DeleteMemory(memory.id);
      await loadMemories();
    } catch (err) {
      error = 'Erro ao excluir: ' + err;
    }
  }

  function handleSearchKeyDown(event) {
    if (event.key === 'Enter') {
      handleSearch();
    }
  }

  function getCategoryLabel(category) {
    return categories.find(c => c.value === category)?.label || category;
  }

  function handleActivate(event) {
    openEditForm(event.detail.item);
  }

  function handleCellAction(event) {
    const { item, column } = event.detail;
    
    if (column.key === 'edit') {
      openEditForm(item);
    } else if (column.key === 'delete') {
      handleDelete(item);
    }
  }

  function handleDeleteEvent(event) {
    handleDelete(event.detail.item);
  }
</script>

<div class="memory-manager">
  <header class="memory-header">
    <h2 id="memory-heading">{label}</h2>
    <button class="btn-primary" on:click={openNewForm}>
      + Nova Memória
    </button>
  </header>

  <div class="search-bar" role="search">
    <label for="memory-search" class="sr-only">Buscar memórias</label>
    <input
      id="memory-search"
      type="search"
      bind:value={searchQuery}
      on:keydown={handleSearchKeyDown}
      placeholder="Buscar memórias..."
    />
    <button class="btn-secondary" on:click={handleSearch}>
      Buscar
    </button>
    {#if searchQuery}
      <button class="btn-secondary" on:click={() => { searchQuery = ''; loadMemories(); }}>
        Limpar
      </button>
    {/if}
  </div>

  {#if error}
    <div class="error" role="alert">{error}</div>
  {/if}

  {#if loading}
    <div class="loading" role="status" aria-live="polite">
      <span class="loading-spinner" aria-hidden="true"></span>
      Carregando memórias...
    </div>
  {:else if memories.length === 0}
    <p class="empty">
      {searchQuery ? 'Nenhuma memória encontrada para esta busca.' : 'Nenhuma memória salva ainda. Clique em "+ Nova Memória" para criar.'}
    </p>
  {:else}
    <p class="memory-count" aria-live="polite">
      {memories.length} memória{memories.length !== 1 ? 's' : ''} encontrada{memories.length !== 1 ? 's' : ''}
    </p>
    
    <p class="sr-only">
      Use setas verticais para navegar entre memórias e setas horizontais para navegar entre os campos.
      Enter para ativar, Delete para excluir.
    </p>
    
    <DataGrid
      bind:this={gridComponent}
      items={memories}
      {columns}
      {label}
      getItemId={(m) => m.id}
      multiSelect={false}
      on:activate={handleActivate}
      on:delete={handleDeleteEvent}
      on:cellAction={handleCellAction}
    >
      <p slot="empty" class="empty">Nenhuma memória encontrada.</p>
    </DataGrid>
  {/if}
</div>

{#if showForm}
  <div class="modal-overlay" on:click|self={closeForm} on:keydown={(e) => e.key === 'Escape' && closeForm()}>
    <div
      class="modal-content"
      role="dialog"
      aria-modal="true"
      aria-labelledby="memory-form-title"
    >
      <h3 id="memory-form-title">{editingId ? 'Editar Memória' : 'Nova Memória'}</h3>
      
      <form on:submit|preventDefault={handleSubmit}>
        {#if formError}
          <div class="form-error" role="alert">{formError}</div>
        {/if}
        
        <div class="form-group">
          <label for="memory-title">Título</label>
          <input
            id="memory-title"
            type="text"
            bind:value={formTitle}
            placeholder="Ex: Meu nome é..."
            required
          />
        </div>

        <div class="form-group">
          <label for="memory-content">Conteúdo</label>
          <textarea
            id="memory-content"
            bind:value={formContent}
            rows="4"
            placeholder="Informação que o assistente deve lembrar..."
            required
          ></textarea>
        </div>
        
        <div class="form-group">
          <label for="memory-category">Categoria</label>
          <select id="memory-category" bind:value={formCategory}>
            {#each categories as cat}
              <option value={cat.value}>{cat.label}</option>
            {/each}
          </select>
          <p class="form-hint">
            ⭐ <strong>Core</strong>: Informações essenciais que aparecem automaticamente em todas as conversas.
          </p>
        </div>
        
        <div class="form-actions">
          <button type="button" class="btn-secondary" on:click={closeForm} disabled={saving}>
            Cancelar
          </button>
          <button type="submit" class="btn-primary" disabled={saving}>
            {saving ? 'Salvando...' : 'Salvar'}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}

<style>
  .memory-manager {
    display: flex;
    flex-direction: column;
    min-height: 300px;
    gap: var(--spacing-md);
  }

  .memory-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-bottom: var(--spacing-md);
    border-bottom: 1px solid var(--color-border);
  }

  .memory-header h2 {
    margin: 0;
    font-size: var(--font-size-xl);
  }

  .search-bar {
    display: flex;
    gap: var(--spacing-sm);
  }

  .search-bar input {
    flex: 1;
    padding: var(--spacing-sm);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
  }

  .memory-count {
    font-size: var(--font-size-sm);
    color: var(--color-text-muted);
    margin: 0;
  }

  .btn-primary {
    background-color: var(--color-accent);
    color: white;
    border: none;
    border-radius: var(--border-radius);
    padding: var(--spacing-sm) var(--spacing-md);
    cursor: pointer;
  }

  .btn-secondary {
    background-color: var(--color-bg-tertiary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    padding: var(--spacing-sm) var(--spacing-md);
    cursor: pointer;
  }

  .btn-primary:hover, .btn-secondary:hover {
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

  /* Modal */
  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .modal-content {
    background: var(--color-bg-secondary);
    border-radius: var(--border-radius-lg, 12px);
    padding: var(--spacing-lg);
    width: 90%;
    max-width: 500px;
    max-height: 90vh;
    overflow-y: auto;
  }

  .modal-content h3 {
    margin: 0 0 var(--spacing-md);
    font-size: var(--font-size-lg);
  }

  .form-group {
    margin-bottom: var(--spacing-md);
  }

  .form-group label {
    display: block;
    margin-bottom: var(--spacing-xs);
    font-weight: 500;
  }

  .form-group input,
  .form-group textarea,
  .form-group select {
    width: 100%;
    padding: var(--spacing-sm);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
  }

  .form-hint {
    font-size: var(--font-size-sm);
    color: var(--color-text-muted);
    margin-top: var(--spacing-xs);
  }

  .form-error {
    background: rgba(248, 81, 73, 0.1);
    border: 1px solid var(--color-error);
    color: var(--color-error);
    padding: var(--spacing-sm);
    border-radius: var(--border-radius);
    margin-bottom: var(--spacing-md);
  }

  .form-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--spacing-sm);
    margin-top: var(--spacing-lg);
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
