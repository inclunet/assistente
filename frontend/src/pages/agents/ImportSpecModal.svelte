<script>
  import { createEventDispatcher } from 'svelte';
  import { ParseOpenAPISpec, ParsePostmanCollection, ImportOpenAPIToHTTPAgent, ImportPostmanToHTTPAgent } from '../../../wailsjs/go/main/App.js';
  import { Modal } from '../../components/modal';
  
  export let open = false;
  
  const dispatch = createEventDispatcher();
  
  let step = 'input'; // input, preview, importing
  let importType = 'openapi'; // openapi, postman
  let specContent = '';
  let agentName = '';
  let error = '';
  let loading = false;
  
  // Dados do preview
  let previewData = null;
  let selectedEndpoints = [];
  
  function close() {
    step = 'input';
    specContent = '';
    agentName = '';
    error = '';
    previewData = null;
    selectedEndpoints = [];
    dispatch('close');
  }
  
  async function handleParse() {
    if (!specContent.trim()) {
      error = 'Cole o conteúdo da especificação';
      return;
    }
    
    loading = true;
    error = '';
    
    try {
      if (importType === 'openapi') {
        previewData = await ParseOpenAPISpec(specContent);
      } else {
        previewData = await ParsePostmanCollection(specContent);
      }
      
      // Seleciona todos os endpoints por padrão
      selectedEndpoints = previewData.endpoints.map((_, i) => i);
      
      // Gera nome do agente
      if (!agentName) {
        agentName = previewData.display_name
          .toLowerCase()
          .replace(/[^a-z0-9]+/g, '_')
          .replace(/^_|_$/g, '');
      }
      
      step = 'preview';
    } catch (err) {
      error = 'Erro ao analisar: ' + (err.message || err);
    } finally {
      loading = false;
    }
  }
  
  async function handleImport() {
    if (!agentName.trim()) {
      error = 'Nome do agente é obrigatório';
      return;
    }
    
    loading = true;
    error = '';
    step = 'importing';
    
    try {
      let agent;
      if (importType === 'openapi') {
        agent = await ImportOpenAPIToHTTPAgent(specContent, agentName);
      } else {
        agent = await ImportPostmanToHTTPAgent(specContent, agentName);
      }
      
      dispatch('imported', { agent });
      close();
    } catch (err) {
      error = 'Erro ao importar: ' + (err.message || err);
      step = 'preview';
    } finally {
      loading = false;
    }
  }
  
  function toggleEndpoint(index) {
    if (selectedEndpoints.includes(index)) {
      selectedEndpoints = selectedEndpoints.filter(i => i !== index);
    } else {
      selectedEndpoints = [...selectedEndpoints, index];
    }
  }
  
  function selectAllEndpoints() {
    if (previewData) {
      selectedEndpoints = previewData.endpoints.map((_, i) => i);
    }
  }
  
  function deselectAllEndpoints() {
    selectedEndpoints = [];
  }
  
  function getMethodColor(method) {
    switch (method) {
      case 'GET': return '#61affe';
      case 'POST': return '#49cc90';
      case 'PUT': return '#fca130';
      case 'PATCH': return '#50e3c2';
      case 'DELETE': return '#f93e3e';
      default: return '#999';
    }
  }
  
  async function handleFileUpload(event) {
    const file = event.target.files[0];
    if (!file) return;
    
    try {
      specContent = await file.text();
      
      // Detecta tipo baseado no conteúdo
      if (specContent.includes('"openapi"') || specContent.includes('openapi:') ||
          specContent.includes('"swagger"') || specContent.includes('swagger:')) {
        importType = 'openapi';
      } else if (specContent.includes('"info"') && specContent.includes('"item"')) {
        importType = 'postman';
      }
    } catch (err) {
      error = 'Erro ao ler arquivo: ' + (err.message || err);
    }
  }
</script>

