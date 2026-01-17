<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { GetRegisteredAgents, GetAllAgentConfigs, SaveOrUpdateAgentConfig, TestAgent, GetAllHTTPAgentsFull, GetAllMCPAgentsFull, DeleteMCPAgentFull } from '../../../wailsjs/go/main/App.js';
  import HTTPAgentEditor from './HTTPAgentEditor.svelte';
  import MCPAgentEditor from './MCPAgentEditor.svelte';
  import FileAgentConfig from './FileAgentConfig.svelte';
  import ImportSpecModal from './ImportSpecModal.svelte';
  import { Modal } from '../../components/modal';
  import { DataGrid } from '../../components/grid';

  export let label = 'Gerenciador de Agentes';

  const dispatch = createEventDispatcher();
  
  // Atalho local Ctrl+N para novo HTTP Agent
  function handleLocalKeyDown(event) {
    if (event.ctrlKey && event.key.toLowerCase() === 'n') {
      event.preventDefault();
      openHTTPAgentEditor(null);
    }
  }

  let allAgents = []; // Lista unificada de todos os agentes
  let loading = true;
  let error = '';
  
  // Estado do HTTP Agent Editor
  let showHTTPEditor = false;
  let editingHTTPAgentId = null;
  
  // Estado do MCP Agent Editor
  let showMCPEditor = false;
  let editingMCPAgentId = null;
  
  // Estado do FileAgent Config
  let showFileAgentConfig = false;
  
  // Estado do Import Modal
  let showImportModal = false;
  
  // Estado do formulário de edição
  let showForm = false;
  let editingAgent = null;
  let formDisplayName = '';
  let formDescription = '';
  let formModel = '';
  let formSystemPrompt = '';
  let formEnabled = true;
  let formError = '';
  let saving = false;

  // Estado do playground
  let showPlayground = false;
  let playgroundAgent = null;
  let playgroundTask = '';
  let playgroundResult = '';
  let playgroundError = '';
  let testing = false;

  // Modelos disponíveis
  const availableModels = [
    'gpt-4o-mini',
    'gpt-4o',
    'gpt-4-turbo',
    'gpt-3.5-turbo',
    'claude-3-5-sonnet-20241022',
    'claude-3-haiku-20240307'
  ];

  // Grid unificado
  let agentGridComponent;

  // Colunas unificadas para todos os agentes
  const agentColumns = [
    { 
      key: 'type_icon', 
      label: 'Tipo',
      width: '60px',
      format: (value, item) => {
        if (item.agent_type === 'http') return '🌐';
        if (item.agent_type === 'mcp') return '🔌';
        return '⚙️';
      }
    },
    { 
      key: 'display_name', 
      label: 'Nome',
      format: (value, item) => value || item.name
    },
    { 
      key: 'description', 
      label: 'Descrição',
      truncate: true,
      format: (value) => value || 'Sem descrição'
    },
    { 
      key: 'model', 
      label: 'Modelo',
      width: '130px',
      format: (value) => value || 'gpt-4o-mini'
    },
    { 
      key: 'enabled', 
      label: 'Status',
      width: '100px',
      format: (value) => value !== false ? '✅ Ativo' : '⛔ Inativo'
    },
    { 
      key: 'test', 
      label: 'Testar',
      width: '80px',
      action: true,
      actionIcon: '▶️'
    },
    { 
      key: 'config', 
      label: 'Config',
      width: '80px',
      action: true,
      actionIcon: '📁',
      showIf: (item) => item.name === 'file_manager'
    },
    { 
      key: 'edit', 
      label: 'Editar',
      width: '80px',
      action: true,
      actionIcon: '⚙️'
    }
  ];

  // Colunas para HTTP agents
  const httpColumns = [
    { 
      key: 'display_name', 
      label: 'Nome',
      format: (value, item) => value || item.name
    },
    { 
      key: 'base_url', 
      label: 'URL Base',
      truncate: true
    },
    { 
      key: 'endpoints', 
      label: 'Endpoints',
      width: '100px',
      format: (value) => `${value?.length || 0} endpoints`
    },
    { 
      key: 'model', 
      label: 'Modelo',
      width: '130px',
      format: (value) => value || 'gpt-4o-mini'
    },
    { 
      key: 'edit', 
      label: 'Editar',
      width: '80px',
      action: true,
      actionIcon: '⚙️'
    }
  ];

  // Colunas para MCP agents
  const mcpColumns = [
    { 
      key: 'name', 
      label: 'Nome',
      format: (value, item) => item.agent_config?.display_name || item.agent_config?.name || value
    },
    { 
      key: 'server_command', 
      label: 'Comando',
      truncate: true
    },
    { 
      key: 'status', 
      label: 'Status',
      width: '120px',
      format: (value, item) => item.agent_config?.enabled ? '✅ Ativo' : '⛔ Inativo'
    },
    { 
      key: 'auto_connect', 
      label: 'Auto',
      width: '80px',
      format: (value) => value ? '🔗' : '—'
    },
    { 
      key: 'edit', 
      label: 'Editar',
      width: '80px',
      action: true,
      actionIcon: '⚙️',
      disabled: (item) => item.agent_type === 'internal' && !['faq', 'memory', 'file_manager'].includes(item.name)
    },
    { 
      key: 'delete', 
      label: 'Excluir',
      width: '80px',
      action: true,
      actionIcon: '🗑️',
      showIf: (item) => item.agent_type === 'http' || item.agent_type === 'mcp'
    }
  ];

  onMount(async () => {
    window.addEventListener('keydown', handleLocalKeyDown);
    await loadAgents();
  });
  
  onDestroy(() => {
    window.removeEventListener('keydown', handleLocalKeyDown);
  });

  export function focusList() {
    setTimeout(() => {
      agentGridComponent?.focus();
    }, 50);
  }

  async function loadAgents() {
    loading = true;
    error = '';
    try {
      // Lista unificada de todos os agentes
      allAgents = [];
      
      // 1. Carrega agentes internos (sistema)
      const internalAgents = await GetRegisteredAgents() || [];
      
      try {
        const configs = await GetAllAgentConfigs() || [];
        const mappedInternals = internalAgents.map(agent => {
          const savedConfig = configs.find(c => c.name === agent.name);
          if (savedConfig) {
            return {
              ...agent,
              agent_type: 'internal',
              display_name: savedConfig.display_name || agent.display_name,
              description: savedConfig.description || agent.description,
              model: savedConfig.model || agent.model,
              system_prompt: savedConfig.system_prompt || agent.system_prompt,
              enabled: savedConfig.enabled,
              id: savedConfig.id,
              _config_id: savedConfig.id
            };
          }
          return {
            ...agent,
            agent_type: 'internal'
          };
        });
        allAgents = [...allAgents, ...mappedInternals];
      } catch (e) {
        console.log('Usando configurações padrão dos agentes internos');
        allAgents = [...allAgents, ...internalAgents.map(a => ({ ...a, agent_type: 'internal' }))];
      }
      
      // 2. Carrega HTTP agents
      try {
        const httpAgents = await GetAllHTTPAgentsFull() || [];
        const mappedHTTP = httpAgents.map(agent => ({
          ...agent,
          agent_type: 'http',
          _http_agent_id: agent.http_agent_id
        }));
        allAgents = [...allAgents, ...mappedHTTP];
      } catch (e) {
        console.log('Nenhum HTTP agent encontrado');
      }
      
      // 3. Carrega MCP agents
      try {
        const mcpAgents = await GetAllMCPAgentsFull() || [];
        console.log('MCP Agents carregados:', mcpAgents);
        const mappedMCP = mcpAgents.map(agent => ({
          ...agent,
          agent_type: 'mcp',
          // Normaliza os campos do agent_config para o nível superior
          id: agent.agent_config?.id || agent.id,
          name: agent.agent_config?.name || agent.name,
          display_name: agent.agent_config?.display_name || agent.agent_config?.name || agent.name,
          description: agent.agent_config?.description || '',
          model: agent.agent_config?.model || 'gpt-4o-mini',
          enabled: agent.agent_config?.enabled !== false,
          _mcp_agent_id: agent.id,
          _config_id: agent.agent_config?.id
        }));
        allAgents = [...allAgents, ...mappedMCP];
      } catch (e) {
        console.error('Erro ao carregar MCP Agents:', e);
      }
      
      // Ordena: internos primeiro, depois HTTP, depois MCP
      allAgents.sort((a, b) => {
        const typeOrder = { internal: 0, http: 1, mcp: 2 };
        const typeCompare = typeOrder[a.agent_type] - typeOrder[b.agent_type];
        if (typeCompare !== 0) return typeCompare;
        
        // Dentro do mesmo tipo, ordena por nome
        const nameA = (a.display_name || a.name || '').toLowerCase();
        const nameB = (b.display_name || b.name || '').toLowerCase();
        return nameA.localeCompare(nameB);
      });
      
    } catch (err) {
      error = 'Erro ao carregar agentes: ' + (err.message || err);
    } finally {
      loading = false;
    }
  }

  function openEditForm(agent) {
    editingAgent = agent;
    formDisplayName = agent.display_name || agent.name;
    formDescription = agent.description || '';
    formModel = agent.model || 'gpt-4o-mini';
    formSystemPrompt = agent.system_prompt || '';
    formEnabled = agent.enabled !== false;
    formError = '';
    showForm = true;
  }

  function closeForm() {
    showForm = false;
    editingAgent = null;
    formError = '';
  }

  async function handleSubmit() {
    saving = true;
    formError = '';

    try {
      await SaveOrUpdateAgentConfig(
        editingAgent.name,
        formDisplayName,
        formDescription,
        editingAgent.agent_type || 'internal',
        formModel,
        formSystemPrompt,
        '', // config
        formEnabled
      );
      
      closeForm();
      await loadAgents();
    } catch (err) {
      formError = 'Erro ao salvar: ' + err;
    } finally {
      saving = false;
    }
  }

  function openPlayground(agent) {
    playgroundAgent = agent;
    playgroundTask = '';
    playgroundResult = '';
    playgroundError = '';
    showPlayground = true;
  }

  function closePlayground() {
    showPlayground = false;
    playgroundAgent = null;
  }

  async function handleTest() {
    if (!playgroundTask.trim()) {
      playgroundError = 'Digite uma tarefa para testar';
      return;
    }

    testing = true;
    playgroundResult = '';
    playgroundError = '';

    try {
      const result = await TestAgent(playgroundAgent.name, playgroundTask);
      playgroundResult = result;
    } catch (err) {
      playgroundError = 'Erro: ' + (err.message || err);
    } finally {
      testing = false;
    }
  }

  function openHTTPAgentEditor(agentConfigId = null) {
    editingHTTPAgentId = agentConfigId;
    showHTTPEditor = true;
  }
  
  function closeHTTPAgentEditor() {
    showHTTPEditor = false;
    editingHTTPAgentId = null;
    loadAgents();
  }

  function openMCPAgentEditor(mcpAgentId = null) {
    editingMCPAgentId = mcpAgentId;
    showMCPEditor = true;
  }
  
  function closeMCPAgentEditor() {
    showMCPEditor = false;
    editingMCPAgentId = null;
    loadAgents();
  }
  
  function handleImportComplete() {
    showImportModal = false;
    loadAgents();
  }

  async function deleteMCPAgent(mcpAgentId) {
    if (!confirm('Tem certeza que deseja excluir este MCP Agent?')) {
      return;
    }
    
    try {
      await DeleteMCPAgentFull(mcpAgentId);
      await loadAgents();
    } catch (err) {
      error = 'Erro ao excluir MCP Agent: ' + (err.message || err);
    }
  }

  function handleAgentCellAction(event) {
    const { item, column } = event.detail;
    
    if (column.key === 'test') {
      if (item.enabled !== false && item.agent_type === 'internal') {
        openPlayground(item);
      }
    } else if (column.key === 'config') {
      // Abre configuração específica do agente
      if (item.name === 'file_manager') {
        showFileAgentConfig = true;
      }
    } else if (column.key === 'edit') {
      // Roteamento baseado no tipo
      if (item.agent_type === 'http') {
        openHTTPAgentEditor(item.id);
      } else if (item.agent_type === 'mcp') {
        openMCPAgentEditor(item._mcp_agent_id || item.id);
      } else if (item.agent_type === 'internal') {
        openEditForm(item);
      }
    } else if (column.key === 'delete') {
      // Apenas HTTP e MCP podem ser deletados
      if (item.agent_type === 'mcp') {
        deleteMCPAgent(item._mcp_agent_id || item.id);
      } else if (item.agent_type === 'http') {
        deleteHTTPAgent(item.id);
      }
    }
  }

  function handleAgentActivate(event) {
    const item = event.detail.item;
    
    // Duplo clique: abre o editor apropriado
    if (item.agent_type === 'http') {
      openHTTPAgentEditor(item.id);
    } else if (item.agent_type === 'mcp') {
      openMCPAgentEditor(item._mcp_agent_id || item.id);
    } else if (item.agent_type === 'internal') {
      openEditForm(item);
    }
  }

  async function deleteHTTPAgent(agentId) {
    if (!confirm('Tem certeza que deseja excluir este HTTP Agent?')) {
      return;
    }
    
    try {
      const { DeleteHTTPAgentFull } = await import('../../../wailsjs/go/main/App.js');
      await DeleteHTTPAgentFull(agentId);
      await loadAgents();
    } catch (err) {
      error = 'Erro ao excluir HTTP Agent: ' + (err.message || err);
    }
  }

