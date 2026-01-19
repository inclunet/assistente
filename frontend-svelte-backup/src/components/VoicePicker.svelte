<script context="module">
  // Valor especial para voz desativada (usa leitor de telas)
  // Exportado como módulo para ser importado em outros componentes
  export const VOICE_DISABLED = '__disabled__';
</script>

<script>
  import { onMount, createEventDispatcher } from 'svelte';
  import { Combobox as ComboboxPicker } from './combobox';
  import { GetSAPI5Voices, GetOpenAITTSVoices } from '../../wailsjs/go/main/App.js';

  const dispatch = createEventDispatcher();

  // Props
  export let value = '';
  export let label = 'Voz';
  export let icon = '🔊';
  export let placeholder = 'Filtrar vozes...';
  export let disabled = false;
  export let maxWidth = '180px';
  export let language = 'pt'; // Filtro de idioma preferido
  export let allowDisabled = true; // Permite opção "desativada"
  
  // Variante: 'toolbar' (compacto) ou 'form' (com label e help)
  export let variant = 'toolbar';
  export let helpText = '';

  // Estado
  let voices = []; // Lista combinada de vozes (WebSpeech + SAPI5)
  let loading = true;
  let pickerComponent;

  // Opção de desativado
  const disabledOption = { 
    value: VOICE_DISABLED, 
    label: '🔇 Desativada (usar leitor de telas)',
    sublabel: 'Acessibilidade'
  };

  // Items formatados para o ComboboxPicker
  $: items = [
    ...(allowDisabled ? [disabledOption] : []),
    ...voices.map(v => ({ 
      value: v.id || v.name,
      label: formatVoiceName(v.name),
      sublabel: getVoiceSublabel(v)
    }))
  ];
  
  function getVoiceSublabel(voice) {
    const source = voice.source;
    if (source === 'openai') {
      return `${voice.description || 'OpenAI TTS'} ✨`;
    } else if (source === 'sapi5') {
      return `${voice.lang || voice.language} (SAPI5)`;
    } else {
      return voice.lang || voice.language || '';
    }
  }
  
  // Verifica se a voz está desativada
  $: isVoiceDisabled = value === VOICE_DISABLED;

  // Verifica suporte WebSpeech
  $: isWebSpeechSupported = typeof window !== 'undefined' && 'speechSynthesis' in window;

  onMount(() => {
    // Carrega vozes em background para não bloquear a interface
    loadAllVoices();
  });

  async function loadAllVoices() {
    loading = true;
    let allVoices = [];
    
    // 1. Carrega vozes OpenAI TTS (premium, colocamos primeiro)
    try {
      const openaiVoices = await loadOpenAIVoices();
      allVoices = [...allVoices, ...openaiVoices];
    } catch (e) {
      console.log('OpenAI TTS voices not available:', e.message);
    }
    
    // 2. Carrega vozes SAPI5 (Windows only)
    try {
      const sapi5Voices = await loadSAPI5Voices();
      allVoices = [...allVoices, ...sapi5Voices];
    } catch (e) {
      // SAPI5 não disponível (provavelmente Linux/Mac) - ignora silenciosamente
      console.log('SAPI5 not available:', e.message);
    }
    
    // 3. Carrega vozes WebSpeech (fallback local)
    if (isWebSpeechSupported) {
      const webVoices = await loadWebSpeechVoices();
      allVoices = [...allVoices, ...webVoices];
    }
    
    // Filtra por idioma preferido (apenas para vozes locais, não OpenAI)
    const preferredVoices = allVoices.filter(v => {
      // OpenAI sempre passa (são multilíngues)
      if (v.source === 'openai') return true;
      const lang = v.lang || v.language || '';
      return lang.startsWith(language);
    });
    voices = preferredVoices.length > 0 ? preferredVoices : allVoices;
    
    // Seleciona voz padrão se nenhuma selecionada
    if (!value && voices.length > 0) {
      if (allowDisabled) {
        value = VOICE_DISABLED;
      } else {
        const defaultVoice = voices.find(v => (v.lang || v.language) === `${language}-BR`) 
          || voices.find(v => v.default)
          || voices[0];
        value = defaultVoice.id || defaultVoice.name;
      }
      dispatch('change', value);
    }
    
    loading = false;
  }

  function loadWebSpeechVoices() {
    return new Promise((resolve) => {
      const getVoices = () => {
        const allVoices = window.speechSynthesis.getVoices();
        if (allVoices.length > 0) {
          // Adiciona source para identificar
          resolve(allVoices.map(v => ({
            id: v.name, // WebSpeech usa name como id
            name: v.name,
            lang: v.lang,
            default: v.default,
            source: 'webspeech'
          })));
        }
        return allVoices.length > 0;
      };
      
      // Tenta imediatamente
      if (getVoices()) return;
      
      // Se não carregou, espera o evento
      if (window.speechSynthesis.onvoiceschanged !== undefined) {
        window.speechSynthesis.onvoiceschanged = () => {
          getVoices();
        };
      }
      
      // Timeout para não ficar esperando para sempre
      setTimeout(() => {
        if (!getVoices()) {
          resolve([]);
        }
      }, 1000);
    });
  }

  async function loadSAPI5Voices() {
    try {
      // Timeout de 5 segundos para evitar travamentos
      const timeoutPromise = new Promise((_, reject) => 
        setTimeout(() => reject(new Error('SAPI5 timeout')), 5000)
      );
      
      const sapi5Voices = await Promise.race([
        GetSAPI5Voices(),
        timeoutPromise
      ]);
      
      if (!sapi5Voices || sapi5Voices.length === 0) {
        return [];
      }
      // Já vem com source: 'sapi5' do backend
      return sapi5Voices.map(v => ({
        id: v.name, // Usamos name como id para consistência
        name: v.name,
        lang: v.language,
        language: v.language,
        gender: v.gender,
        vendor: v.vendor,
        source: 'sapi5'
      }));
    } catch (e) {
      // SAPI5 não disponível ou timeout
      console.log('SAPI5 voices load failed:', e.message);
      return [];
    }
  }

  async function loadOpenAIVoices() {
    try {
      const openaiVoices = await GetOpenAITTSVoices();
      
      if (!openaiVoices || openaiVoices.length === 0) {
        return [];
      }
      
      return openaiVoices.map(v => ({
        id: `openai:${v.id}`, // Prefixo para identificar no handler
        name: v.name,
        description: v.description,
        gender: v.gender,
        lang: 'multilingual', // OpenAI suporta vários idiomas
        source: 'openai'
      }));
    } catch (e) {
      console.log('OpenAI TTS voices load failed:', e.message);
      return [];
    }
  }

  function formatVoiceName(name) {
    // Valor especial para desativado
    if (name === VOICE_DISABLED) {
      return '🔇 Desativada';
    }
    // Remove prefixos comuns para exibição mais limpa
    return name
      .replace('Microsoft ', '')
      .replace(' Desktop', '')
      .replace(' Online (Natural)', '')
      .replace('Google ', '');
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

  // Retorna a voz selecionada como objeto com informações completas
  export function getSelectedVoice() {
    if (value === VOICE_DISABLED) {
      return { id: VOICE_DISABLED, name: 'Desativada', source: 'disabled' };
    }
    return voices.find(v => (v.id === value) || (v.name === value));
  }
  
  // Verifica se a voz selecionada é SAPI5
  export function isSelectedVoiceSAPI5() {
    const voice = getSelectedVoice();
    return voice?.source === 'sapi5';
  }
  
  // Verifica se a voz selecionada é OpenAI
  export function isSelectedVoiceOpenAI() {
    const voice = getSelectedVoice();
    return voice?.source === 'openai';
  }
  
  // Retorna o ID da voz OpenAI sem o prefixo
  export function getOpenAIVoiceId() {
    if (!isSelectedVoiceOpenAI()) return null;
    const voice = getSelectedVoice();
    if (voice?.id?.startsWith('openai:')) {
      return voice.id.substring(7); // Remove "openai:"
    }
    return voice?.id;
  }
</script>

{#if !isWebSpeechSupported && voices.length === 0}
  <div class="not-supported" title="Síntese de voz não suportada neste navegador">
    <span class="icon">🔇</span>
    <span>TTS indisponível</span>
  </div>
{:else if variant === 'form'}
  <div class="voice-picker-form">
    <label for="voice-picker-{label}">{label}</label>
    
    {#if loading}
      <div class="loading-state">
        <span class="loading-spinner"></span>
        Carregando vozes...
      </div>
    {:else if voices.length === 0}
      <div class="empty-state">
        Nenhuma voz disponível
      </div>
    {:else}
      <ComboboxPicker
        bind:this={pickerComponent}
        {icon}
        label={formatVoiceName(value) || 'Selecionar voz'}
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
  {:else if voices.length === 0}
    <!-- Não mostra nada se não há vozes -->
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
  .voice-picker-form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .voice-picker-form label {
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

  .not-supported {
    display: flex;
    align-items: center;
    gap: var(--spacing-xs);
    padding: var(--spacing-xs) var(--spacing-sm);
    color: var(--color-text-muted);
    font-size: var(--font-size-sm);
    opacity: 0.7;
  }

  .not-supported .icon {
    opacity: 0.5;
  }

  /* Estilo para o ComboboxPicker quando em modo form */
  .voice-picker-form :global(.combobox-picker) {
    width: 100%;
  }

  .voice-picker-form :global(.picker-button) {
    width: 100%;
    max-width: none;
  }
</style>