<Modal title="📥 Importar Especificação" {open} on:close={close} wide={true}>
  <div class="import-modal">
    {#if step === 'input'}
      <div class="step-input">
        <div class="import-type-selector">
          <button 
            class="type-btn" 
            class:active={importType === 'openapi'}
            on:click={() => importType = 'openapi'}
          >
            <span class="icon">📋</span>
            <span class="label">OpenAPI / Swagger</span>
          </button>
          <button 
            class="type-btn" 
            class:active={importType === 'postman'}
            on:click={() => importType = 'postman'}
          >
            <span class="icon">📮</span>
            <span class="label">Postman Collection</span>
          </button>
        </div>
        
        <div class="file-upload">
          <input 
            type="file" 
            id="spec-file"
            accept=".json,.yaml,.yml"
            on:change={handleFileUpload}
          />
          <label for="spec-file" class="file-label">
            📁 Selecionar arquivo
          </label>
          <span class="or">ou</span>
        </div>
        
        <div class="form-group">
          <label for="spec-content">Cole o conteúdo da especificação (JSON ou YAML)</label>
          <textarea 
            id="spec-content"
            bind:value={specContent}
            rows="15"
            placeholder={importType === 'openapi' 
              ? '{\n  "openapi": "3.0.0",\n  "info": { "title": "My API" },\n  ...\n}'
              : '{\n  "info": { "name": "My Collection" },\n  "item": [...]\n}'}
            class="spec-textarea"
          ></textarea>
        </div>
        
        {#if error}
          <div class="error-message">{error}</div>
        {/if}
        
        <div class="actions">
          <button class="btn-secondary" on:click={close}>Cancelar</button>
          <button class="btn-primary" on:click={handleParse} disabled={loading}>
            {loading ? 'Analisando...' : 'Analisar Especificação'}
          </button>
        </div>
      </div>
      
    {:else if step === 'preview'}
      <div class="step-preview">
        <div class="preview-header">
          <h3>{previewData.display_name}</h3>
          <p class="description">{previewData.description || 'Sem descrição'}</p>
        </div>
        
        <div class="preview-info">
          <div class="info-item">
            <span class="label">Base URL:</span>
            <code>{previewData.base_url}</code>
          </div>
          <div class="info-item">
            <span class="label">Autenticação:</span>
            <span>{previewData.auth_type || 'Nenhuma'}</span>
          </div>
          <div class="info-item">
            <span class="label">Endpoints:</span>
            <span>{previewData.endpoints.length} encontrados</span>
          </div>
        </div>
        
        <div class="form-group">
          <label for="agent-name">Nome do Agente (ID)</label>
          <input 
            type="text" 
            id="agent-name"
            bind:value={agentName}
            placeholder="minha_api"
            class="input-mono"
          />
        </div>
        
        <div class="endpoints-section">
          <div class="endpoints-header">
            <h4>Endpoints a importar</h4>
            <div class="endpoints-actions">
              <button class="btn-link" on:click={selectAllEndpoints}>Selecionar todos</button>
              <button class="btn-link" on:click={deselectAllEndpoints}>Limpar seleção</button>
            </div>
          </div>
          
          <div class="endpoints-list">
            {#each previewData.endpoints as endpoint, index}
              <label class="endpoint-item" class:selected={selectedEndpoints.includes(index)}>
                <input 
                  type="checkbox" 
                  checked={selectedEndpoints.includes(index)}
                  on:change={() => toggleEndpoint(index)}
                />
                <span class="method-badge" style="background: {getMethodColor(endpoint.method)}">
                  {endpoint.method}
                </span>
                <span class="endpoint-name">{endpoint.name}</span>
                <span class="endpoint-path">{endpoint.path_template}</span>
              </label>
            {/each}
          </div>
        </div>
        
        {#if error}
          <div class="error-message">{error}</div>
        {/if}
        
        <div class="actions">
          <button class="btn-secondary" on:click={() => step = 'input'}>Voltar</button>
          <button class="btn-primary" on:click={handleImport} disabled={loading || selectedEndpoints.length === 0}>
            {loading ? 'Importando...' : `Importar ${selectedEndpoints.length} endpoints`}
          </button>
        </div>
      </div>
      
    {:else if step === 'importing'}
      <div class="step-importing">
        <div class="loading-spinner"></div>
        <p>Criando agente HTTP e endpoints...</p>
      </div>
    {/if}
  </div>
</Modal>

<style>
  .import-modal {
    min-height: 400px;
  }
  
  .import-type-selector {
    display: flex;
    gap: var(--spacing-md);
    margin-bottom: var(--spacing-lg);
  }
  
  .type-btn {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-lg);
    border: 2px solid var(--color-border);
    border-radius: var(--border-radius);
    background: var(--color-bg-secondary);
    cursor: pointer;
    transition: all 0.2s;
  }
  
  .type-btn:hover {
    border-color: var(--color-accent);
  }
  
  .type-btn.active {
    border-color: var(--color-accent);
    background: rgba(var(--color-accent-rgb), 0.1);
  }
  
  .type-btn .icon {
    font-size: 2rem;
  }
  
  .type-btn .label {
    font-weight: 500;
    color: var(--color-text-primary);
  }
  
  .file-upload {
    display: flex;
    align-items: center;
    gap: var(--spacing-md);
    margin-bottom: var(--spacing-md);
  }
  
  .file-upload input[type="file"] {
    display: none;
  }
  
  .file-label {
    padding: var(--spacing-sm) var(--spacing-md);
    background: var(--color-bg-tertiary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    cursor: pointer;
    transition: all 0.2s;
  }
  
  .file-label:hover {
    border-color: var(--color-accent);
  }
  
  .or {
    color: var(--color-text-muted);
  }
  
  .form-group {
    margin-bottom: var(--spacing-md);
  }
  
  .form-group label {
    display: block;
    margin-bottom: var(--spacing-xs);
    font-weight: 500;
  }
  
  .spec-textarea {
    width: 100%;
    padding: var(--spacing-sm);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-family: 'Fira Code', monospace;
    font-size: var(--font-size-sm);
    resize: vertical;
  }
  
  .input-mono {
    width: 100%;
    padding: var(--spacing-sm);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-family: 'Fira Code', monospace;
  }
  
  .error-message {
    margin: var(--spacing-md) 0;
    padding: var(--spacing-sm);
    background: rgba(248, 81, 73, 0.1);
    border: 1px solid var(--color-error);
    border-radius: var(--border-radius);
    color: var(--color-error);
  }
  
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--spacing-sm);
    margin-top: var(--spacing-lg);
    padding-top: var(--spacing-md);
    border-top: 1px solid var(--color-border);
  }
  
  .btn-primary, .btn-secondary {
    padding: var(--spacing-sm) var(--spacing-lg);
    border-radius: var(--border-radius);
    cursor: pointer;
    font-weight: 500;
  }
  
  .btn-primary {
    background: var(--color-accent);
    color: white;
    border: none;
  }
  
  .btn-primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .btn-secondary {
    background: var(--color-bg-tertiary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border);
  }
  
  /* Preview step */
  .preview-header {
    margin-bottom: var(--spacing-lg);
  }
  
  .preview-header h3 {
    margin: 0 0 var(--spacing-xs);
  }
  
  .preview-header .description {
    color: var(--color-text-secondary);
    margin: 0;
  }
  
  .preview-info {
    display: flex;
    flex-wrap: wrap;
    gap: var(--spacing-lg);
    padding: var(--spacing-md);
    background: var(--color-bg-tertiary);
    border-radius: var(--border-radius);
    margin-bottom: var(--spacing-lg);
  }
  
  .info-item {
    display: flex;
    gap: var(--spacing-sm);
  }
  
  .info-item .label {
    font-weight: 500;
    color: var(--color-text-muted);
  }
  
  .info-item code {
    font-family: 'Fira Code', monospace;
    font-size: var(--font-size-sm);
    background: var(--color-bg-secondary);
    padding: 2px 6px;
    border-radius: 3px;
  }
  
  .endpoints-section {
    margin-top: var(--spacing-lg);
  }
  
  .endpoints-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--spacing-sm);
  }
  
  .endpoints-header h4 {
    margin: 0;
  }
  
  .endpoints-actions {
    display: flex;
    gap: var(--spacing-sm);
  }
  
  .btn-link {
    background: none;
    border: none;
    color: var(--color-accent);
    cursor: pointer;
    font-size: var(--font-size-sm);
    padding: 0;
  }
  
  .btn-link:hover {
    text-decoration: underline;
  }
  
  .endpoints-list {
    max-height: 250px;
    overflow-y: auto;
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
  }
  
  .endpoint-item {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm) var(--spacing-md);
    cursor: pointer;
    border-bottom: 1px solid var(--color-border);
    transition: background 0.2s;
  }
  
  .endpoint-item:last-child {
    border-bottom: none;
  }
  
  .endpoint-item:hover {
    background: var(--color-bg-secondary);
  }
  
  .endpoint-item.selected {
    background: rgba(var(--color-accent-rgb), 0.1);
  }
  
  .endpoint-item input[type="checkbox"] {
    flex-shrink: 0;
  }
  
  .method-badge {
    padding: 2px 6px;
    border-radius: 3px;
    font-size: var(--font-size-xs);
    font-weight: 600;
    color: white;
    flex-shrink: 0;
    min-width: 50px;
    text-align: center;
  }
  
  .endpoint-name {
    font-weight: 500;
    font-family: 'Fira Code', monospace;
    font-size: var(--font-size-sm);
  }
  
  .endpoint-path {
    color: var(--color-text-muted);
    font-family: 'Fira Code', monospace;
    font-size: var(--font-size-xs);
    margin-left: auto;
    flex-shrink: 0;
  }
  
  /* Importing step */
  .step-importing {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    min-height: 200px;
    gap: var(--spacing-md);
  }
  
  .loading-spinner {
    width: 40px;
    height: 40px;
    border: 3px solid var(--color-border);
    border-top-color: var(--color-accent);
    border-radius: 50%;
    animation: spin 1s linear infinite;
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>






