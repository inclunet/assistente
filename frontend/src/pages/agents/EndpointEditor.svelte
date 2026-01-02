<script>
  import { createEventDispatcher } from 'svelte';
  import { TemplateEditor as MonacoTemplate } from '../../components/editor';
  import SchemaBuilder from './SchemaBuilder.svelte';
  
  export let endpoint = null;
  export let onSave = null;
  export let onCancel = null;
  export let onTest = null;
  
  const dispatch = createEventDispatcher();
  
  // Métodos HTTP
  const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'];
  
  // Estado do formulário
  let name = '';
  let description = '';
  let method = 'GET';
  let pathTemplate = '';
  let queryTemplate = '';
  let headersJSON = '';
  let bodyTemplate = '';
  let parameters = { type: 'object', properties: {}, required: [] };
  let responseTemplate = '';
  let saving = false;
  let error = '';
  
  // Carrega endpoint se fornecido
  $: if (endpoint) {
    name = endpoint.name || '';
    description = endpoint.description || '';
    method = endpoint.method || 'GET';
    pathTemplate = endpoint.path_template || '';
    queryTemplate = endpoint.query_template || '';
    headersJSON = endpoint.headers_json || '';
    bodyTemplate = endpoint.body_template || '';
    responseTemplate = endpoint.response_template || '';
    
    try {
      parameters = endpoint.parameters ? JSON.parse(endpoint.parameters) : { type: 'object', properties: {}, required: [] };
    } catch (e) {
      parameters = { type: 'object', properties: {}, required: [] };
    }
  }
  
  function handleSchemaChange(event) {
    parameters = event.detail.schema;
  }
  
  async function handleSave() {
    if (!name.trim()) {
      error = 'Nome é obrigatório';
      return;
    }
    if (!pathTemplate.trim()) {
      error = 'Path template é obrigatório';
      return;
    }
    
    saving = true;
    error = '';
    
    const endpointData = {
      id: endpoint?.id,
      name: name.trim(),
      description: description.trim(),
      method,
      path_template: pathTemplate,
      query_template: queryTemplate,
      headers_json: headersJSON,
      body_template: bodyTemplate,
      parameters: JSON.stringify(parameters),
      response_template: responseTemplate,
    };
    
    try {
      if (onSave) {
        await onSave(endpointData);
      }
      dispatch('save', endpointData);
    } catch (err) {
      error = 'Erro ao salvar: ' + (err.message || err);
    } finally {
      saving = false;
    }
  }
  
  function handleCancel() {
    if (onCancel) {
      onCancel();
    }
    dispatch('cancel');
  }
  
  async function handleTest() {
    if (onTest) {
      onTest({
        name,
        method,
        path_template: pathTemplate,
        query_template: queryTemplate,
        body_template: bodyTemplate,
        parameters,
      });
    }
    dispatch('test', { name, parameters });
  }
  
  function getMethodColor(m) {
    switch (m) {
      case 'GET': return '#61affe';
      case 'POST': return '#49cc90';
      case 'PUT': return '#fca130';
      case 'PATCH': return '#50e3c2';
      case 'DELETE': return '#f93e3e';
      default: return '#999';
    }
  }
</script>

