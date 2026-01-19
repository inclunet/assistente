<script>
  import { createEventDispatcher, onMount, onDestroy } from 'svelte';
  import { ttsService, TTS_PROVIDERS } from '../../lib/speech/tts-service.js';
  
  const dispatch = createEventDispatcher();
  
  // Props
  /** @type {number} Volume atual (0-100) */
  export let volume = 100;
  
  /** @type {number} Velocidade atual (-10 a 10) */
  export let rate = 0;
  
  /** @type {boolean} Se deve falar respostas automaticamente */
  export let autoSpeak = true;
  
  /** @type {string} Nome da voz selecionada */
  export let selectedVoice = '';
  
  /** @type {string} Fonte da voz: 'disabled' | 'webspeech' | 'sapi5' | 'openai' */
  export let voiceSource = 'disabled';
  
  /** @type {string} Texto para teste de voz */
  export let testText = 'Olá! Esta é uma demonstração da voz selecionada.';
  
  /** @type {boolean} Se mostra o toggle de auto-speak */
  export let showAutoSpeak = true;
  
  /** @type {boolean} Se mostra informações da voz atual */
  export let showVoiceInfo = true;
  
  /** @type {boolean} Se mostra botão de testar voz */
  export let showTestButton = true;
  
  // Estado local
  let isTesting = false;
  
  // Referência para focus
  let volumeInput;
  
  // Computed
  $: isDisabled = voiceSource === 'disabled';
  $: voiceDisplayName = getVoiceDisplayName(selectedVoice, voiceSource);
  $: voiceSourceLabel = getVoiceSourceLabel(voiceSource);
  
  function getVoiceDisplayName(voice, source) {
    if (!voice || source === 'disabled') {
      return 'Desativada';
    }
    if (voice.startsWith('openai:')) {
      return voice.substring(7);
    }
    return voice;
  }
  
  function getVoiceSourceLabel(source) {
    switch (source) {
      case 'openai': return 'OpenAI ✨';
      case 'sapi5': return 'SAPI5';
      case 'webspeech': return 'WebSpeech';
      default: return '';
    }
  }
  
  // === Handlers ===
  
  async function handleVolumeChange() {
    await ttsService.setVolume(volume);
    dispatch('volumeChange', { volume });
  }
  
  async function handleRateChange() {
    await ttsService.setRate(rate);
    dispatch('rateChange', { rate });
  }
  
  function handleAutoSpeakChange() {
    dispatch('autoSpeakChange', { autoSpeak });
  }
  
  async function testVoice() {
    if (isDisabled || isTesting) return;
    
    isTesting = true;
    
    try {
      await ttsService.speak(testText);
    } catch (e) {
      console.error('Erro ao testar voz:', e);
    }
    
    isTesting = false;
  }
  
  function stopTest() {
    ttsService.stop();
    isTesting = false;
  }
  
  // Listeners do serviço
  function handleSpeakEnd() {
    isTesting = false;
  }
  
  onMount(() => {
    ttsService.addEventListener('speakEnd', handleSpeakEnd);
    
    // Foca no primeiro input
    setTimeout(() => volumeInput?.focus(), 50);
  });
  
  onDestroy(() => {
    ttsService.removeEventListener('speakEnd', handleSpeakEnd);
  });
  
  /**
   * Foca no painel (método público)
   */
  export function focus() {
    volumeInput?.focus();
  }
</script>

