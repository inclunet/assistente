<script context="module">
  // Constantes de provedores STT
  export const STT_WEBSPEECH = 'webspeech';
  export const STT_WHISPER = 'whisper';
  // Futuros provedores
  export const STT_AZURE = 'azure';
  export const STT_GOOGLE = 'google';
  export const STT_REALTIME = 'realtime'; // GPT-4o Realtime
</script>

<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { Combobox as ComboboxPicker } from '../combobox';

  const dispatch = createEventDispatcher();

  // Props
  export let value = STT_WEBSPEECH;
  export let label = 'STT';
  export let icon = '🎤';
  export let placeholder = 'Filtrar provedores...';
  export let disabled = false;
  export let maxWidth = '180px';
  
  // Variante: 'toolbar' (compacto) ou 'form' (com label e help)
  export let variant = 'toolbar';
  export let helpText = '';

  // Estado
  let providers = [];
  let loading = true;
  let pickerComponent;

  // Provedores disponíveis
  const allProviders = [
    {
      id: STT_WEBSPEECH,
      name: 'WebSpeech',
      description: 'Navegador (grátis)',
      icon: '🌐',
      available: true,
      requiresApiKey: false
    },
    {
      id: STT_WHISPER,
      name: 'Whisper',
      description: 'OpenAI (premium)',
      icon: '🤖',
      available: true, // Verifica se tem API key
      requiresApiKey: true
    },
    {
      id: STT_REALTIME,
      name: 'GPT-4o Realtime',
      description: 'Multimodal (em breve)',
      icon: '🚀',
      available: false,
      requiresApiKey: true
    }
  ];

  // Items formatados para o ComboboxPicker
  $: items = providers.map(p => ({
    value: p.id,
    label: `${p.icon} ${p.name}`,
    sublabel: p.description,
    disabled: !p.available
  }));

  // Verifica suporte WebSpeech
  $: isWebSpeechSupported = typeof window !== 'undefined' && 
    ('SpeechRecognition' in window || 'webkitSpeechRecognition' in window);

  onMount(() => {
    loadProviders();
  });

  function loadProviders() {
    loading = true;
    
    // Filtra provedores disponíveis
    providers = allProviders.filter(p => {
      // WebSpeech só se suportado
      if (p.id === STT_WEBSPEECH && !isWebSpeechSupported) {
        return false;
      }
      return true;
    });
    
    // Se valor atual não está disponível, seleciona primeiro disponível
    const currentAvailable = providers.find(p => p.id === value && p.available);
    if (!currentAvailable) {
      const firstAvailable = providers.find(p => p.available);
      if (firstAvailable) {
        value = firstAvailable.id;
        dispatch('change', value);
      }
    }
    
    loading = false;
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

  // Retorna o provedor selecionado
  export function getSelectedProvider() {
    return providers.find(p => p.id === value);
  }
  
  // Verifica se o provedor selecionado requer API key
  export function requiresApiKey() {
    const provider = getSelectedProvider();
    return provider?.requiresApiKey || false;
  }
</script>

{#if variant === 'form'}
  <div class="stt-picker-form">
    <label for="stt-picker-{label}">{label}</label>
    
    {#if loading}
      <div class="loading-state">
        <span class="loading-spinner"></span>
        Carregando provedores...
      </div>
    {:else if providers.length === 0}
      <div class="empty-state">
        Nenhum provedor disponível
      </div>
    {:else}
      <ComboboxPicker
        bind:this={pickerComponent}
        {icon}
        label={providers.find(p => p.id === value)?.name || 'Selecionar'}
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
    </div>
  {:else if providers.length === 0}
    <!-- Não mostra nada se não há provedores -->
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
  .stt-picker-form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .stt-picker-form label {
    font-weight: 500;
    color: var(--color-text-primary, #e0e0e0);
  }

  .loading-state,
  .empty-state {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm);
    color: var(--color-text-muted);
    font-size: var(--font-size-sm);
  }

  .help-text {
    color: var(--color-text-muted, #888);
    font-size: 0.85rem;
  }

  .loading-status {
    display: flex;
    align-items: center;
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
</style>