<div class="endpoint-editor">
  <div class="editor-header">
    <h3>{endpoint?.id ? 'Editar Endpoint' : 'Novo Endpoint'}</h3>
  </div>
  
  {#if error}
    <div class="error-message" role="alert">{error}</div>
  {/if}
  
  <div class="form-section">
    <h4>Identificação</h4>
    
    <div class="form-row">
      <div class="form-group flex-1">
        <label for="endpoint-name">Nome da Função</label>
        <input 
          type="text"
          id="endpoint-name"
          bind:value={name}
          placeholder="get_customer, create_order, etc."
          class="input-mono"
        />
        <small>Identificador único usado pelo LLM para chamar este endpoint</small>
      </div>
      
      <div class="form-group" style="width: 120px;">
        <label for="endpoint-method">Método</label>
        <select id="endpoint-method" bind:value={method}>
          {#each methods as m}
            <option value={m} style="color: {getMethodColor(m)}">{m}</option>
          {/each}
        </select>
      </div>
    </div>
    
    <div class="form-group">
      <label for="endpoint-description">Descrição</label>
      <textarea 
        id="endpoint-description"
        bind:value={description}
        rows="2"
        placeholder="Descreva o que este endpoint faz. Esta descrição ajuda o LLM a decidir quando usar."
      ></textarea>
    </div>
  </div>
  
  <div class="form-section">
    <h4>Request</h4>
    
    <div class="form-group">
      <label>Path Template</label>
      <MonacoTemplate 
        bind:value={pathTemplate}
        schema={parameters}
        placeholder={"/users/{" + "{.user_id}" + "}/orders"}
        height="40px"
        singleLine={true}
      />
      <small>Caminho da URL. Use {"{" + "{.variavel}" + "}"} para parâmetros dinâmicos.</small>
    </div>
    
    <div class="form-group">
      <label>Query Template <span class="optional">(opcional)</span></label>
      <MonacoTemplate 
        bind:value={queryTemplate}
        schema={parameters}
        placeholder={"page={" + "{.page | default 1}" + "}&limit={" + "{.limit | default 10}" + "}"}
        height="40px"
        singleLine={true}
      />
      <small>Query string. Não inclua o "?".</small>
    </div>
    
    {#if method !== 'GET' && method !== 'DELETE'}
      <div class="form-group">
        <label>Body Template <span class="optional">(opcional)</span></label>
        <MonacoTemplate 
          bind:value={bodyTemplate}
          schema={parameters}
          placeholder={"{\n  \"name\": \"{" + "{.name}" + "}\"\n}"}
          height="150px"
        />
        <small>Corpo da requisição. Geralmente JSON.</small>
      </div>
    {/if}
    
    <div class="form-group">
      <label>Headers Específicos <span class="optional">(opcional)</span></label>
      <MonacoTemplate 
        bind:value={headersJSON}
        schema={parameters}
        placeholder={"{\n  \"X-Custom-Header\": \"valor\"\n}"}
        height="80px"
      />
      <small>Headers adicionais para este endpoint. Serão mesclados com os headers padrão.</small>
    </div>
  </div>
  
  <div class="form-section">
    <h4>Parâmetros (Schema)</h4>
    <p class="section-description">
      Define os parâmetros que o LLM pode passar para este endpoint.
      Estes parâmetros ficam disponíveis nos templates acima.
    </p>
    
    <SchemaBuilder 
      schema={parameters}
      on:change={handleSchemaChange}
    />
  </div>
  
  <div class="form-section">
    <h4>Response</h4>
    
    <div class="form-group">
      <label>Response Template <span class="optional">(opcional)</span></label>
      <MonacoTemplate 
        bind:value={responseTemplate}
        schema={parameters}
        placeholder={"Cliente {" + "{.name}" + "} encontrado com ID {" + "{.id}" + "}"}
        height="80px"
      />
      <small>Template para formatar a resposta da API. As variáveis da resposta JSON ficam disponíveis.</small>
    </div>
  </div>
  
  <div class="form-actions">
    <button type="button" class="btn-secondary" on:click={handleCancel} disabled={saving}>
      Cancelar
    </button>
    <button type="button" class="btn-secondary" on:click={handleTest} disabled={saving}>
      🧪 Testar
    </button>
    <button type="button" class="btn-primary" on:click={handleSave} disabled={saving}>
      {saving ? 'Salvando...' : 'Salvar Endpoint'}
    </button>
  </div>
</div>

<style>
  .endpoint-editor {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-lg);
  }
  
  .editor-header h3 {
    margin: 0;
    font-size: var(--font-size-lg);
  }
  
  .error-message {
    padding: var(--spacing-sm);
    background: rgba(248, 81, 73, 0.1);
    border: 1px solid var(--color-error);
    border-radius: var(--border-radius);
    color: var(--color-error);
  }
  
  .form-section {
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    padding: var(--spacing-md);
  }
  
  .form-section h4 {
    margin: 0 0 var(--spacing-md);
    font-size: var(--font-size-md);
    color: var(--color-text-primary);
  }
  
  .section-description {
    margin: 0 0 var(--spacing-md);
    font-size: var(--font-size-sm);
    color: var(--color-text-muted);
  }
  
  .form-row {
    display: flex;
    gap: var(--spacing-md);
  }
  
  .flex-1 {
    flex: 1;
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
    color: var(--color-text-primary);
  }
  
  .form-group small {
    display: block;
    margin-top: var(--spacing-xs);
    font-size: var(--font-size-xs);
    color: var(--color-text-muted);
  }
  
  .optional {
    font-weight: normal;
    color: var(--color-text-muted);
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
    font-size: var(--font-size-base);
  }
  
  .form-group textarea {
    resize: vertical;
  }
  
  .input-mono {
    font-family: 'Fira Code', monospace;
  }
  
  .form-group input:focus,
  .form-group textarea:focus,
  .form-group select:focus {
    outline: none;
    border-color: var(--color-accent);
  }
  
  .form-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--spacing-sm);
    padding-top: var(--spacing-md);
    border-top: 1px solid var(--color-border);
  }
  
  .btn-primary {
    padding: var(--spacing-sm) var(--spacing-lg);
    background: var(--color-accent);
    color: white;
    border: none;
    border-radius: var(--border-radius);
    cursor: pointer;
    font-weight: 500;
  }
  
  .btn-secondary {
    padding: var(--spacing-sm) var(--spacing-lg);
    background: var(--color-bg-tertiary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    cursor: pointer;
  }
  
  .btn-primary:hover:not(:disabled),
  .btn-secondary:hover:not(:disabled) {
    opacity: 0.9;
  }
  
  .btn-primary:disabled,
  .btn-secondary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>

