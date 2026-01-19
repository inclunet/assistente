<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { 
    GetMCPAgent, 
    GetAgentConfigByID,
    CreateMCPAgentFull, 
    UpdateMCPAgentFull,
    ConnectMCPAgent,
    DisconnectMCPAgent,
    GetMCPAgentStatus,
    TestMCPAgent,
    GetMCPResources,
    GetMCPResourceTemplates,
    ReadMCPResource,
    GetMCPPrompts,
    GetMCPPrompt
  } from '../../../wailsjs/go/main/App.js';

  export let mcpAgentId = null;
  export let onClose = () => {};

  const dispatch = createEventDispatcher();

  // Form data
  let name = '';
  let displayName = '';
  let description = '';
  let model = 'gpt-4o-mini';
  let systemPrompt = '';
  
  // Tipo de transporte
  let transportType = 'stdio';
  
  // Configuração stdio
  let serverCommand = '';
  let serverArgs = '[]';
  let serverEnv = '[]';
  let workingDir = '';
  
  // Configuração HTTP
  let serverURL = '';
  let authType = 'none';
  let authValue = '';
  let httpHeaders = '{}';
  
  // Configuração comum
  let executionMode = 'convert';
  let autoConnect = false;
  let enabled = true;

  // Tipos de transporte disponíveis
  const transportTypes = [
    { value: 'stdio', label: 'Local (stdio)', description: 'Servidor MCP local executado como processo filho.' },
    { value: 'http', label: 'Remoto (HTTP/SSE)', description: 'Servidor MCP remoto via HTTP com Server-Sent Events.' }
  ];

  // Tipos de autenticação HTTP
  const authTypes = [
    { value: 'none', label: 'Nenhuma' },
    { value: 'bearer', label: 'Bearer Token' },
    { value: 'api_key', label: 'API Key' }
  ];

  // Modos de execução disponíveis
  const executionModes = [
    { value: 'convert', label: 'Converter (Padrão)', description: 'Converte tools MCP para formato OpenAI. Compatível com qualquer modelo.' },
    { value: 'native', label: 'Nativo', description: 'Passa tools MCP diretamente. Para modelos com suporte nativo (ex: Claude).' },
    { value: 'passthrough', label: 'Passthrough', description: 'Envia tarefa direto ao servidor MCP. Útil quando o servidor já tem um LLM.' }
  ];

  // Status
  let loading = true;
  let saving = false;
  let error = '';
  let successMessage = '';

  // Connection status
  let connected = false;
  let connecting = false;
  let serverInfo = null;
  let availableTools = [];
  
  // MCP Advanced (Resources, Prompts)
  let availableResources = [];
  let resourceTemplates = [];
  let availablePrompts = [];
  let loadingResources = false;
  let loadingPrompts = false;
  let selectedResource = null;
  let resourceContent = null;
  let selectedPrompt = null;
  let promptResult = null;
  let promptArgs = {};

  // Playground
  let showPlayground = false;
  let playgroundTask = '';
  let playgroundResult = '';
  let playgroundError = '';
  let testing = false;

  const availableModels = [
    'gpt-4o-mini',
    'gpt-4o',
    'gpt-4-turbo',
    'gpt-3.5-turbo',
    'claude-3-5-sonnet-20241022',
    'claude-3-haiku-20240307'
  ];

  onMount(async () => {
    if (mcpAgentId) {
      await loadMCPAgent();
    } else {
      loading = false;
    }
  });

  async function loadMCPAgent() {
    loading = true;
    error = '';
    
    try {
      const mcpAgent = await GetMCPAgent(mcpAgentId);
      const agentConfig = await GetAgentConfigByID(mcpAgent.agent_config_id);
      
      name = agentConfig.name;
      displayName = agentConfig.display_name;
      description = agentConfig.description;
      model = agentConfig.model || 'gpt-4o-mini';
      systemPrompt = agentConfig.system_prompt || '';
      enabled = agentConfig.enabled;
      
      transportType = mcpAgent.transport_type || 'stdio';
      serverCommand = mcpAgent.server_command || '';
      serverArgs = mcpAgent.server_args || '[]';
      serverEnv = mcpAgent.server_env || '[]';
      workingDir = mcpAgent.working_dir || '';
      serverURL = mcpAgent.server_url || '';
      authType = mcpAgent.auth_type || 'none';
      authValue = mcpAgent.auth_value || '';
      httpHeaders = mcpAgent.http_headers || '{}';
      executionMode = mcpAgent.execution_mode || 'convert';
      autoConnect = mcpAgent.auto_connect;
      
      // Check connection status
      await refreshConnectionStatus();
      
    } catch (err) {
      error = 'Erro ao carregar MCP Agent: ' + (err.message || err);
    } finally {
      loading = false;
    }
  }

  async function refreshConnectionStatus() {
    if (!mcpAgentId) return;
    
    try {
      const status = await GetMCPAgentStatus(mcpAgentId);
      connected = status.connected;
      serverInfo = status.server_info || null;
      availableTools = status.tools || [];
    } catch (err) {
      connected = false;
      serverInfo = null;
      availableTools = [];
    }
  }

  async function handleConnect() {
    connecting = true;
    error = '';
    
    try {
      await ConnectMCPAgent(mcpAgentId);
      await refreshConnectionStatus();
      
      // Carrega resources e prompts após conectar
      await Promise.all([loadResources(), loadPrompts()]);
      
      successMessage = 'Conectado ao servidor MCP!';
      setTimeout(() => successMessage = '', 3000);
    } catch (err) {
      error = 'Erro ao conectar: ' + (err.message || err);
    } finally {
      connecting = false;
    }
  }

  async function handleDisconnect() {
    connecting = true;
    error = '';
    
    try {
      await DisconnectMCPAgent(mcpAgentId);
      connected = false;
      serverInfo = null;
      availableTools = [];
      availableResources = [];
      resourceTemplates = [];
      availablePrompts = [];
      successMessage = 'Desconectado do servidor MCP';
      setTimeout(() => successMessage = '', 3000);
    } catch (err) {
      error = 'Erro ao desconectar: ' + (err.message || err);
    } finally {
      connecting = false;
    }
  }

  // ==================== MCP Resources ====================
  
  async function loadResources() {
    if (!mcpAgentId || !connected) return;
    
    loadingResources = true;
    try {
      availableResources = await GetMCPResources(mcpAgentId) || [];
      resourceTemplates = await GetMCPResourceTemplates(mcpAgentId) || [];
    } catch (err) {
      console.log('Resources não suportados:', err);
      availableResources = [];
      resourceTemplates = [];
    } finally {
      loadingResources = false;
    }
  }
  
  async function readResource(uri) {
    if (!mcpAgentId) return;
    
    selectedResource = uri;
    resourceContent = null;
    
    try {
      resourceContent = await ReadMCPResource(mcpAgentId, uri);
    } catch (err) {
      error = 'Erro ao ler resource: ' + (err.message || err);
    }
  }
  
  // ==================== MCP Prompts ====================
  
  async function loadPrompts() {
    if (!mcpAgentId || !connected) return;
    
    loadingPrompts = true;
    try {
      availablePrompts = await GetMCPPrompts(mcpAgentId) || [];
    } catch (err) {
      console.log('Prompts não suportados:', err);
      availablePrompts = [];
    } finally {
      loadingPrompts = false;
    }
  }
  
  async function getPrompt(promptName) {
    if (!mcpAgentId) return;
    
    selectedPrompt = promptName;
    promptResult = null;
    
    // Encontra os argumentos do prompt
    const prompt = availablePrompts.find(p => p.name === promptName);
    if (prompt && prompt.arguments) {
      promptArgs = {};
      for (const arg of prompt.arguments) {
        promptArgs[arg.name] = '';
      }
    }
  }
  
  async function executePrompt() {
    if (!selectedPrompt || !mcpAgentId) return;
    
    try {
      promptResult = await GetMCPPrompt(mcpAgentId, selectedPrompt, promptArgs);
    } catch (err) {
      error = 'Erro ao obter prompt: ' + (err.message || err);
    }
  }

  async function handleSubmit() {
    if (!name.trim()) {
      error = 'Nome é obrigatório';
      return;
    }
    
    // Validação baseada no tipo de transporte
    if (transportType === 'stdio' && !serverCommand.trim()) {
      error = 'Comando do servidor é obrigatório para transporte local';
      return;
    }
    
    if (transportType === 'http' && !serverURL.trim()) {
      error = 'URL do servidor é obrigatória para transporte remoto';
      return;
    }

    saving = true;
    error = '';
    
    try {
      // Validações de JSON baseadas no tipo de transporte
      if (transportType === 'stdio') {
        try {
          JSON.parse(serverArgs);
        } catch {
          throw new Error('Argumentos do servidor deve ser um JSON array válido');
        }
        
        try {
          JSON.parse(serverEnv);
        } catch {
          throw new Error('Variáveis de ambiente deve ser um JSON array válido');
        }
      }
      
      if (transportType === 'http' && httpHeaders.trim()) {
        try {
          JSON.parse(httpHeaders);
        } catch {
          throw new Error('Headers HTTP deve ser um JSON objeto válido');
        }
      }

      if (mcpAgentId) {
        await UpdateMCPAgentFull(
          mcpAgentId,
          displayName || name,
          description,
          model,
          systemPrompt,
          transportType,
          serverCommand,
          serverArgs,
          serverEnv,
          workingDir,
          serverURL,
          authType,
          authValue,
          httpHeaders,
          executionMode,
          autoConnect,
          enabled
        );
        successMessage = 'MCP Agent atualizado!';
      } else {
        const result = await CreateMCPAgentFull(
          name,
          displayName || name,
          description,
          model,
          systemPrompt,
          transportType,
          serverCommand,
          serverArgs,
          serverEnv,
          workingDir,
          serverURL,
          authType,
          authValue,
          httpHeaders,
          executionMode,
          autoConnect,
          enabled
        );
        console.log('MCP Agent criado:', result);
        successMessage = 'MCP Agent criado!';
      }
      
      setTimeout(() => {
        dispatch('saved');
        onClose();
      }, 500);
      
    } catch (err) {
      error = err.message || err;
    } finally {
      saving = false;
    }
  }

  async function handleTestPlayground() {
    if (!playgroundTask.trim()) {
      playgroundError = 'Digite uma tarefa para testar';
      return;
    }

    testing = true;
    playgroundResult = '';
    playgroundError = '';

    try {
      playgroundResult = await TestMCPAgent(mcpAgentId, playgroundTask);
    } catch (err) {
      playgroundError = err.message || err;
    } finally {
      testing = false;
    }
  }
