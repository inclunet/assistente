<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { 
    GetFileAgentAuthorizedPaths, 
    CreateFileAgentAuthorizedPath, 
    DeleteFileAgentAuthorizedPath,
    GetFileAgentProtectedPaths 
  } from '../../../wailsjs/go/main/App.js';
  import { DataGrid } from '../../components/grid';

  const dispatch = createEventDispatcher();

  let authorizedPaths = [];
  let protectedInfo = { paths: [], extensions: [], files: [] };
  let loading = true;
  let error = '';

  // Formulário de nova pasta
  let showAddForm = false;
  let newPath = '';
  let newAllowDelete = true;
  let newAllowWrite = true;
  let newRecursive = true;
  let saving = false;
  let formError = '';

  // Colunas do grid
  const columns = [
    { 
      key: 'path', 
      label: 'Pasta',
      format: (value) => value
    },
    { 
      key: 'allow_delete', 
      label: 'Deletar',
      width: '80px',
      format: (value) => value ? '✅' : '❌'
    },
    { 
      key: 'allow_write', 
      label: 'Escrever',
      width: '80px',
      format: (value) => value ? '✅' : '❌'
    },
    { 
      key: 'recursive', 
      label: 'Recursivo',
      width: '90px',
      format: (value) => value ? '🔄' : '—'
    },
    { 
      key: 'delete', 
      label: 'Remover',
      width: '80px',
      action: true,
      actionIcon: '🗑️'
    }
  ];

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    loading = true;
    error = '';
    try {
      authorizedPaths = await GetFileAgentAuthorizedPaths() || [];
      protectedInfo = await GetFileAgentProtectedPaths() || { paths: [], extensions: [], files: [] };
    } catch (err) {
      error = 'Erro ao carregar dados: ' + (err.message || err);
    } finally {
      loading = false;
    }
  }

  function openAddForm() {
    newPath = '';
    newAllowDelete = true;
    newAllowWrite = true;
    newRecursive = true;
    formError = '';
    showAddForm = true;
  }

  function closeAddForm() {
    showAddForm = false;
    formError = '';
  }

  async function handleAdd() {
    if (!newPath.trim()) {
      formError = 'Caminho é obrigatório';
      return;
    }

    saving = true;
    formError = '';

    try {
      await CreateFileAgentAuthorizedPath(newPath.trim(), newAllowDelete, newAllowWrite, newRecursive);
      await loadData();
      closeAddForm();
    } catch (err) {
      formError = 'Erro ao adicionar: ' + (err.message || err);
    } finally {
      saving = false;
    }
  }

  async function handleDelete(item) {
    if (!confirm(`Remover autorização para "${item.path}"?`)) {
      return;
    }

    try {
      await DeleteFileAgentAuthorizedPath(item.id);
      await loadData();
    } catch (err) {
      error = 'Erro ao remover: ' + (err.message || err);
    }
  }

  function handleGridAction(event) {
    const { column, item } = event.detail;
    if (column.key === 'delete') {
      handleDelete(item);
    }
  }
</script>

