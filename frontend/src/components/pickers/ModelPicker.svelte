<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { GetModels } from '../../wailsjs/go/main/App.js';
  import { Combobox as ComboboxPicker } from './combobox';

  const dispatch = createEventDispatcher();

  // Props
  export let value = '';
  export let label = 'Modelo';
  export let icon = '🤖';
  export let placeholder = 'Filtrar modelos...';
  export let disabled = false;
  export let allowCustom = false;
  export let maxWidth = '180px';
  
  // Variante: 'toolbar' (compacto) ou 'form' (com label e help)
  export let variant = 'toolbar';
  export let helpText = '';

  // Estado
  let models = [];
  let loading = true;
  let error = '';
  let pickerComponent;

  // Items formatados para o ComboboxPicker
  $: items = models.map(m => ({ value: m, label: m }));

  onMount(async () => {
    await loadModels();
  });

  async function loadModels() {
    loading = true;
    error = '';
    try {
      models = await GetModels() || [];
    } catch (e) {
      error = 'Erro ao carregar modelos';
      console.error('ModelPicker: erro ao carregar modelos', e);
    } finally {
      loading = false;
    }
  }

  function handleSelect(event) {
    value = event.detail.value;
    dispatch('select', event.detail);
    dispatch('change', value);
  }

  function handleAnnounce(event) {
    dispatch('announce', event.detail);
  }

  // Expõe métodos do picker
  export function open() {
    pickerComponent?.open();
  }

  export function close() {
    pickerComponent?.close();
  }

  export function reload() {
    loadModels();
  }
</script>

{#if variant === 'form'}
  <div class="model-picker-form">
    <label for="model-picker-{label}">{label}</label>
    
    {#if loading}
      <div class="loading-state">
        <span class="loading-spinner"></span>
        Carregando modelos...
      </div>
    {:else if error}
      <div class="error-state">
        <span>{error}</span>
        <button type="button" class="retry-btn" on:click={loadModels}>
          🔄 Tentar novamente
        </button>
      </div>
    {:else}
      <ComboboxPicker
        bind:this={pickerComponent}
        {icon}
        label={value || 'Selecionar modelo'}
        {items}
        bind:selected={value}
        {placeholder}
        {disabled}
        maxWidth="100%"
        on:select={handleSelect}
        on:announce={handleAnnounce}
      />
    {/if}
    
    {#if helpText}
      <small class="help-text">{helpText}</small>
    {/if}
  </div>
{:else}
  <!-- Variante toolbar (compacta) -->
  {#if loading}
    <div class="loading-status">
      <span class="loading-spinner"></span>
      Carregando...
    </div>
  {:else if error}
    <button class="toolbar-btn error-btn" on:click={loadModels}>
      🔄 Recarregar
    </button>
  {:else}
    <ComboboxPicker
      bind:this={pickerComponent}
      {icon}
      {label}
      {items}
      bind:selected={value}
      {placeholder}
      {disabled}
      {maxWidth}
      on:select={handleSelect}
      on:announce={handleAnnounce}
    />
  {/if}
{/if}

<style>
  .model-picker-form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .model-picker-form label {
    font-weight: 500;
    color: var(--color-text-primary, #e0e0e0);
  }

  .loading-state,
  .error-state {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm);
    color: var(--color-text-muted);
    font-size: var(--font-size-sm);
  }

  .error-state {
    color: var(--color-error, #f85149);
  }

  .retry-btn {
    padding: var(--spacing-xs) var(--spacing-sm);
    background: transparent;
    border: 1px solid currentColor;
    border-radius: var(--border-radius);
    color: inherit;
    cursor: pointer;
    font-size: var(--font-size-xs);
  }

  .retry-btn:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .help-text {
    color: var(--color-text-muted, #888);
    font-size: 0.85rem;
  }

  .loading-status {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm);
    color: var(--color-text-muted);
    font-size: var(--font-size-sm);
  }

  .loading-spinner {
    display: inline-block;
    width: 14px;
    height: 14px;
    border: 2px solid var(--color-border, #444);
    border-top-color: var(--color-accent, #58a6ff);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .error-btn {
    display: flex;
    align-items: center;
    gap: var(--spacing-xs);
    padding: var(--spacing-xs) var(--spacing-sm);
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-error, #f85149);
    border-radius: var(--border-radius);
    color: var(--color-error, #f85149);
    font-size: var(--font-size-sm);
    cursor: pointer;
  }

  .error-btn:hover {
    background: rgba(248, 81, 73, 0.1);
  }

  /* Estilo para o ComboboxPicker quando em modo form */
  .model-picker-form :global(.combobox-picker) {
    width: 100%;
  }

  .model-picker-form :global(.picker-button) {
    width: 100%;
    max-width: none;
  }
</style>