</script>

<div class="mcp-editor">
  {#if loading}
    <div class="loading">
      <span class="spinner" aria-hidden="true"></span>
      Carregando...
    </div>
  {:else}
    {#if error}
      <div class="message error" role="alert">{error}</div>
    {/if}
    
    {#if successMessage}
      <div class="message success" role="status">{successMessage}</div>
    {/if}

    <form on:submit|preventDefault={handleSubmit}>
      <!-- Informações Básicas -->
      <fieldset>
        <legend>Informações Básicas</legend>
        
        <div class="form-row">
          <div class="form-group">
            <label for="mcp-name">Nome (identificador)</label>
            <input
              id="mcp-name"
              type="text"
              bind:value={name}
              placeholder="meu_servidor_mcp"
              disabled={!!mcpAgentId}
              required
            />
            {#if mcpAgentId}
              <p class="hint">Nome não pode ser alterado após criação</p>
            {/if}
          </div>
          
          <div class="form-group">
            <label for="mcp-display-name">Nome de Exibição</label>
            <input
              id="mcp-display-name"
              type="text"
              bind:value={displayName}
              placeholder="Meu Servidor MCP"
            />
          </div>
        </div>
        
        <div class="form-group">
          <label for="mcp-description">Descrição</label>
          <textarea
            id="mcp-description"
            bind:value={description}
            rows="2"
            placeholder="O que este servidor MCP faz..."
          ></textarea>
        </div>
        
        <div class="form-row">
          <div class="form-group">
            <label for="mcp-model">Modelo LLM</label>
            <select id="mcp-model" bind:value={model}>
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
      </fieldset>

      <!-- Tipo de Transporte -->
      <fieldset>
        <legend>Tipo de Conexão</legend>
        
        <div class="form-group">
          <label for="mcp-transport">Transporte</label>
          <select id="mcp-transport" bind:value={transportType}>
            {#each transportTypes as t}
              <option value={t.value}>{t.label}</option>
            {/each}
          </select>
          <p class="hint">
            {transportTypes.find(t => t.value === transportType)?.description}
          </p>
        </div>
      </fieldset>

      <!-- Configuração stdio (Local) -->
      {#if transportType === 'stdio'}
        <fieldset>
          <legend>Servidor Local (stdio)</legend>
          
          <div class="form-group">
            <label for="mcp-command">Comando do Servidor</label>
            <input
              id="mcp-command"
              type="text"
              bind:value={serverCommand}
              placeholder="npx -y @modelcontextprotocol/server-exemplo"
            />
            <p class="hint">
              Comando para iniciar o servidor MCP. Ex: npx, python, node, etc.
            </p>
          </div>
          
          <div class="form-group">
            <label for="mcp-args">Argumentos (JSON array)</label>
            <input
              id="mcp-args"
              type="text"
              bind:value={serverArgs}
              placeholder='["--port", "3000"]'
            />
            <p class="hint">
              Argumentos adicionais para o comando. Ex: ["--verbose"]
            </p>
          </div>
          
          <div class="form-group">
            <label for="mcp-env">Variáveis de Ambiente (JSON array)</label>
            <input
              id="mcp-env"
              type="text"
              bind:value={serverEnv}
              placeholder='["API_KEY=xxx", "DEBUG=true"]'
            />
            <p class="hint">
              Variáveis de ambiente no formato ["VAR=valor", ...]
            </p>
          </div>
          
          <div class="form-group">
            <label for="mcp-workdir">Diretório de Trabalho (opcional)</label>
            <input
              id="mcp-workdir"
              type="text"
              bind:value={workingDir}
              placeholder="C:\caminho\para\projeto"
            />
          </div>
        </fieldset>
      {/if}

      <!-- Configuração HTTP (Remoto) -->
      {#if transportType === 'http'}
        <fieldset>
          <legend>Servidor Remoto (HTTP/SSE)</legend>
          
          <div class="form-group">
            <label for="mcp-url">URL do Servidor</label>
            <input
              id="mcp-url"
              type="url"
              bind:value={serverURL}
              placeholder="https://mcp.example.com"
            />
            <p class="hint">
              URL base do servidor MCP remoto
            </p>
          </div>
          
          <div class="form-row">
            <div class="form-group">
              <label for="mcp-auth-type">Autenticação</label>
              <select id="mcp-auth-type" bind:value={authType}>
                {#each authTypes as a}
                  <option value={a.value}>{a.label}</option>
                {/each}
              </select>
            </div>
            
            {#if authType !== 'none'}
              <div class="form-group">
                <label for="mcp-auth-value">
                  {authType === 'bearer' ? 'Token' : 'API Key'}
                </label>
                <input
                  id="mcp-auth-value"
                  type="password"
                  bind:value={authValue}
                  placeholder={authType === 'bearer' ? 'Bearer token' : 'API Key'}
                />
              </div>
            {/if}
          </div>
          
          <div class="form-group">
            <label for="mcp-headers">Headers Customizados (JSON)</label>
            <input
              id="mcp-headers"
              type="text"
              bind:value={httpHeaders}
              placeholder={'{"X-Custom-Header": "value"}'}
            />
            <p class="hint">
              Headers adicionais para as requisições HTTP
            </p>
          </div>
        </fieldset>
      {/if}

      <!-- Configurações Avançadas -->
      <fieldset>
        <legend>Configurações Avançadas</legend>
        
        <div class="form-group">
          <label for="mcp-execution-mode">Modo de Execução</label>
          <select id="mcp-execution-mode" bind:value={executionMode}>
            {#each executionModes as mode}
              <option value={mode.value}>{mode.label}</option>
            {/each}
          </select>
          <p class="hint">
            {executionModes.find(m => m.value === executionMode)?.description}
          </p>
        </div>
        
        <div class="form-group checkbox-group">
          <label>
            <input type="checkbox" bind:checked={autoConnect} />
            Conectar automaticamente ao iniciar
          </label>
        </div>
      </fieldset>

      <!-- System Prompt -->
      <fieldset>
        <legend>System Prompt (opcional)</legend>
        
        <div class="form-group">
          <textarea
            id="mcp-system-prompt"
            bind:value={systemPrompt}
            rows="4"
            placeholder="Instruções adicionais para o agente..."
          ></textarea>
        </div>
      </fieldset>

      <!-- Status de Conexão (apenas para edição) -->
      {#if mcpAgentId}
        <fieldset>
          <legend>Status de Conexão</legend>
          
          <div class="connection-status">
            <div class="status-indicator" class:connected>
              {#if connected}
                <span class="status-dot connected" aria-hidden="true"></span>
                Conectado
              {:else}
                <span class="status-dot" aria-hidden="true"></span>
                Desconectado
              {/if}
            </div>
            
            <div class="status-actions">
              {#if connected}
                <button
                  type="button"
                  class="btn-secondary"
                  on:click={handleDisconnect}
                  disabled={connecting}
                >
                  {connecting ? 'Desconectando...' : 'Desconectar'}
                </button>
              {:else}
                <button
                  type="button"
                  class="btn-primary"
                  on:click={handleConnect}
                  disabled={connecting}
                >
                  {connecting ? 'Conectando...' : 'Conectar'}
                </button>
              {/if}
            </div>
          </div>
          
          {#if connected && serverInfo}
            <div class="server-info">
              <h4>Informações do Servidor</h4>
              <dl>
                <dt>Nome:</dt>
                <dd>{serverInfo.name}</dd>
                <dt>Versão:</dt>
                <dd>{serverInfo.version}</dd>
                <dt>Protocolo:</dt>
                <dd>{serverInfo.protocol_version}</dd>
              </dl>
            </div>
          {/if}
          
          {#if connected && availableTools.length > 0}
            <div class="tools-list">
              <h4>🔧 Ferramentas Disponíveis ({availableTools.length})</h4>
              <ul>
                {#each availableTools as tool}
                  <li>
                    <strong>{tool.name}</strong>
                    {#if tool.description}
                      <span class="tool-desc">- {tool.description}</span>
                    {/if}
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
          
          <!-- MCP Resources -->
          {#if connected && (availableResources.length > 0 || resourceTemplates.length > 0)}
            <div class="resources-section">
              <h4>📁 Resources ({availableResources.length})</h4>
              
              {#if loadingResources}
                <p class="loading-text">Carregando resources...</p>
              {:else}
                {#if availableResources.length > 0}
                  <div class="resources-list">
                    {#each availableResources as resource}
                      <div 
                        class="resource-item" 
                        class:selected={selectedResource === resource.uri}
                        on:click={() => readResource(resource.uri)}
                        on:keypress={(e) => e.key === 'Enter' && readResource(resource.uri)}
                        role="button"
                        tabindex="0"
                      >
                        <span class="resource-name">{resource.name}</span>
                        <span class="resource-uri">{resource.uri}</span>
                        {#if resource.mime_type}
                          <span class="resource-mime">{resource.mime_type}</span>
                        {/if}
                      </div>
                    {/each}
                  </div>
                {/if}
                
                {#if resourceTemplates.length > 0}
                  <h5>Templates de Resource ({resourceTemplates.length})</h5>
                  <ul class="template-list">
                    {#each resourceTemplates as template}
                      <li>
                        <code>{template.uri_template}</code>
                        {#if template.description}
                          <span>- {template.description}</span>
                        {/if}
                      </li>
                    {/each}
                  </ul>
                {/if}
                
                {#if resourceContent}
                  <div class="resource-content">
                    <h5>Conteúdo: {selectedResource}</h5>
                    {#if resourceContent.is_blob}
                      <p class="blob-notice">📦 Conteúdo binário (blob)</p>
                    {:else}
                      <pre>{resourceContent.text}</pre>
                    {/if}
                  </div>
                {/if}
              {/if}
            </div>
          {/if}
          
          <!-- MCP Prompts -->
          {#if connected && availablePrompts.length > 0}
            <div class="prompts-section">
              <h4>💬 Prompts ({availablePrompts.length})</h4>
              
              {#if loadingPrompts}
                <p class="loading-text">Carregando prompts...</p>
              {:else}
                <div class="prompts-list">
                  {#each availablePrompts as prompt}
                    <div 
                      class="prompt-item"
                      class:selected={selectedPrompt === prompt.name}
                      on:click={() => getPrompt(prompt.name)}
                      on:keypress={(e) => e.key === 'Enter' && getPrompt(prompt.name)}
                      role="button"
                      tabindex="0"
                    >
                      <span class="prompt-name">{prompt.name}</span>
                      {#if prompt.description}
                        <span class="prompt-desc">{prompt.description}</span>
                      {/if}
                      {#if prompt.arguments && prompt.arguments.length > 0}
                        <span class="prompt-args">({prompt.arguments.length} args)</span>
                      {/if}
                    </div>
                  {/each}
                </div>
                
                {#if selectedPrompt}
                  <div class="prompt-editor">
                    <h5>Prompt: {selectedPrompt}</h5>
                    
                    {#each Object.keys(promptArgs) as argName}
                      <div class="form-group">
                        <label for="arg-{argName}">{argName}</label>
                        <input
                          id="arg-{argName}"
                          type="text"
                          bind:value={promptArgs[argName]}
                          placeholder="Valor para {argName}"
                        />
                      </div>
                    {/each}
                    
                    <button
                      type="button"
                      class="btn-primary btn-small"
                      on:click={executePrompt}
                    >
                      ▶️ Executar Prompt
                    </button>
                    
                    {#if promptResult}
                      <div class="prompt-result">
                        <h6>Mensagens:</h6>
                        {#each promptResult.messages as msg}
                          <div class="prompt-message" class:user={msg.role === 'user'} class:assistant={msg.role === 'assistant'}>
                            <strong>{msg.role}:</strong>
                            <span>{msg.content}</span>
                          </div>
                        {/each}
                      </div>
                    {/if}
                  </div>
                {/if}
              {/if}
            </div>
          {/if}
        </fieldset>

        <!-- Playground -->
        <fieldset>
          <legend>
            <button
              type="button"
              class="legend-toggle"
              on:click={() => showPlayground = !showPlayground}
            >
              🧪 Playground {showPlayground ? '▲' : '▼'}
            </button>
          </legend>
          
          {#if showPlayground}
            <div class="playground">
              {#if !connected}
                <p class="warning">⚠️ Conecte ao servidor MCP primeiro para testar.</p>
              {:else}
                <div class="form-group">
                  <label for="playground-task">Tarefa (linguagem natural)</label>
                  <textarea
                    id="playground-task"
                    bind:value={playgroundTask}
                    rows="3"
                    placeholder="Descreva o que você quer que o agente faça..."
                  ></textarea>
                </div>
                
                <button
                  type="button"
                  class="btn-primary"
                  on:click={handleTestPlayground}
                  disabled={testing}
                >
                  {testing ? 'Executando...' : '▶️ Executar'}
                </button>
                
                {#if playgroundError}
                  <div class="message error">{playgroundError}</div>
                {/if}
                
                {#if playgroundResult}
                  <div class="result">
                    <strong>Resultado:</strong>
                    <pre>{playgroundResult}</pre>
                  </div>
                {/if}
              {/if}
            </div>
          {/if}
        </fieldset>
      {/if}

      <!-- Actions -->
      <div class="form-actions">
        <button type="button" class="btn-secondary" on:click={onClose} disabled={saving}>
          Cancelar
        </button>
        <button type="submit" class="btn-primary" disabled={saving}>
          {saving ? 'Salvando...' : mcpAgentId ? 'Salvar' : 'Criar MCP Agent'}
        </button>
      </div>
    </form>
  {/if}
</div>

<style>
  .mcp-editor {
    padding: var(--spacing-md);
    max-width: 700px;
  }

  .loading {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--spacing-xl);
    color: var(--color-text-muted);
  }

  .spinner {
    width: 20px;
    height: 20px;
    border: 2px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: spin 0.75s linear infinite;
    margin-right: var(--spacing-sm);
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .message {
    padding: var(--spacing-sm);
    border-radius: var(--border-radius);
    margin-bottom: var(--spacing-md);
  }

  .message.error {
    background: rgba(248, 81, 73, 0.1);
    border: 1px solid var(--color-error);
    color: var(--color-error);
  }

  .message.success {
    background: rgba(63, 185, 80, 0.1);
    border: 1px solid var(--color-success, #3fb950);
    color: var(--color-success, #3fb950);
  }

  fieldset {
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    padding: var(--spacing-md);
    margin-bottom: var(--spacing-md);
  }

  legend {
    font-weight: 600;
    padding: 0 var(--spacing-sm);
    color: var(--color-text-primary);
  }

  .legend-toggle {
    background: none;
    border: none;
    color: inherit;
    font: inherit;
    cursor: pointer;
    padding: 0;
  }

  .legend-toggle:hover {
    color: var(--color-accent);
  }

  .form-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--spacing-md);
  }

  .form-group {
    margin-bottom: var(--spacing-md);
  }

  .form-group:last-child {
    margin-bottom: 0;
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
    background: var(--color-bg-input, var(--color-bg-secondary));
    color: var(--color-text-primary);
    font-family: inherit;
  }

  .form-group input:focus,
  .form-group textarea:focus,
  .form-group select:focus {
    outline: none;
    border-color: var(--color-accent);
  }

  .form-group input:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .hint {
    font-size: var(--font-size-sm);
    color: var(--color-text-muted);
    margin-top: var(--spacing-xs);
  }

  .checkbox-group label {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    cursor: pointer;
    font-weight: normal;
  }

  .checkbox-group input {
    width: auto;
  }

  .connection-status {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--spacing-md);
  }

  .status-indicator {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
  }

  .status-dot {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--color-text-muted);
  }

  .status-dot.connected {
    background: var(--color-success, #3fb950);
  }

  .server-info {
    background: var(--color-bg-tertiary);
    padding: var(--spacing-sm);
    border-radius: var(--border-radius);
    margin-bottom: var(--spacing-md);
  }

  .server-info h4 {
    margin: 0 0 var(--spacing-sm);
    font-size: var(--font-size-sm);
    font-weight: 600;
  }

  .server-info dl {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: var(--spacing-xs) var(--spacing-sm);
    margin: 0;
    font-size: var(--font-size-sm);
  }

  .server-info dt {
    color: var(--color-text-muted);
  }

  .tools-list {
    background: var(--color-bg-tertiary);
    padding: var(--spacing-sm);
    border-radius: var(--border-radius);
  }

  .tools-list h4 {
    margin: 0 0 var(--spacing-sm);
    font-size: var(--font-size-sm);
    font-weight: 600;
  }

  .tools-list ul {
    margin: 0;
    padding-left: var(--spacing-md);
    font-size: var(--font-size-sm);
  }

  .tools-list li {
    margin-bottom: var(--spacing-xs);
  }

  .tool-desc {
    color: var(--color-text-muted);
  }

  .playground {
    padding-top: var(--spacing-sm);
  }

  .warning {
    color: var(--color-warning, #d29922);
    font-size: var(--font-size-sm);
  }

  .result {
    background: var(--color-bg-tertiary);
    padding: var(--spacing-sm);
    border-radius: var(--border-radius);
    margin-top: var(--spacing-md);
  }

  .result pre {
    margin: var(--spacing-sm) 0 0;
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 200px;
    overflow-y: auto;
    font-size: var(--font-size-sm);
  }

  .form-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--spacing-sm);
    padding-top: var(--spacing-md);
    border-top: 1px solid var(--color-border);
  }

  .btn-primary {
    background-color: var(--color-accent);
    color: white;
    border: none;
    border-radius: var(--border-radius);
    padding: var(--spacing-sm) var(--spacing-md);
    cursor: pointer;
    font-weight: 500;
  }

  .btn-secondary {
    background-color: var(--color-bg-tertiary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    padding: var(--spacing-sm) var(--spacing-md);
    cursor: pointer;
  }

  .btn-primary:hover,
  .btn-secondary:hover {
    opacity: 0.9;
  }

  .btn-primary:disabled,
  .btn-secondary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .btn-small {
    padding: var(--spacing-xs) var(--spacing-sm);
    font-size: var(--font-size-sm);
  }

  /* Resources Section */
  .resources-section,
  .prompts-section {
    margin-top: var(--spacing-md);
    padding-top: var(--spacing-md);
    border-top: 1px solid var(--color-border);
  }

  .resources-section h4,
  .prompts-section h4 {
    margin: 0 0 var(--spacing-sm);
    font-size: var(--font-size-md);
  }

  .resources-section h5,
  .prompts-section h5 {
    margin: var(--spacing-md) 0 var(--spacing-xs);
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
  }

  .loading-text {
    color: var(--color-text-muted);
    font-style: italic;
  }

  .resources-list,
  .prompts-list {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-xs);
  }

  .resource-item,
  .prompt-item {
    display: flex;
    flex-wrap: wrap;
    gap: var(--spacing-xs);
    padding: var(--spacing-sm);
    background: var(--color-bg-tertiary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    cursor: pointer;
    transition: all 0.15s;
  }

  .resource-item:hover,
  .prompt-item:hover {
    border-color: var(--color-accent);
  }

  .resource-item.selected,
  .prompt-item.selected {
    background: rgba(var(--color-accent-rgb, 88, 166, 255), 0.1);
    border-color: var(--color-accent);
  }

  .resource-name,
  .prompt-name {
    font-weight: 600;
  }

  .resource-uri {
    font-family: monospace;
    font-size: var(--font-size-xs);
    color: var(--color-text-muted);
  }

  .resource-mime,
  .prompt-args {
    font-size: var(--font-size-xs);
    padding: 2px 6px;
    background: var(--color-bg-secondary);
    border-radius: 4px;
    color: var(--color-text-secondary);
  }

  .prompt-desc {
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
  }

  .template-list {
    list-style: none;
    padding: 0;
    margin: 0;
  }

  .template-list li {
    padding: var(--spacing-xs);
    font-size: var(--font-size-sm);
  }

  .template-list code {
    background: var(--color-bg-tertiary);
    padding: 2px 6px;
    border-radius: 4px;
    font-family: monospace;
  }

  .resource-content {
    margin-top: var(--spacing-md);
    padding: var(--spacing-md);
    background: var(--color-bg-tertiary);
    border-radius: var(--border-radius);
  }

  .resource-content pre {
    margin: var(--spacing-sm) 0 0;
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 300px;
    overflow-y: auto;
    font-size: var(--font-size-sm);
    background: var(--color-bg-secondary);
    padding: var(--spacing-sm);
    border-radius: var(--border-radius);
  }

  .blob-notice {
    color: var(--color-text-muted);
    font-style: italic;
  }

  .prompt-editor {
    margin-top: var(--spacing-md);
    padding: var(--spacing-md);
    background: var(--color-bg-tertiary);
    border-radius: var(--border-radius);
  }

  .prompt-result {
    margin-top: var(--spacing-md);
  }

  .prompt-result h6 {
    margin: 0 0 var(--spacing-xs);
    font-size: var(--font-size-sm);
  }

  .prompt-message {
    padding: var(--spacing-sm);
    margin-bottom: var(--spacing-xs);
    border-radius: var(--border-radius);
    background: var(--color-bg-secondary);
  }

  .prompt-message.user {
    background: rgba(88, 166, 255, 0.1);
  }

  .prompt-message.assistant {
    background: rgba(63, 185, 80, 0.1);
  }

  .prompt-message strong {
    display: block;
    font-size: var(--font-size-xs);
    text-transform: uppercase;
    color: var(--color-text-muted);
    margin-bottom: var(--spacing-xs);
  }
</style>

