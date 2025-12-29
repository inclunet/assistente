<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { GetAllFAQs, CreateFAQ, UpdateFAQ, DeleteFAQ, SearchFAQ, GetFAQEmbeddingStatus, RegenerateFAQEmbeddings } from '../../wailsjs/go/main/App.js';
  import DataGrid from './DataGrid.svelte';

  export let label = 'Perguntas Frequentes';

  const dispatch = createEventDispatcher();
  
  // Atalho local Ctrl+N para nova FAQ
  function handleLocalKeyDown(event) {
    if (event.ctrlKey && event.key.toLowerCase() === 'n') {
      event.preventDefault();
      openNewForm();
    }
  }

  let faqs = [];
  let loading = true;
  let error = '';
  let searchQuery = '';
  
  // Embeddings status
  let embeddingStatus = null;
  let generatingEmbeddings = false;
  
  // Estado do formulário
  let showForm = false;
  let editingId = null;
  let formQuestion = '';
  let formAnswer = '';
  let formTags = '';
  let formError = '';
  let saving = false;
  
  // Grid
  let gridComponent;
  
  // Definição das colunas
  const columns = [
    { 
      key: 'question', 
      label: 'Pergunta',
      truncate: true
    },
    { 
      key: 'answer', 
      label: 'Resposta',
      truncate: true,
      format: (value) => value?.length > 80 ? value.substring(0, 80) + '...' : value
    },
    { 
      key: 'tags', 
      label: 'Tags',
      width: '150px',
      format: (value) => value || '—'
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
    await loadFAQs();
  });
  
  onDestroy(() => {
    window.removeEventListener('keydown', handleLocalKeyDown);
  });

  export function focusList() {
    setTimeout(() => {
      gridComponent?.focus();
    }, 50);
  }

  async function loadFAQs() {
    loading = true;
    error = '';
    try {
      const result = await GetAllFAQs();
      faqs = result || [];
      // Carrega status de embeddings
      await loadEmbeddingStatus();
    } catch (err) {
      error = 'Erro ao carregar FAQs: ' + (err.message || err);
    } finally {
      loading = false;
    }
  }
  
  async function loadEmbeddingStatus() {
    try {
      embeddingStatus = await GetFAQEmbeddingStatus();
    } catch (err) {
      console.log('Erro ao carregar status de embeddings:', err);
      embeddingStatus = null;
    }
  }
  
  async function handleRegenerateEmbeddings() {
    if (generatingEmbeddings) return;
    
    generatingEmbeddings = true;
    error = '';
    
    try {
      const count = await RegenerateFAQEmbeddings();
      if (count > 0) {
        alert(`✅ ${count} embedding(s) gerado(s) com sucesso!`);
      } else {
        alert('Todas as FAQs já possuem embeddings.');
      }
      await loadEmbeddingStatus();
    } catch (err) {
      error = 'Erro ao gerar embeddings: ' + (err.message || err);
    } finally {
      generatingEmbeddings = false;
    }
  }

  async function handleSearch() {
    if (!searchQuery.trim()) {
      await loadFAQs();
      return;
    }
    
    loading = true;
    error = '';
    try {
      faqs = await SearchFAQ(searchQuery) || [];
    } catch (err) {
      error = 'Erro ao buscar: ' + err;
    } finally {
      loading = false;
    }
  }

  function openNewForm() {
    editingId = null;
    formQuestion = '';
    formAnswer = '';
    formTags = '';
    formError = '';
    showForm = true;
  }

  function openEditForm(faq) {
    editingId = faq.id;
    formQuestion = faq.question;
    formAnswer = faq.answer;
    formTags = faq.tags || '';
    formError = '';
    showForm = true;
  }

  function closeForm() {
    showForm = false;
    editingId = null;
    formQuestion = '';
    formAnswer = '';
    formTags = '';
    formError = '';
  }

  async function handleSubmit() {
    if (!formQuestion.trim() || !formAnswer.trim()) {
      formError = 'Pergunta e resposta são obrigatórias';
      return;
    }

    saving = true;
    formError = '';

    try {
      if (editingId) {
        await UpdateFAQ(editingId, formQuestion, formAnswer, formTags);
      } else {
        await CreateFAQ(formQuestion, formAnswer, formTags);
      }
      closeForm();
      await loadFAQs();
    } catch (err) {
      formError = 'Erro ao salvar: ' + err;
    } finally {
      saving = false;
    }
  }

  async function handleDelete(faq) {
    if (!confirm(`Tem certeza que deseja excluir a FAQ "${faq.question}"?`)) return;

    try {
      await DeleteFAQ(faq.id);
      await loadFAQs();
    } catch (err) {
      error = 'Erro ao excluir: ' + err;
    }
  }

  function handleSearchKeyDown(event) {
    if (event.key === 'Enter') {
      handleSearch();
    }
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

<div class="faq-manager">
  <header class="faq-header">
    <div class="header-left">
      <h2 id="faq-heading">{label}</h2>
      {#if embeddingStatus}
        <span class="embedding-status" class:warning={embeddingStatus.without_embedding > 0}>
          🧠 {embeddingStatus.with_embedding}/{embeddingStatus.total_faqs} com busca semântica
        </span>
      {/if}
    </div>
    <div class="header-actions">
      {#if embeddingStatus && embeddingStatus.without_embedding > 0}
        <button 
          class="btn-secondary" 
          on:click={handleRegenerateEmbeddings}
          disabled={generatingEmbeddings}
          title="Gerar embeddings para busca semântica"
        >
          {generatingEmbeddings ? '⏳ Gerando...' : '🧠 Gerar Embeddings'}
        </button>
      {/if}
      <button class="btn-primary" on:click={openNewForm}>
        + Nova FAQ
      </button>
    </div>
  </header>

  <div class="search-bar" role="search">
    <label for="faq-search" class="sr-only">Buscar FAQs</label>
    <input
      id="faq-search"
      type="search"
      bind:value={searchQuery}
      on:keydown={handleSearchKeyDown}
      placeholder="Buscar perguntas..."
    />
    <button class="btn-secondary" on:click={handleSearch}>
      Buscar
    </button>
    {#if searchQuery}
      <button class="btn-secondary" on:click={() => { searchQuery = ''; loadFAQs(); }}>
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
      Carregando FAQs...
    </div>
  {:else if faqs.length === 0}
    <p class="empty">
      {searchQuery ? 'Nenhuma FAQ encontrada para esta busca.' : 'Nenhuma FAQ cadastrada. Clique em "+ Nova FAQ" para criar.'}
    </p>
  {:else}
    <p class="faq-count" aria-live="polite">
      {faqs.length} FAQ{faqs.length !== 1 ? 's' : ''} encontrada{faqs.length !== 1 ? 's' : ''}
    </p>
    
    <p class="sr-only">
      Use setas verticais para navegar entre FAQs e setas horizontais para navegar entre os campos.
      Enter para ativar, Delete para excluir.
    </p>
    
    <DataGrid
      bind:this={gridComponent}
      items={faqs}
      {columns}
      {label}
      getItemId={(f) => f.id}
      multiSelect={false}
      on:activate={handleActivate}
      on:delete={handleDeleteEvent}
      on:cellAction={handleCellAction}
    >
      <p slot="empty" class="empty">Nenhuma FAQ encontrada.</p>
    </DataGrid>
  {/if}
</div>

{#if showForm}
  <div class="modal-overlay" on:click|self={closeForm} on:keydown={(e) => e.key === 'Escape' && closeForm()}>
    <div
      class="modal-content"
      role="dialog"
      aria-modal="true"
      aria-labelledby="faq-form-title"
    >
      <h3 id="faq-form-title">{editingId ? 'Editar FAQ' : 'Nova FAQ'}</h3>
      
      <form on:submit|preventDefault={handleSubmit}>
        {#if formError}
          <div class="form-error" role="alert">{formError}</div>
        {/if}
        
        <div class="form-group">
          <label for="faq-question">Pergunta</label>
          <input
            id="faq-question"
            type="text"
            bind:value={formQuestion}
            placeholder="Ex: Como faço para..."
            required
          />
        </div>

        <div class="form-group">
          <label for="faq-answer">Resposta</label>
          <textarea
            id="faq-answer"
            bind:value={formAnswer}
            rows="4"
            placeholder="Resposta detalhada..."
            required
          ></textarea>
        </div>
        
        <div class="form-group">
          <label for="faq-tags">Tags (separadas por vírgula)</label>
          <input
            id="faq-tags"
            type="text"
            bind:value={formTags}
            placeholder="Ex: configuração, início, ajuda"
          />
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
  .faq-manager {
    display: flex;
    flex-direction: column;
    min-height: 300px;
    gap: var(--spacing-md);
  }

  .faq-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-bottom: var(--spacing-md);
    border-bottom: 1px solid var(--color-border);
    flex-wrap: wrap;
    gap: var(--spacing-sm);
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: var(--spacing-md);
    flex-wrap: wrap;
  }

  .faq-header h2 {
    margin: 0;
    font-size: var(--font-size-xl);
  }

  .header-actions {
    display: flex;
    gap: var(--spacing-sm);
  }

  .embedding-status {
    font-size: var(--font-size-sm);
    padding: 4px 10px;
    border-radius: 12px;
    background: rgba(74, 222, 128, 0.15);
    color: #16a34a;
  }

  .embedding-status.warning {
    background: rgba(250, 204, 21, 0.15);
    color: #ca8a04;
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

  .faq-count {
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
  .form-group textarea {
    width: 100%;
    padding: var(--spacing-sm);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
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
