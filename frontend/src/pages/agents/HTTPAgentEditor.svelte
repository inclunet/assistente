<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { 
    CreateHTTPAgentFull, 
    GetHTTPAgentFull, 
    UpdateHTTPAgentFull, 
    DeleteHTTPAgentFull,
    CreateHTTPEndpoint,
    UpdateHTTPEndpoint,
    DeleteHTTPEndpoint,
    TestHTTPEndpoint
  } from '../../../wailsjs/go/main/App.js';
  import EndpointEditor from './EndpointEditor.svelte';
  import { Modal } from '../../components/modal';
  
  export let agentConfigId = null;  // Se fornecido, carrega agent existente
  export let onClose = null;
  
  const dispatch = createEventDispatcher();
  
  // Estado do agent
  let loading = true;
  let saving = false;
  let error = '';
  let httpAgentId = null;
  
  // Campos do AgentConfig
  let name = '';
  let displayName = '';
  let description = '';
  let model = 'gpt-4o-mini';
  let systemPrompt = '';
  let enabled = true;
  
  // Campos específicos do HTTPAgent
  let baseURL = '';
  let authType = 'none';
  let authConfig = {};
  let defaultHeaders = [];
  let timeoutSeconds = 30;
  let retryCount = 3;
  let endpoints = [];
  
  // Estado de edição
  let editingEndpoint = null;
  let showEndpointEditor = false;
  let showTestModal = false;
  let testEndpoint = null;
  let testParams = '';
  let testResult = '';
  let testError = '';
  let testing = false;
  
  // Tipos de autenticação
  const authTypes = [
    { value: 'none', label: 'Sem autenticação' },
    { value: 'api_key', label: 'API Key' },
    { value: 'bearer', label: 'Bearer Token' },
    { value: 'basic', label: 'HTTP Basic' },
    { value: 'oauth2', label: 'OAuth 2.0' },
  ];
  
  // Tipos de grant OAuth2
  const oauth2GrantTypes = [
    { value: 'client_credentials', label: 'Client Credentials' },
    { value: 'authorization_code', label: 'Authorization Code (requer interação)' },
  ];
  
  // Modelos disponíveis
  const availableModels = [
    'gpt-4o-mini',
    'gpt-4o',
    'gpt-4-turbo',
    'gpt-3.5-turbo',
  ];
  
  onMount(async () => {
    if (agentConfigId) {
      await loadAgent();
    } else {
      loading = false;
    }
  });
  
  async function loadAgent() {
    loading = true;
    error = '';
    
    try {
      const agent = await GetHTTPAgentFull(agentConfigId);
      
      name = agent.name;
      displayName = agent.display_name;
      description = agent.description;
      model = agent.model || 'gpt-4o-mini';
      systemPrompt = agent.system_prompt || '';
      enabled = agent.enabled;
      
      httpAgentId = agent.http_agent_id;
      baseURL = agent.base_url;
      authType = agent.auth_type || 'none';
      timeoutSeconds = agent.timeout_seconds || 30;
      retryCount = agent.retry_count || 3;
      
      try {
        authConfig = agent.auth_config ? JSON.parse(agent.auth_config) : {};
      } catch (e) {
        authConfig = {};
      }
      
      try {
        const headers = agent.default_headers ? JSON.parse(agent.default_headers) : {};
        defaultHeaders = Object.entries(headers).map(([key, value]) => ({ key, value }));
      } catch (e) {
        defaultHeaders = [];
      }
      
      endpoints = agent.endpoints || [];
    } catch (err) {
      error = 'Erro ao carregar agente: ' + (err.message || err);
    } finally {
      loading = false;
    }
  }
  
  async function handleSave() {
    if (!name.trim()) {
      error = 'Nome é obrigatório';
      return;
    }
    if (!displayName.trim()) {
      error = 'Nome de exibição é obrigatório';
      return;
    }
    if (!baseURL.trim()) {
      error = 'URL base é obrigatória';
      return;
    }
    
    saving = true;
    error = '';
    
    try {
      // Converte headers para JSON
      const headersObj = {};
      for (const h of defaultHeaders) {
        if (h.key.trim()) {
          headersObj[h.key.trim()] = h.value;
        }
      }
      const headersJSON = JSON.stringify(headersObj);
      const authConfigJSON = JSON.stringify(authConfig);
      
      if (agentConfigId) {
        // Atualiza existente
        await UpdateHTTPAgentFull(
          agentConfigId,
          displayName,
          description,
          model,
          systemPrompt,
          enabled,
          baseURL,
          authType,
          authConfigJSON,
          headersJSON,
          timeoutSeconds,
          retryCount
        );
      } else {
        // Cria novo
        const result = await CreateHTTPAgentFull(
          name,
          displayName,
          description,
          model,
          systemPrompt,
          enabled,
          baseURL,
          authType,
          authConfigJSON,
          headersJSON,
          timeoutSeconds,
          retryCount
        );
        agentConfigId = result.id;
        httpAgentId = result.http_agent_id;
      }
      
      dispatch('saved');
      
      if (onClose) {
        onClose();
      }
    } catch (err) {
      error = 'Erro ao salvar: ' + (err.message || err);
    } finally {
      saving = false;
    }
  }
  
  async function handleDelete() {
    if (!confirm('Tem certeza que deseja excluir este agente HTTP e todos os seus endpoints?')) {
      return;
    }
    
    try {
      await DeleteHTTPAgentFull(agentConfigId);
      dispatch('deleted');
      if (onClose) {
        onClose();
      }
    } catch (err) {
      error = 'Erro ao excluir: ' + (err.message || err);
    }
  }
  
  function addHeader() {
    defaultHeaders = [...defaultHeaders, { key: '', value: '' }];
  }
  
  function removeHeader(index) {
    defaultHeaders = defaultHeaders.filter((_, i) => i !== index);
  }
  
  function openEndpointEditor(endpoint = null) {
    editingEndpoint = endpoint;
    showEndpointEditor = true;
  }
  
  async function handleEndpointSave(endpointData) {
    try {
      if (endpointData.id) {
        // Atualiza existente
        await UpdateHTTPEndpoint(
          endpointData.id,
          endpointData.name,
          endpointData.description,
          endpointData.method,
          endpointData.path_template,
          endpointData.query_template,
          endpointData.headers_json,
          endpointData.body_template,
          endpointData.parameters,
          endpointData.response_template
        );
      } else {
        // Cria novo
        await CreateHTTPEndpoint(
          httpAgentId,
          endpointData.name,
          endpointData.description,
          endpointData.method,
          endpointData.path_template,
          endpointData.query_template,
          endpointData.headers_json,
          endpointData.body_template,
          endpointData.parameters,
          endpointData.response_template
        );
      }
      
      showEndpointEditor = false;
      editingEndpoint = null;
      await loadAgent();  // Recarrega para atualizar lista
    } catch (err) {
      throw err;  // Propaga para o EndpointEditor mostrar
    }
  }
  
  async function handleEndpointDelete(endpoint) {
    if (!confirm(`Excluir endpoint "${endpoint.name}"?`)) {
      return;
    }
    
    try {
      await DeleteHTTPEndpoint(endpoint.id);
      await loadAgent();
    } catch (err) {
      error = 'Erro ao excluir endpoint: ' + (err.message || err);
    }
  }
  
  function openTestModal(endpoint) {
    testEndpoint = endpoint;
    testParams = '{}';
    testResult = '';
    testError = '';
    showTestModal = true;
  }
  
  async function runTest() {
    testing = true;
    testResult = '';
    testError = '';
    
    try {
      const result = await TestHTTPEndpoint(httpAgentId, testEndpoint.name, testParams);
      testResult = result;
    } catch (err) {
      testError = err.message || err;
    } finally {
      testing = false;
    }
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
</script>

<div class="http-agent-editor">
  {#if loading}
    <div class="loading">Carregando...</div>
  {:else}
    <div class="editor-header">
      <h2>{agentConfigId ? 'Editar HTTP Agent' : 'Novo HTTP Agent'}</h2>
      {#if agentConfigId}
        <button class="btn-delete" on:click={handleDelete}>
          🗑️ Excluir
        </button>
      {/if}
    </div>
    
    {#if error}
      <div class="error-message" role="alert">{error}</div>
    {/if}
    
    <div class="editor-content">
      <!-- Informações Básicas -->
      <section class="form-section">
        <h3>Informações Básicas</h3>
        
        <div class="form-row">
          <div class="form-group flex-1">
            <label for="agent-name">Nome (ID)</label>
            <input 
              type="text"
              id="agent-name"
              bind:value={name}
              placeholder="meu_api"
              disabled={!!agentConfigId}
              class="input-mono"
            />
          </div>
          <div class="form-group flex-1">
            <label for="agent-display-name">Nome de Exibição</label>
            <input 
              type="text"
              id="agent-display-name"
              bind:value={displayName}
              placeholder="Minha API"
            />
          </div>
        </div>
        
        <div class="form-group">
          <label for="agent-description">Descrição</label>
          <textarea 
            id="agent-description"
            bind:value={description}
            rows="2"
            placeholder="Descreva o que este agente faz..."
          ></textarea>
        </div>
        
        <div class="form-row">
          <div class="form-group">
            <label for="agent-model">Modelo LLM</label>
            <select id="agent-model" bind:value={model}>
              {#each availableModels as m}
                <option value={m}>{m}</option>
              {/each}
            </select>
          </div>
          <div class="form-group checkbox-group">
            <label>
              <input type="checkbox" bind:checked={enabled} />
              Habilitado
            </label>
          </div>
        </div>
      </section>
      
      <!-- Conexão -->
      <section class="form-section">
        <h3>Conexão</h3>
        
        <div class="form-group">
          <label for="base-url">URL Base</label>
          <input 
            type="text"
            id="base-url"
            bind:value={baseURL}
            placeholder="https://api.example.com/v1"
            class="input-mono"
          />
        </div>
        
        <div class="form-row">
          <div class="form-group flex-1">
            <label for="auth-type">Autenticação</label>
            <select id="auth-type" bind:value={authType}>
              {#each authTypes as at}
                <option value={at.value}>{at.label}</option>
              {/each}
            </select>
          </div>
          
          {#if authType === 'api_key'}
            <div class="form-group flex-1">
              <label for="auth-header">Nome do Header/Param</label>
              <input 
                type="text"
                id="auth-header"
                bind:value={authConfig.header_name}
                placeholder="X-API-Key"
              />
            </div>
            <div class="form-group flex-1">
              <label for="auth-value-env">Variável de Ambiente</label>
              <input 
                type="text"
                id="auth-value-env"
                bind:value={authConfig.value_env}
                placeholder="MY_API_KEY"
              />
            </div>
          {:else if authType === 'bearer'}
            <div class="form-group flex-1">
              <label for="token-env">Variável de Ambiente do Token</label>
              <input 
                type="text"
                id="token-env"
                bind:value={authConfig.token_env}
                placeholder="API_TOKEN"
              />
            </div>
          {:else if authType === 'basic'}
            <div class="form-group flex-1">
              <label for="username-env">Var. Ambiente Usuário</label>
              <input 
                type="text"
                id="username-env"
                bind:value={authConfig.username_env}
                placeholder="API_USER"
              />
            </div>
            <div class="form-group flex-1">
              <label for="password-env">Var. Ambiente Senha</label>
              <input 
                type="text"
                id="password-env"
                bind:value={authConfig.password_env}
                placeholder="API_PASS"
              />
            </div>
          {:else if authType === 'oauth2'}
            <div class="form-group flex-1">
              <label for="oauth-grant-type">Tipo de Grant</label>
              <select id="oauth-grant-type" bind:value={authConfig.grant_type}>
                {#each oauth2GrantTypes as gt}
                  <option value={gt.value}>{gt.label}</option>
                {/each}
              </select>
            </div>
          {/if}
        </div>
        
        {#if authType === 'oauth2'}
          <div class="oauth2-config">
            <div class="form-row">
              <div class="form-group flex-1">
                <label for="oauth-token-url">Token URL</label>
                <input 
                  type="text"
                  id="oauth-token-url"
                  bind:value={authConfig.token_url}
                  placeholder="https://oauth.example.com/token"
                  class="input-mono"
                />
              </div>
            </div>
            
            {#if authConfig.grant_type === 'authorization_code'}
              <div class="form-row">
                <div class="form-group flex-1">
                  <label for="oauth-authorize-url">Authorize URL</label>
                  <input 
                    type="text"
                    id="oauth-authorize-url"
                    bind:value={authConfig.authorize_url}
                    placeholder="https://oauth.example.com/authorize"
                    class="input-mono"
                  />
                </div>
              </div>
            {/if}
            
            <div class="form-row">
              <div class="form-group flex-1">
                <label for="oauth-client-id-env">Var. Ambiente Client ID</label>
                <input 
                  type="text"
                  id="oauth-client-id-env"
                  bind:value={authConfig.client_id_env}
                  placeholder="OAUTH_CLIENT_ID"
                />
              </div>
              <div class="form-group flex-1">
                <label for="oauth-client-secret-env">Var. Ambiente Client Secret</label>
                <input 
                  type="text"
                  id="oauth-client-secret-env"
                  bind:value={authConfig.client_secret_env}
                  placeholder="OAUTH_CLIENT_SECRET"
                />
              </div>
            </div>
            
            <div class="form-row">
              <div class="form-group flex-1">
                <label for="oauth-scopes">Scopes (separados por espaço)</label>
                <input 
                  type="text"
                  id="oauth-scopes"
                  bind:value={authConfig.scopes}
                  placeholder="read write admin"
                />
              </div>
              <div class="form-group flex-1">
                <label for="oauth-audience">Audience (opcional)</label>
                <input 
                  type="text"
                  id="oauth-audience"
                  bind:value={authConfig.audience}
                  placeholder="https://api.example.com"
                />
              </div>
            </div>
            
            <div class="form-group checkbox-inline">
              <label>
                <input 
                  type="checkbox" 
                  bind:checked={authConfig.send_credentials_in_body}
                />
                Enviar credenciais no body (em vez de Basic Auth header)
              </label>
            </div>
          </div>
        {/if}
        
        <div class="form-row">
          <div class="form-group">
            <label for="timeout">Timeout (segundos)</label>
            <input 
              type="number"
              id="timeout"
              bind:value={timeoutSeconds}
              min="1"
              max="300"
            />
          </div>
          <div class="form-group">
            <label for="retry">Tentativas</label>
            <input 
              type="number"
              id="retry"
              bind:value={retryCount}
              min="0"
              max="10"
            />
          </div>
        </div>
      </section>
      
      <!-- Headers Padrão -->
      <section class="form-section">
        <h3>
          Headers Padrão
          <button class="btn-add-small" on:click={addHeader} type="button">+ Adicionar</button>
        </h3>
        
        {#if defaultHeaders.length === 0}
          <p class="empty-hint">Nenhum header padrão. Headers são enviados em todas as requisições.</p>
        {:else}
          <div class="headers-list">
            {#each defaultHeaders as header, index}
              <div class="header-row">
                <input 
                  type="text"
                  bind:value={header.key}
                  placeholder="Content-Type"
                />
                <input 
                  type="text"
                  bind:value={header.value}
                  placeholder="application/json"
                />
                <button class="btn-remove" on:click={() => removeHeader(index)} type="button">✕</button>
              </div>
            {/each}
          </div>
        {/if}
      </section>
      
      <!-- Endpoints -->
      <section class="form-section">
        <h3>
          Endpoints (Funções)
          {#if httpAgentId}
            <button class="btn-add-small" on:click={() => openEndpointEditor(null)} type="button">+ Novo Endpoint</button>
          {/if}
        </h3>
        
        {#if !httpAgentId}
          <p class="empty-hint">Salve o agente primeiro para adicionar endpoints.</p>
        {:else if endpoints.length === 0}
          <p class="empty-hint">Nenhum endpoint configurado. Adicione endpoints para que o LLM possa chamar a API.</p>
        {:else}
          <div class="endpoints-list">
            {#each endpoints as endpoint}
              <div class="endpoint-card">
                <div class="endpoint-header">
                  <span class="method-badge" style="background: {getMethodColor(endpoint.method)}">
                    {endpoint.method}
                  </span>
                  <span class="endpoint-name">{endpoint.name}</span>
                </div>
                <div class="endpoint-path">{endpoint.path_template}</div>
                <div class="endpoint-description">{endpoint.description || 'Sem descrição'}</div>
                <div class="endpoint-actions">
                  <button class="btn-small" on:click={() => openTestModal(endpoint)}>🧪 Testar</button>
                  <button class="btn-small" on:click={() => openEndpointEditor(endpoint)}>✏️ Editar</button>
                  <button class="btn-small btn-danger" on:click={() => handleEndpointDelete(endpoint)}>🗑️</button>
                </div>
              </div>
            {/each}
          </div>
        {/if}
      </section>
    </div>
    
    <div class="editor-footer">
      <button class="btn-secondary" on:click={onClose} disabled={saving}>
        Cancelar
      </button>
      <button class="btn-primary" on:click={handleSave} disabled={saving}>
        {saving ? 'Salvando...' : 'Salvar Agente'}
      </button>
    </div>
  {/if}
</div>

<!-- Modal do EndpointEditor -->
<Modal 
  title={editingEndpoint ? 'Editar Endpoint' : 'Novo Endpoint'} 
  open={showEndpointEditor} 
  on:close={() => { showEndpointEditor = false; editingEndpoint = null; }}
>
  <EndpointEditor 
    endpoint={editingEndpoint}
    onSave={handleEndpointSave}
    onCancel={() => { showEndpointEditor = false; editingEndpoint = null; }}
    onTest={(ep) => { showEndpointEditor = false; openTestModal(ep); }}
  />
</Modal>

<!-- Modal de Teste -->
<Modal 
  title="🧪 Testar Endpoint: {testEndpoint?.name}" 
  open={showTestModal} 
  on:close={() => showTestModal = false}
>
  <div class="test-modal">
    <div class="form-group">
      <label>Parâmetros (JSON)</label>
      <textarea 
        bind:value={testParams}
        rows="4"
        placeholder={'{\n  "param1": "valor1"\n}'}
        class="input-mono"
      ></textarea>
    </div>
    
    <button class="btn-primary btn-full" on:click={runTest} disabled={testing}>
      {testing ? 'Executando...' : '▶️ Executar'}
    </button>
    
    {#if testError}
      <div class="test-error">
        <strong>Erro:</strong> {testError}
      </div>
    {/if}
    
    {#if testResult}
      <div class="test-result">
        <strong>Resultado:</strong>
        <pre>{testResult}</pre>
      </div>
    {/if}
  </div>
</Modal>

<style>
  .http-agent-editor {
    display: flex;
    flex-direction: column;
    height: 100%;
    max-height: 80vh;
  }
  
  .loading {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--spacing-xl);
    color: var(--color-text-muted);
  }
  
  .editor-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-bottom: var(--spacing-md);
    border-bottom: 1px solid var(--color-border);
  }
  
  .editor-header h2 {
    margin: 0;
    font-size: var(--font-size-xl);
  }
  
  .btn-delete {
    background: none;
    border: 1px solid var(--color-error);
    color: var(--color-error);
    padding: var(--spacing-xs) var(--spacing-sm);
    border-radius: var(--border-radius);
    cursor: pointer;
  }
  
  .error-message {
    margin: var(--spacing-md) 0;
    padding: var(--spacing-sm);
    background: rgba(248, 81, 73, 0.1);
    border: 1px solid var(--color-error);
    border-radius: var(--border-radius);
    color: var(--color-error);
  }
  
  .editor-content {
    flex: 1;
    overflow-y: auto;
    padding: var(--spacing-md) 0;
  }
  
  .form-section {
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    padding: var(--spacing-md);
    margin-bottom: var(--spacing-md);
  }
  
  .form-section h3 {
    margin: 0 0 var(--spacing-md);
    font-size: var(--font-size-md);
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
  }
  
  .btn-add-small {
    font-size: var(--font-size-xs);
    padding: 2px 8px;
    background: var(--color-accent);
    color: white;
    border: none;
    border-radius: var(--border-radius);
    cursor: pointer;
  }
  
  .form-row {
    display: flex;
    gap: var(--spacing-md);
    flex-wrap: wrap;
  }
  
  .flex-1 {
    flex: 1;
    min-width: 150px;
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
  .form-group select,
  .form-group textarea {
    width: 100%;
    padding: var(--spacing-sm);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
  }
  
  .input-mono {
    font-family: 'Fira Code', monospace;
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
  
  .checkbox-inline {
    margin-top: var(--spacing-sm);
  }
  
  .checkbox-inline label {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    cursor: pointer;
    font-size: var(--font-size-sm);
  }
  
  .checkbox-inline input {
    width: auto;
  }
  
  .oauth2-config {
    margin-top: var(--spacing-md);
    padding: var(--spacing-md);
    background: var(--color-bg-tertiary);
    border-radius: var(--border-radius);
  }
  
  .empty-hint {
    color: var(--color-text-muted);
    font-size: var(--font-size-sm);
    margin: 0;
  }
  
  .headers-list {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-sm);
  }
  
  .header-row {
    display: flex;
    gap: var(--spacing-sm);
  }
  
  .header-row input {
    flex: 1;
    padding: var(--spacing-xs) var(--spacing-sm);
  }
  
  .btn-remove {
    background: none;
    border: none;
    color: var(--color-text-muted);
    cursor: pointer;
    padding: var(--spacing-xs);
  }
  
  .btn-remove:hover {
    color: var(--color-error);
  }
  
  .endpoints-list {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-sm);
  }
  
  .endpoint-card {
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    padding: var(--spacing-sm);
    background: var(--color-bg-tertiary);
  }
  
  .endpoint-header {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    margin-bottom: var(--spacing-xs);
  }
  
  .method-badge {
    padding: 2px 6px;
    border-radius: 3px;
    font-size: var(--font-size-xs);
    font-weight: 600;
    color: white;
  }
  
  .endpoint-name {
    font-weight: 600;
    font-family: 'Fira Code', monospace;
  }
  
  .endpoint-path {
    font-size: var(--font-size-sm);
    color: var(--color-text-muted);
    font-family: 'Fira Code', monospace;
  }
  
  .endpoint-description {
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    margin: var(--spacing-xs) 0;
  }
  
  .endpoint-actions {
    display: flex;
    gap: var(--spacing-xs);
    margin-top: var(--spacing-sm);
  }
  
  .btn-small {
    padding: 2px 8px;
    font-size: var(--font-size-xs);
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    cursor: pointer;
    color: var(--color-text-primary);
  }
  
  .btn-small.btn-danger {
    color: var(--color-error);
    border-color: var(--color-error);
  }
  
  .editor-footer {
    display: flex;
    justify-content: flex-end;
    gap: var(--spacing-sm);
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
  
  .btn-secondary {
    background: var(--color-bg-tertiary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border);
  }
  
  /* Test Modal */
  .test-modal {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-md);
  }
  
  .btn-full {
    width: 100%;
  }
  
  .test-error {
    padding: var(--spacing-sm);
    background: rgba(248, 81, 73, 0.1);
    border-radius: var(--border-radius);
    color: var(--color-error);
  }
  
  .test-result {
    padding: var(--spacing-sm);
    background: var(--color-bg-tertiary);
    border-radius: var(--border-radius);
  }
  
  .test-result pre {
    margin: var(--spacing-sm) 0 0;
    overflow-x: auto;
    font-size: var(--font-size-sm);
    max-height: 300px;
    overflow-y: auto;
  }
</style>



