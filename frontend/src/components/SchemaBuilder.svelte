<script>
  import { createEventDispatcher } from 'svelte';
  
  export let schema = { type: 'object', properties: {}, required: [] };
  
  const dispatch = createEventDispatcher();
  
  // Tipos disponíveis
  const types = ['string', 'integer', 'number', 'boolean', 'array', 'object'];
  
  // Estado local
  let properties = [];
  let editingIndex = -1;
  
  // Converte schema para array de properties
  $: {
    if (schema && schema.properties) {
      const requiredList = schema.required || [];
      properties = Object.entries(schema.properties).map(([name, prop]) => ({
        name,
        type: prop.type || 'string',
        description: prop.description || '',
        required: requiredList.includes(name),
        enum: prop.enum || [],
        items: prop.items || null,
      }));
    } else {
      properties = [];
    }
  }
  
  function addProperty() {
    properties = [...properties, {
      name: '',
      type: 'string',
      description: '',
      required: false,
      enum: [],
      items: null,
    }];
    editingIndex = properties.length - 1;
    emitChange();
  }
  
  function removeProperty(index) {
    properties = properties.filter((_, i) => i !== index);
    emitChange();
  }
  
  function updateProperty(index, field, value) {
    properties[index][field] = value;
    properties = [...properties];
    emitChange();
  }
  
  function toggleRequired(index) {
    properties[index].required = !properties[index].required;
    properties = [...properties];
    emitChange();
  }
  
  function emitChange() {
    const newSchema = buildSchema();
    dispatch('change', { schema: newSchema });
  }
  
  function buildSchema() {
    const props = {};
    const required = [];
    
    for (const prop of properties) {
      if (!prop.name.trim()) continue;
      
      const propDef = {
        type: prop.type,
      };
      
      if (prop.description) {
        propDef.description = prop.description;
      }
      
      if (prop.enum && prop.enum.length > 0) {
        propDef.enum = prop.enum;
      }
      
      if (prop.type === 'array' && prop.items) {
        propDef.items = prop.items;
      }
      
      props[prop.name] = propDef;
      
      if (prop.required) {
        required.push(prop.name);
      }
    }
    
    return {
      type: 'object',
      properties: props,
      required: required.length > 0 ? required : undefined,
    };
  }
  
  function handleNameKeyDown(event, index) {
    if (event.key === 'Enter') {
      event.preventDefault();
      editingIndex = -1;
    }
  }
  
  export function getSchema() {
    return buildSchema();
  }
  
  export function setSchema(newSchema) {
    schema = newSchema;
  }
</script>

<div class="schema-builder">
  <div class="schema-header">
    <span class="schema-title">Parâmetros</span>
    <button class="btn-add" on:click={addProperty} type="button">
      + Adicionar
    </button>
  </div>
  
  {#if properties.length === 0}
    <div class="empty-state">
      Nenhum parâmetro definido. Clique em "+ Adicionar" para criar.
    </div>
  {:else}
    <table class="schema-table">
      <thead>
        <tr>
          <th>Nome</th>
          <th>Tipo</th>
          <th>Obrigatório</th>
          <th>Descrição</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {#each properties as prop, index (index)}
          <tr class:editing={editingIndex === index}>
            <td>
              <input 
                type="text"
                bind:value={prop.name}
                on:blur={() => { editingIndex = -1; emitChange(); }}
                on:keydown={(e) => handleNameKeyDown(e, index)}
                placeholder="nome_param"
                class="input-name"
              />
            </td>
            <td>
              <select 
                bind:value={prop.type}
                on:change={() => emitChange()}
                class="select-type"
              >
                {#each types as type}
                  <option value={type}>{type}</option>
                {/each}
              </select>
            </td>
            <td class="td-center">
              <input 
                type="checkbox"
                checked={prop.required}
                on:change={() => toggleRequired(index)}
                class="checkbox-required"
              />
            </td>
            <td>
              <input 
                type="text"
                bind:value={prop.description}
                on:blur={() => emitChange()}
                placeholder="Descrição do parâmetro"
                class="input-description"
              />
            </td>
            <td class="td-actions">
              <button 
                class="btn-icon btn-delete"
                on:click={() => removeProperty(index)}
                type="button"
                aria-label="Remover parâmetro"
              >
                ✕
              </button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
  
  {#if properties.length > 0}
    <details class="schema-json">
      <summary>Ver JSON Schema</summary>
      <pre>{JSON.stringify(buildSchema(), null, 2)}</pre>
    </details>
  {/if}
</div>

<style>
  .schema-builder {
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    padding: var(--spacing-md);
    background: var(--color-bg-tertiary);
  }
  
  .schema-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--spacing-md);
  }
  
  .schema-title {
    font-weight: 600;
    color: var(--color-text-primary);
  }
  
  .btn-add {
    padding: var(--spacing-xs) var(--spacing-sm);
    background: var(--color-accent);
    color: white;
    border: none;
    border-radius: var(--border-radius);
    cursor: pointer;
    font-size: var(--font-size-sm);
  }
  
  .btn-add:hover {
    opacity: 0.9;
  }
  
  .empty-state {
    text-align: center;
    padding: var(--spacing-lg);
    color: var(--color-text-muted);
    font-size: var(--font-size-sm);
  }
  
  .schema-table {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--font-size-sm);
  }
  
  .schema-table th {
    text-align: left;
    padding: var(--spacing-xs) var(--spacing-sm);
    border-bottom: 1px solid var(--color-border);
    color: var(--color-text-muted);
    font-weight: 500;
    font-size: var(--font-size-xs);
    text-transform: uppercase;
  }
  
  .schema-table td {
    padding: var(--spacing-xs) var(--spacing-sm);
    border-bottom: 1px solid var(--color-border);
    vertical-align: middle;
  }
  
  .schema-table tr:last-child td {
    border-bottom: none;
  }
  
  .schema-table tr.editing {
    background: rgba(88, 166, 255, 0.1);
  }
  
  .input-name {
    width: 120px;
    padding: var(--spacing-xs);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-family: 'Fira Code', monospace;
    font-size: var(--font-size-sm);
  }
  
  .select-type {
    padding: var(--spacing-xs);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-size: var(--font-size-sm);
    min-width: 80px;
  }
  
  .td-center {
    text-align: center;
  }
  
  .checkbox-required {
    width: 18px;
    height: 18px;
    cursor: pointer;
  }
  
  .input-description {
    width: 100%;
    padding: var(--spacing-xs);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-size: var(--font-size-sm);
  }
  
  .td-actions {
    width: 40px;
    text-align: center;
  }
  
  .btn-delete {
    background: none;
    border: none;
    color: var(--color-text-muted);
    cursor: pointer;
    padding: var(--spacing-xs);
    font-size: var(--font-size-sm);
  }
  
  .btn-delete:hover {
    color: var(--color-error);
  }
  
  .schema-json {
    margin-top: var(--spacing-md);
    font-size: var(--font-size-sm);
  }
  
  .schema-json summary {
    cursor: pointer;
    color: var(--color-text-muted);
    padding: var(--spacing-xs) 0;
  }
  
  .schema-json summary:hover {
    color: var(--color-text-primary);
  }
  
  .schema-json pre {
    margin-top: var(--spacing-sm);
    padding: var(--spacing-sm);
    background: var(--color-bg-secondary);
    border-radius: var(--border-radius);
    overflow-x: auto;
    font-size: var(--font-size-xs);
    color: var(--color-text-secondary);
  }
  
  input:focus, select:focus {
    outline: none;
    border-color: var(--color-accent);
  }
</style>