<div class="voice-settings-panel">
  <!-- Volume -->
  <div class="setting-group">
    <label for="voice-volume" class="setting-label">
      Volume: <strong>{volume}%</strong>
    </label>
    <p class="setting-description">
      Ajusta o volume da síntese de voz.
    </p>
    <input
      id="voice-volume"
      type="range"
      bind:this={volumeInput}
      bind:value={volume}
      on:change={handleVolumeChange}
      min="0"
      max="100"
      step="5"
      aria-valuemin="0"
      aria-valuemax="100"
      aria-valuenow={volume}
      disabled={isDisabled}
    />
    <div class="range-labels" aria-hidden="true">
      <span>🔇 0%</span>
      <span>🔊 100%</span>
    </div>
  </div>
  
  <!-- Velocidade -->
  <div class="setting-group">
    <label for="voice-rate" class="setting-label">
      Velocidade: <strong>{rate > 0 ? '+' : ''}{rate}</strong>
    </label>
    <p class="setting-description">
      Ajusta a velocidade da fala. Valores negativos são mais lentos, positivos são mais rápidos.
    </p>
    <input
      id="voice-rate"
      type="range"
      bind:value={rate}
      on:change={handleRateChange}
      min="-10"
      max="10"
      step="1"
      aria-valuemin="-10"
      aria-valuemax="10"
      aria-valuenow={rate}
      disabled={isDisabled}
    />
    <div class="range-labels" aria-hidden="true">
      <span>🐢 Lento (-10)</span>
      <span>🐇 Rápido (+10)</span>
    </div>
  </div>
  
  <!-- Auto-speak -->
  {#if showAutoSpeak}
    <div class="setting-group">
      <label class="toggle-label">
        <input
          type="checkbox"
          bind:checked={autoSpeak}
          on:change={handleAutoSpeakChange}
          aria-describedby="autospeak-description"
        />
        Falar respostas automaticamente
      </label>
      <p id="autospeak-description" class="setting-description">
        Quando ativado, o assistente fala as respostas automaticamente usando a voz selecionada.
      </p>
    </div>
  {/if}
  
  <!-- Informação da voz atual -->
  {#if showVoiceInfo}
    <div class="voice-info" role="note">
      <strong>Voz atual:</strong> 
      {#if isDisabled}
        🔇 Desativada (usando leitor de telas)
      {:else}
        {voiceDisplayName} 
        <span class="voice-source">({voiceSourceLabel})</span>
      {/if}
    </div>
  {/if}
  
  <!-- Botão de teste -->
  {#if showTestButton}
    <div class="actions">
      {#if isTesting}
        <button 
          type="button"
          class="btn-secondary"
          on:click={stopTest}
        >
          ⏹️ Parar
        </button>
      {:else}
        <button 
          type="button"
          class="btn-secondary"
          on:click={testVoice}
          disabled={isDisabled}
        >
          🔊 Testar Voz
        </button>
      {/if}
    </div>
  {/if}
</div>

<style>
  .voice-settings-panel {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-lg, 24px);
  }
  
  .setting-group {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-xs, 4px);
  }
  
  .setting-label {
    font-weight: 500;
    color: var(--color-text-primary, #f0f6fc);
    margin: 0;
  }
  
  .setting-label strong {
    color: var(--color-accent, #58a6ff);
  }
  
  .setting-description {
    font-size: var(--font-size-sm, 0.875rem);
    color: var(--color-text-muted, #8b949e);
    margin: 0 0 var(--spacing-sm, 8px) 0;
  }
  
  /* Range inputs */
  input[type="range"] {
    width: 100%;
    height: 8px;
    background: var(--color-bg-tertiary, #21262d);
    border-radius: 4px;
    outline: none;
    cursor: pointer;
  }
  
  input[type="range"]:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  input[type="range"]::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 20px;
    height: 20px;
    background: var(--color-accent, #58a6ff);
    border-radius: 50%;
    cursor: pointer;
    transition: transform 0.15s ease;
  }
  
  input[type="range"]::-webkit-slider-thumb:hover {
    transform: scale(1.1);
  }
  
  input[type="range"]:focus-visible {
    box-shadow: 0 0 0 3px var(--color-accent, #58a6ff);
  }
  
  .range-labels {
    display: flex;
    justify-content: space-between;
    font-size: var(--font-size-sm, 0.75rem);
    color: var(--color-text-muted, #8b949e);
    margin-top: var(--spacing-xs, 4px);
  }
  
  /* Toggle */
  .toggle-label {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm, 8px);
    font-weight: 500;
    color: var(--color-text-primary, #f0f6fc);
    cursor: pointer;
  }
  
  .toggle-label input[type="checkbox"] {
    width: 18px;
    height: 18px;
    cursor: pointer;
    accent-color: var(--color-accent, #58a6ff);
  }
  
  /* Voice info */
  .voice-info {
    padding: var(--spacing-md, 16px);
    background: var(--color-bg-tertiary, #21262d);
    border-radius: var(--border-radius, 8px);
    font-size: var(--font-size-sm, 0.875rem);
    color: var(--color-text-secondary, #c9d1d9);
  }
  
  .voice-source {
    color: var(--color-text-muted, #8b949e);
    font-size: var(--font-size-sm, 0.75rem);
  }
  
  /* Actions */
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--spacing-sm, 8px);
    padding-top: var(--spacing-md, 16px);
    border-top: 1px solid var(--color-border, #30363d);
  }
  
  .btn-secondary {
    padding: var(--spacing-sm, 8px) var(--spacing-md, 16px);
    background: var(--color-bg-tertiary, #21262d);
    color: var(--color-text-primary, #f0f6fc);
    border: 1px solid var(--color-border, #30363d);
    border-radius: var(--border-radius, 8px);
    font-size: var(--font-size-sm, 0.875rem);
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s ease;
  }
  
  .btn-secondary:hover:not(:disabled) {
    background: var(--color-border, #30363d);
  }
  
  .btn-secondary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .btn-secondary:focus-visible {
    outline: 2px solid var(--color-accent, #58a6ff);
    outline-offset: 2px;
  }
</style>