<div class="file-agent-config">
  <div class="section">
    <div class="section-header">
      <h3>📁 Pastas Autorizadas</h3>
      <button class="btn-primary btn-small" on:click={openAddForm}>
        + Adicionar
      </button>
    </div>
    
    <p class="section-description">
      Pastas onde o FileAgent pode deletar arquivos sem pedir confirmação.
    </p>

    {#if loading}
      <div class="loading">Carregando...</div>
    {:else if error}
      <div class="error">{error}</div>
    {:else if authorizedPaths.length === 0}
      <div class="empty-state">
        <p>Nenhuma pasta autorizada.</p>
        <p class="hint">O FileAgent pedirá confirmação para cada exclusão.</p>
      </div>
    {:else}
      <DataGrid
        data={authorizedPaths}
        {columns}
        emptyMessage="Nenhuma pasta autorizada"
        on:action={handleGridAction}
      />
    {/if}
  </div>

  <div class="section">
    <h3>🔒 Pastas Protegidas (Sistema)</h3>
    <p class="section-description">
      Estas pastas e extensões são bloqueadas automaticamente e não podem ser acessadas.
    </p>

    <div class="protected-lists">
      <div class="protected-group">
        <h4>Pastas</h4>
        <div class="protected-items">
          {#each protectedInfo.paths || [] as path}
            <span class="protected-tag folder">{path}</span>
          {/each}
        </div>
      </div>

      <div class="protected-group">
        <h4>Extensões</h4>
        <div class="protected-items">
          {#each protectedInfo.extensions || [] as ext}
            <span class="protected-tag extension">{ext}</span>
          {/each}
        </div>
      </div>

      <div class="protected-group">
        <h4>Arquivos</h4>
        <div class="protected-items">
          {#each protectedInfo.files || [] as file}
            <span class="protected-tag file">{file}</span>
          {/each}
        </div>
      </div>
    </div>
  </div>
</div>

<!-- Modal de Adicionar -->
{#if showAddForm}
  <div class="modal-overlay" on:click|self={closeAddForm} on:keydown={(e) => e.key === 'Escape' && closeAddForm()}>
    <div class="modal-content" role="dialog" aria-modal="true">
      <h3>Adicionar Pasta Autorizada</h3>
      
      <form on:submit|preventDefault={handleAdd}>
        {#if formError}
          <div class="form-error" role="alert">{formError}</div>
        {/if}
        
        <div class="form-group">
          <label for="new-path">Caminho da Pasta</label>
          <input
            id="new-path"
            type="text"
            bind:value={newPath}
            placeholder="C:\Users\usuario\Downloads"
            autofocus
          />
        </div>

        <div class="form-group checkbox-group">
          <label>
            <input type="checkbox" bind:checked={newAllowDelete} />
            Permitir exclusão
          </label>
        </div>

        <div class="form-group checkbox-group">
          <label>
            <input type="checkbox" bind:checked={newAllowWrite} />
            Permitir escrita
          </label>
        </div>

        <div class="form-group checkbox-group">
          <label>
            <input type="checkbox" bind:checked={newRecursive} />
            Incluir subpastas
          </label>
        </div>
        
        <div class="form-actions">
          <button type="button" class="btn-secondary" on:click={closeAddForm} disabled={saving}>
            Cancelar
          </button>
          <button type="submit" class="btn-primary" disabled={saving}>
            {saving ? 'Salvando...' : 'Adicionar'}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}

<style>
  .file-agent-config {
    display: flex;
    flex-direction: column;
    gap: 2rem;
    padding: 1rem;
  }

  .section {
    background: var(--bg-secondary, #1a1a2e);
    border-radius: 8px;
    padding: 1.5rem;
  }

  .section-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.5rem;
  }

  .section h3 {
    margin: 0;
    color: var(--text-primary, #fff);
    font-size: 1.1rem;
  }

  .section-description {
    color: var(--text-secondary, #888);
    font-size: 0.9rem;
    margin: 0 0 1rem 0;
  }

  .btn-primary {
    background: var(--accent-color, #6366f1);
    color: white;
    border: none;
    padding: 0.5rem 1rem;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.9rem;
    transition: background 0.2s;
  }

  .btn-primary:hover {
    background: var(--accent-hover, #4f46e5);
  }

  .btn-primary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .btn-small {
    padding: 0.4rem 0.8rem;
    font-size: 0.85rem;
  }

  .btn-secondary {
    background: var(--bg-tertiary, #2a2a3e);
    color: var(--text-primary, #fff);
    border: 1px solid var(--border-color, #3a3a4e);
    padding: 0.5rem 1rem;
    border-radius: 6px;
    cursor: pointer;
  }

  .btn-secondary:hover {
    background: var(--bg-hover, #3a3a4e);
  }

  .loading {
    text-align: center;
    color: var(--text-secondary, #888);
    padding: 2rem;
  }

  .error {
    color: var(--error-color, #ef4444);
    padding: 1rem;
    background: rgba(239, 68, 68, 0.1);
    border-radius: 6px;
  }

  .empty-state {
    text-align: center;
    padding: 2rem;
    color: var(--text-secondary, #888);
  }

  .empty-state .hint {
    font-size: 0.85rem;
    opacity: 0.7;
  }

  .protected-lists {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .protected-group h4 {
    margin: 0 0 0.5rem 0;
    color: var(--text-secondary, #888);
    font-size: 0.85rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .protected-items {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .protected-tag {
    padding: 0.25rem 0.5rem;
    border-radius: 4px;
    font-size: 0.8rem;
    font-family: monospace;
  }

  .protected-tag.folder {
    background: rgba(239, 68, 68, 0.15);
    color: #f87171;
    border: 1px solid rgba(239, 68, 68, 0.3);
  }

  .protected-tag.extension {
    background: rgba(251, 191, 36, 0.15);
    color: #fbbf24;
    border: 1px solid rgba(251, 191, 36, 0.3);
  }

  .protected-tag.file {
    background: rgba(168, 85, 247, 0.15);
    color: #a855f7;
    border: 1px solid rgba(168, 85, 247, 0.3);
  }

  /* Modal */
  .modal-overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .modal-content {
    background: var(--bg-secondary, #1a1a2e);
    border-radius: 12px;
    padding: 1.5rem;
    width: 90%;
    max-width: 500px;
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
  }

  .modal-content h3 {
    margin: 0 0 1.5rem 0;
    color: var(--text-primary, #fff);
  }

  .form-group {
    margin-bottom: 1rem;
  }

  .form-group label {
    display: block;
    margin-bottom: 0.5rem;
    color: var(--text-secondary, #888);
    font-size: 0.9rem;
  }

  .form-group input[type="text"] {
    width: 100%;
    padding: 0.75rem;
    background: var(--bg-tertiary, #2a2a3e);
    border: 1px solid var(--border-color, #3a3a4e);
    border-radius: 6px;
    color: var(--text-primary, #fff);
    font-size: 0.95rem;
  }

  .form-group input[type="text"]:focus {
    outline: none;
    border-color: var(--accent-color, #6366f1);
  }

  .checkbox-group label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
  }

  .checkbox-group input[type="checkbox"] {
    width: 1.1rem;
    height: 1.1rem;
    accent-color: var(--accent-color, #6366f1);
  }

  .form-error {
    background: rgba(239, 68, 68, 0.1);
    color: #ef4444;
    padding: 0.75rem;
    border-radius: 6px;
    margin-bottom: 1rem;
    font-size: 0.9rem;
  }

  .form-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
    margin-top: 1.5rem;
  }
</style>