</script>

<div class="agent-manager">
  <header class="agent-header">
    <div>
      <h2 id="agent-heading">{label}</h2>
      <p class="subtitle">Configure os agentes inteligentes do assistente</p>
    </div>
    <div class="header-actions">
      <button class="btn-import" on:click={() => showImportModal = true}>
        📥 Importar OpenAPI/Postman
      </button>
      <button class="btn-secondary" on:click={() => openMCPAgentEditor(null)}>
        + Novo MCP Agent
      </button>
      <button class="btn-primary" on:click={() => openHTTPAgentEditor(null)}>
        + Novo HTTP Agent
      </button>
    </div>
  </header>

  {#if error}
    <div class="error" role="alert">{error}</div>
  {/if}

  {#if loading}
    <div class="loading" role="status" aria-live="polite">
      <span class="loading-spinner" aria-hidden="true"></span>
      Carregando agentes...
    </div>
  {:else if allAgents.length === 0}
    <p class="empty">Nenhum agente registrado.</p>
  {:else}
    <!-- Grid Unificado de Agentes -->
    <section aria-labelledby="all-agents-heading">
      <p class="sr-only" id="all-agents-heading">
        Todos os agentes. Use setas verticais para navegar entre agentes e setas horizontais para navegar entre os campos.
      </p>
      
      <DataGrid
        bind:this={agentGridComponent}
        items={allAgents}
        columns={agentColumns}
        label="Todos os agentes do sistema"
        getItemId={(a) => `${a.agent_type}-${a.id || a.name}`}
        multiSelect={false}
        on:activate={handleAgentActivate}
        on:cellAction={handleAgentCellAction}
      />
    </section>
  {/if}
</div>

<!-- Modal do HTTP Agent Editor -->
<Modal 
  title={editingHTTPAgentId ? 'Editar HTTP Agent' : 'Novo HTTP Agent'} 
  open={showHTTPEditor} 
  on:close={closeHTTPAgentEditor}
>
  <HTTPAgentEditor 
    agentConfigId={editingHTTPAgentId}
    onClose={closeHTTPAgentEditor}
  />
</Modal>

<!-- Modal do MCP Agent Editor -->
<Modal 
  title={editingMCPAgentId ? 'Editar MCP Agent' : 'Novo MCP Agent'} 
  open={showMCPEditor} 
  on:close={closeMCPAgentEditor}
>
  <MCPAgentEditor 
    mcpAgentId={editingMCPAgentId}
    onClose={closeMCPAgentEditor}
  />
</Modal>

<!-- Modal de Configuração do FileAgent -->
<Modal 
  title="📁 Configuração do File Manager" 
  open={showFileAgentConfig} 
  on:close={() => showFileAgentConfig = false}
>
  <FileAgentConfig />
</Modal>

<!-- Modal de Importação OpenAPI/Postman -->
<ImportSpecModal 
  open={showImportModal}
  on:close={() => showImportModal = false}
  on:imported={handleImportComplete}
/>

<!-- Modal de Edição -->
{#if showForm}
  <div class="modal-overlay" on:click|self={closeForm} on:keydown={(e) => e.key === 'Escape' && closeForm()}>
    <div
      class="modal-content"
      role="dialog"
      aria-modal="true"
      aria-labelledby="agent-form-title"
    >
      <h3 id="agent-form-title">Configurar: {editingAgent?.name}</h3>
      
      <form on:submit|preventDefault={handleSubmit}>
        {#if formError}
          <div class="form-error" role="alert">{formError}</div>
        {/if}
        
        <div class="form-group">
          <label for="agent-display-name">Nome de Exibição</label>
          <input
            id="agent-display-name"
            type="text"
            bind:value={formDisplayName}
            placeholder="Nome amigável do agente"
          />
        </div>

        <div class="form-group">
          <label for="agent-description">Descrição</label>
          <textarea
            id="agent-description"
            bind:value={formDescription}
            rows="2"
            placeholder="O que este agente faz..."
          ></textarea>
        </div>
        
        <div class="form-group">
          <label for="agent-model">Modelo LLM</label>
          <select id="agent-model" bind:value={formModel}>
            {#each availableModels as model}
              <option value={model}>{model}</option>
            {/each}
          </select>
        </div>
        
        <div class="form-group">
          <label for="agent-system-prompt">System Prompt (opcional)</label>
          <textarea
            id="agent-system-prompt"
            bind:value={formSystemPrompt}
            rows="4"
            placeholder="Instruções adicionais para o agente..."
          ></textarea>
        </div>
        
        <div class="form-group checkbox-group">
          <label>
            <input type="checkbox" bind:checked={formEnabled} />
            Habilitado
          </label>
          <p class="form-hint">Agentes desabilitados não serão usados pelo orquestrador.</p>
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

<!-- Modal do Playground -->
{#if showPlayground}
  <div class="modal-overlay" on:click|self={closePlayground} on:keydown={(e) => e.key === 'Escape' && closePlayground()}>
    <div
      class="modal-content playground-modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="playground-title"
    >
      <h3 id="playground-title">🧪 Testar: {playgroundAgent?.display_name || playgroundAgent?.name}</h3>
      
      <div class="form-group">
        <label for="playground-task">Tarefa (linguagem natural)</label>
        <textarea
          id="playground-task"
          bind:value={playgroundTask}
          rows="3"
          placeholder="Descreva a tarefa para o agente executar..."
        ></textarea>
      </div>
      
      <button 
        class="btn-primary btn-full" 
        on:click={handleTest}
        disabled={testing}
      >
        {testing ? 'Executando...' : '▶️ Executar'}
      </button>
      
      {#if playgroundError}
        <div class="playground-error" role="alert">
          <strong>Erro:</strong> {playgroundError}
        </div>
      {/if}
      
      {#if playgroundResult}
        <div class="playground-result">
          <strong>Resultado:</strong>
          <pre>{playgroundResult}</pre>
        </div>
      {/if}
      
      <button class="btn-secondary btn-full" on:click={closePlayground}>
        Fechar
      </button>
    </div>
  </div>
{/if}

<style>
  .agent-manager {
    display: flex;
    flex-direction: column;
    min-height: 300px;
    gap: var(--spacing-md);
  }

  .agent-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    padding-bottom: var(--spacing-md);
    border-bottom: 1px solid var(--color-border);
  }

  .agent-header h2 {
    margin: 0;
    font-size: var(--font-size-xl);
  }

  .header-actions {
    display: flex;
    gap: var(--spacing-sm);
  }

  .subtitle {
    margin: var(--spacing-xs) 0 0;
    font-size: var(--font-size-sm);
    color: var(--color-text-muted);
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
  
  .btn-import {
    background-color: transparent;
    color: var(--color-accent);
    border: 1px solid var(--color-accent);
    border-radius: var(--border-radius);
    padding: var(--spacing-sm) var(--spacing-md);
    cursor: pointer;
  }
  
  .btn-import:hover {
    background-color: rgba(var(--color-accent-rgb), 0.1);
  }

  .btn-primary:hover, .btn-secondary:hover {
    opacity: 0.9;
  }

  .btn-full {
    width: 100%;
    margin-bottom: var(--spacing-sm);
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

  .playground-modal {
    max-width: 600px;
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

  .checkbox-group label {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    cursor: pointer;
  }

  .checkbox-group input {
    width: auto;
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

  .playground-error {
    background: rgba(248, 81, 73, 0.1);
    color: var(--color-error);
    padding: var(--spacing-sm);
    border-radius: var(--border-radius);
    margin-bottom: var(--spacing-sm);
  }

  .playground-result {
    background: var(--color-bg-tertiary);
    padding: var(--spacing-sm);
    border-radius: var(--border-radius);
    margin-bottom: var(--spacing-sm);
  }

  .playground-result pre {
    margin: var(--spacing-sm) 0 0;
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 200px;
    overflow-y: auto;
    font-size: var(--font-size-sm);
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
