<script context="module">
  /**
   * Status de suporte a visão de um modelo
   */
  export const VisionStatus = {
    UNKNOWN: 'unknown',   // Nunca testado
    SUPPORTED: 'supported', // Confirmado que funciona
    NOT_SUPPORTED: 'not_supported', // Confirmado que não funciona
    ERROR: 'error' // Erro ao testar
  };
  
  /**
   * Verifica se um modelo suporta visão baseado APENAS nos dados do banco
   * Não usa heurísticas - o sistema aprende conforme o usuário usa
   * 
   * @param {string} modelName 
   * @param {Object} capabilitiesCache - Cache de capacidades do backend
   * @returns {string} VisionStatus
   */
  export function getVisionStatus(modelName, capabilitiesCache = {}) {
    if (!modelName) return VisionStatus.UNKNOWN;
    
    const cap = capabilitiesCache[modelName];
    if (!cap) return VisionStatus.UNKNOWN;
    
    if (cap.supports_vision === true) return VisionStatus.SUPPORTED;
    if (cap.supports_vision === false) return VisionStatus.NOT_SUPPORTED;
    
    return VisionStatus.UNKNOWN;
  }
</script>

<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { GetModels, GetAllModelCapabilities } from '../../wailsjs/go/main/App.js';
  import { Combobox as ComboboxPicker } from './combobox';

  const dispatch = createEventDispatcher();

  // Props
  export let value = '';
  export let label = 'Modelo de Imagem';
  export let icon = '🖼️';
  export let placeholder = 'Filtrar modelos...';
  export let disabled = false;
  export let maxWidth = '180px';
  
  /** Mostrar apenas modelos com suporte confirmado a visão */
  export let filterVisionOnly = false; // Agora mostra todos por padrão
  
  /** Modelo de chat principal (para detectar se precisa de modelo auxiliar) */
  export let chatModel = '';
  
  // Variante: 'toolbar' (compacto) ou 'form' (com label e help)
  export let variant = 'toolbar';
  export let helpText = 'Selecione um modelo para processar imagens';

  // Estado
  let models = [];
  let capabilities = {}; // Cache de capacidades do banco
  let loading = true;
  let error = '';
  let pickerComponent;
  
  // Status de visão do chat model
  $: chatModelVisionStatus = getVisionStatus(chatModel, capabilities);
  $: chatModelSupportsVision = chatModelVisionStatus === VisionStatus.SUPPORTED;
  $: chatModelVisionUnknown = chatModelVisionStatus === VisionStatus.UNKNOWN;
  
  // Função para obter status de visão de um modelo
  function getModelVisionStatus(modelName) {
    return getVisionStatus(modelName, capabilities);
  }
  
  // Modelos filtrados
  $: filteredModels = filterVisionOnly 
    ? models.filter(m => getModelVisionStatus(m) === VisionStatus.SUPPORTED)
    : models;

  // Gera sublabel baseado no status
  function getModelSublabel(modelName) {
    const status = getModelVisionStatus(modelName);
    switch (status) {
      case VisionStatus.SUPPORTED: return '✓ Visão';
      case VisionStatus.NOT_SUPPORTED: return '✗ Sem visão';
      case VisionStatus.ERROR: return '⚠️ Erro';
      default: return ''; // Não mostra nada para desconhecido
    }
  }

  // Items formatados para o ComboboxPicker
  $: items = [
    // Opção automática (usar modelo de chat)
    { 
      value: '', 
      label: 'Usar modelo do chat', 
      sublabel: chatModelSupportsVision 
        ? '✓ Suporta' 
        : chatModelVisionUnknown 
          ? 'Não testado' 
          : '✗ Não suporta'
    },
    ...filteredModels.map(m => ({ 
      value: m, 
      label: m,
      sublabel: getModelSublabel(m)
    }))
  ];
  
  // Precisa de modelo auxiliar?
  // Sim se: chat não suporta visão E não tem modelo de imagem selecionado
  // Mas se chat é desconhecido, vamos tentar mesmo assim
  $: needsImageModel = chatModelVisionStatus === VisionStatus.NOT_SUPPORTED && !value;

  onMount(async () => {
    await Promise.all([loadModels(), loadCapabilities()]);
  });

  async function loadModels() {
    loading = true;
    error = '';
    try {
      models = await GetModels() || [];
    } catch (e) {
      error = 'Erro ao carregar modelos';
      console.error('ImageModelPicker: erro ao carregar modelos', e);
    } finally {
      loading = false;
    }
  }
  
  async function loadCapabilities() {
    try {
      const caps = await GetAllModelCapabilities() || [];
      // Converte array para objeto indexado por nome
      capabilities = caps.reduce((acc, cap) => {
        acc[cap.model_name] = cap;
        return acc;
      }, {});
    } catch (e) {
      console.warn('ImageModelPicker: erro ao carregar capacidades', e);
    }
  }
  
  /**
   * Recarrega capacidades (chamar após aprender nova capacidade)
   */
  export async function refreshCapabilities() {
    await loadCapabilities();
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
  
  /**
   * Retorna o modelo efetivo para processar imagens
   * @returns {string} modelo a usar para imagens
   */
  export function getEffectiveModel() {
    if (value) return value;
    if (chatModelSupportsVision) return chatModel;
    return '';
  }
</script>

{#if variant === 'form'}
  <div class="image-model-picker-form">
    <label for="image-model-picker">{label}</label>
    
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
        label={value || 'Automático'}
        {items}
        bind:selected={value}
        {placeholder}
        {disabled}
        maxWidth="100%"
        on:select={handleSelect}
        on:announce={handleAnnounce}
      />
    {/if}
    
    {#if needsImageModel}
      <small class="warning-text">
        ⚠️ O modelo de chat não suporta imagens. Selecione um modelo auxiliar.
      </small>
    {:else if helpText}
      <small class="help-text">{helpText}</small>
    {/if}
  </div>
{:else}
  <!-- Variante toolbar (compacta) -->
  {#if loading}
    <div class="loading-status">
      <span class="loading-spinner"></span>
    </div>
  {:else if error}
    <button class="toolbar-btn error-btn" on:click={loadModels} title="Erro ao carregar. Clique para tentar novamente.">
      🔄
    </button>
  {:else}
    <div class="toolbar-picker" class:warning={needsImageModel}>
      <ComboboxPicker
        bind:this={pickerComponent}
        {icon}
        label={value || 'Auto'}
        {items}
        bind:selected={value}
        {placeholder}
        {disabled}
        {maxWidth}
        on:select={handleSelect}
        on:announce={handleAnnounce}
      />
      {#if needsImageModel}
        <span class="warning-indicator" title="Modelo de chat não suporta imagens">⚠️</span>
      {/if}
    </div>
  {/if}
{/if}

<style>
  .image-model-picker-form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .image-model-picker-form label {
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
  
  .warning-text {
    color: var(--color-warning, #d29922);
    font-size: 0.85rem;
  }

  .loading-status {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--spacing-sm);
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
    justify-content: center;
    width: 32px;
    height: 32px;
    padding: 0;
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
  
  .toolbar-picker {
    display: flex;
    align-items: center;
    position: relative;
  }
  
  .toolbar-picker.warning :global(.picker-button) {
    border-color: var(--color-warning, #d29922);
  }
  
  .warning-indicator {
    position: absolute;
    top: -4px;
    right: -4px;
    font-size: 12px;
    line-height: 1;
    pointer-events: none;
  }

  /* Estilo para o ComboboxPicker quando em modo form */
  .image-model-picker-form :global(.combobox-picker) {
    width: 100%;
  }

  .image-model-picker-form :global(.picker-button) {
    width: 100%;
    max-width: none;
  }
</style>

