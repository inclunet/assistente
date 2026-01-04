<script>
  import { createEventDispatcher, tick, onMount } from 'svelte';
  import { TabPanel } from '../tabs';
  import { ModelPicker, VoicePicker, STTProviderPicker, VOICE_DISABLED, STT_WEBSPEECH, STT_WHISPER } from '../pickers';
  
  /**
   * ChatPreferences - Componente de preferências do chat com guias
   * 
   * Usado tanto no modal de preferências por conversa quanto na página de configurações globais.
   * Event-driven: dispara 'change' quando qualquer valor muda.
   * 
   * @event change - Disparado quando qualquer preferência muda
   *   detail: { field: string, value: any, preferences: ChatPreferencesData }
   */
  
  const dispatch = createEventDispatcher();
  
  // ==================== Preferências de Chat ====================
  
  /** Modelo LLM selecionado */
  export let model = '';
  
  /** Temperatura (0.0 - 2.0) */
  export let temperature = 0.7;
  
  /** Máximo de tokens */
  export let maxTokens = 4096;
  
  /** Top P (0.0 - 1.0) */
  export let topP = 1.0;
  
  /** Se usa ferramentas/agentes */
  export let useTools = true;
  
  /** Se mostra mensagens internas (tool calls, debug) */
  export let showInternalMessages = false;
  
  // ==================== Preferências de Voz ====================
  
  /** Voz TTS selecionada (ID ou VOICE_DISABLED) */
  export let voice = VOICE_DISABLED;
  
  /** Se fala respostas automaticamente */
  export let autoSpeak = true;
  
  /** Volume da voz (0-100) */
  export let voiceVolume = 100;
  
  /** Velocidade da voz (-10 a 10) */
  export let voiceRate = 0;
  
  // ==================== Preferências de Transcrição ====================
  
  /** Provedor STT (webspeech ou whisper) */
  export let sttProvider = STT_WEBSPEECH;
  
  /** Modo de gravação (ptt, toggle, vad_silence, vad_activity) */
  export let recordingMode = 'ptt';
  
  // ==================== Props de Controle ====================
  
  /** Se mostra parâmetros avançados (temperatura, top_p, etc.) */
  export let showAdvanced = true;
  
  /** Se está desabilitado */
  export let disabled = false;
  
  /** Se exibe em modo compacto (menos espaçamento) */
  export let compact = false;
  
  /** Se desabilita os atalhos de teclado do TabPanel */
  export let disableTabShortcuts = true;
  
  /** Aba inicial ativa */
  export let initialTab = 'chat';
  
  // ==================== Estado Interno ====================
  
  let activeTab = initialTab;
  
  // Definição das guias
  const tabs = [
    { id: 'chat', label: 'Chat', icon: '💬' },
    { id: 'voice', label: 'Voz', icon: '🔊' },
    { id: 'stt', label: 'Transcrição', icon: '🎤' }
  ];
  
  // Modos de gravação disponíveis
  const RECORDING_MODES = [
    { id: 'ptt', label: 'Push-to-Talk', description: 'Segure para falar' },
    { id: 'toggle', label: 'Toggle', description: 'Clique para iniciar/parar' },
    { id: 'vad_silence', label: 'VAD Silêncio', description: 'Para automaticamente no silêncio' },
    { id: 'vad_activity', label: 'VAD Atividade', description: 'Inicia e para automaticamente' }
  ];
  
  // Referências para componentes
  let voicePickerComponent;
  let modelPickerComponent;
  let sttPickerComponent;
  
  /**
   * Retorna todas as preferências atuais como objeto
   */
  export function getPreferences() {
    return {
      // Chat
      model,
      temperature,
      maxTokens,
      topP,
      useTools,
      showInternalMessages,
      // Voz
      voice,
      autoSpeak,
      voiceVolume,
      voiceRate,
      // STT
      sttProvider,
      recordingMode
    };
  }
  
  /**
   * Define todas as preferências de uma vez
   */
  export function setPreferences(prefs) {
    if (!prefs) return;
    
    if (prefs.model !== undefined) model = prefs.model;
    if (prefs.temperature !== undefined) temperature = prefs.temperature;
    if (prefs.maxTokens !== undefined) maxTokens = prefs.maxTokens;
    if (prefs.topP !== undefined) topP = prefs.topP;
    if (prefs.useTools !== undefined) useTools = prefs.useTools;
    if (prefs.showInternalMessages !== undefined) showInternalMessages = prefs.showInternalMessages;
    if (prefs.voice !== undefined) voice = prefs.voice;
    if (prefs.autoSpeak !== undefined) autoSpeak = prefs.autoSpeak;
    if (prefs.voiceVolume !== undefined) voiceVolume = prefs.voiceVolume;
    if (prefs.voiceRate !== undefined) voiceRate = prefs.voiceRate;
    if (prefs.sttProvider !== undefined) sttProvider = prefs.sttProvider;
    if (prefs.recordingMode !== undefined) recordingMode = prefs.recordingMode;
  }
  
  /**
   * Handler genérico de mudança
   */
  function handleChange(field, value) {
    dispatch('change', {
      field,
      value,
      preferences: getPreferences()
    });
  }
  
  // Handlers específicos que chamam handleChange
  function onModelChange(e) {
    model = e.detail;
    handleChange('model', model);
  }
  
  function onVoiceChange(e) {
    voice = e.detail;
    handleChange('voice', voice);
  }
  
  function onSTTChange(e) {
    sttProvider = e.detail;
    handleChange('sttProvider', sttProvider);
  }
</script>

<div class="chat-preferences" class:compact class:disabled>
  <TabPanel 
    {tabs} 
    bind:activeTab
    variant="underline"
    size="sm"
  >
    <svelte:fragment slot="tab-content" let:tab>
      {#if tab.id === 'chat'}
        <!-- ==================== ABA: CHAT ==================== -->
        <div class="preferences-section">
          <div class="form-group">
            <ModelPicker 
              bind:this={modelPickerComponent}
              bind:value={model}
              label="Modelo LLM"
              placeholder="Selecione ou digite um modelo"
              helpText="Modelo de linguagem para esta conversa"
              allowCustom={true}
              variant="form"
              {disabled}
              on:change={onModelChange}
            />
          </div>
          
          {#if showAdvanced}
            <div class="form-group">
              <label for="pref-temperature">
                Temperatura: <strong>{temperature.toFixed(1)}</strong>
              </label>
              <p class="param-description">
                Controla a criatividade. Valores menores são mais precisos, maiores são mais criativos.
              </p>
              <input
                id="pref-temperature"
                type="range"
                bind:value={temperature}
                on:change={() => handleChange('temperature', temperature)}
                min="0"
                max="2"
                step="0.1"
                {disabled}
              />
              <div class="range-labels">
                <span>Preciso (0)</span>
                <span>Criativo (2)</span>
              </div>
            </div>

            <div class="form-group">
              <label for="pref-max-tokens">
                Máximo de Tokens: <strong>{maxTokens}</strong>
              </label>
              <p class="param-description">
                Limite de tokens na resposta. Valores maiores permitem respostas mais longas.
              </p>
              <input
                id="pref-max-tokens"
                type="range"
                bind:value={maxTokens}
                on:change={() => handleChange('maxTokens', maxTokens)}
                min="100"
                max="16000"
                step="100"
                {disabled}
              />
              <div class="range-labels">
                <span>100</span>
                <span>16000</span>
              </div>
            </div>

            <div class="form-group">
              <label for="pref-top-p">
                Top P: <strong>{topP.toFixed(2)}</strong>
              </label>
              <p class="param-description">
                Controla a diversidade via nucleus sampling.
              </p>
              <input
                id="pref-top-p"
                type="range"
                bind:value={topP}
                on:change={() => handleChange('topP', topP)}
                min="0"
                max="1"
                step="0.05"
                {disabled}
              />
              <div class="range-labels">
                <span>Focado (0)</span>
                <span>Diverso (1)</span>
              </div>
            </div>
          {/if}
          
          <div class="form-group checkbox-group">
            <label class="checkbox-label">
              <input
                type="checkbox"
                bind:checked={useTools}
                on:change={() => handleChange('useTools', useTools)}
                {disabled}
              />
              <span>Habilitar agentes e ferramentas</span>
            </label>
            <p class="param-description">
              Permite que o assistente use ferramentas como FAQ, arquivos, HTTP, etc.
            </p>
          </div>
          
          <div class="form-group checkbox-group">
            <label class="checkbox-label">
              <input
                type="checkbox"
                bind:checked={showInternalMessages}
                on:change={() => handleChange('showInternalMessages', showInternalMessages)}
                {disabled}
              />
              <span>Mostrar mensagens internas</span>
            </label>
            <p class="param-description">
              Exibe tool calls e respostas de agentes (útil para debug).
            </p>
          </div>
        </div>
      {:else if tab.id === 'voice'}
        <!-- ==================== ABA: VOZ ==================== -->
        <div class="preferences-section">
          <div class="form-group">
            <VoicePicker
              bind:this={voicePickerComponent}
              bind:value={voice}
              label="Voz TTS"
              language="pt"
              {disabled}
              on:change={onVoiceChange}
            />
            <p class="param-description">
              Selecione uma voz para síntese de fala ou desative para usar leitor de telas.
            </p>
          </div>
          
          <div class="form-group checkbox-group">
            <label class="checkbox-label">
              <input
                type="checkbox"
                bind:checked={autoSpeak}
                on:change={() => handleChange('autoSpeak', autoSpeak)}
                disabled={disabled || voice === VOICE_DISABLED}
              />
              <span>Falar respostas automaticamente</span>
            </label>
            <p class="param-description">
              O assistente fala as respostas automaticamente quando recebidas.
            </p>
          </div>
          
          <div class="form-group">
            <label for="pref-voice-volume">
              Volume: <strong>{voiceVolume}%</strong>
            </label>
            <input
              id="pref-voice-volume"
              type="range"
              bind:value={voiceVolume}
              on:change={() => handleChange('voiceVolume', voiceVolume)}
              min="0"
              max="100"
              step="5"
              disabled={disabled || voice === VOICE_DISABLED}
            />
            <div class="range-labels">
              <span>0%</span>
              <span>100%</span>
            </div>
          </div>
          
          <div class="form-group">
            <label for="pref-voice-rate">
              Velocidade: <strong>{voiceRate > 0 ? '+' : ''}{voiceRate}</strong>
            </label>
            <input
              id="pref-voice-rate"
              type="range"
              bind:value={voiceRate}
              on:change={() => handleChange('voiceRate', voiceRate)}
              min="-10"
              max="10"
              step="1"
              disabled={disabled || voice === VOICE_DISABLED}
            />
            <div class="range-labels">
              <span>Lento (-10)</span>
              <span>Rápido (+10)</span>
            </div>
          </div>
        </div>
      {:else if tab.id === 'stt'}
        <!-- ==================== ABA: TRANSCRIÇÃO ==================== -->
        <div class="preferences-section">
          <div class="form-group">
            <STTProviderPicker
              bind:this={sttPickerComponent}
              bind:value={sttProvider}
              label="Provedor de Transcrição"
              {disabled}
              on:change={onSTTChange}
            />
            <p class="param-description">
              WebSpeech usa o navegador (grátis). Whisper usa OpenAI (melhor qualidade).
            </p>
          </div>
          
          <fieldset class="form-group" disabled={disabled}>
            <legend>Modo de Gravação</legend>
            
            <div class="radio-group">
              {#each RECORDING_MODES as mode}
                <label class="radio-label">
                  <input
                    type="radio"
                    name="recording-mode"
                    value={mode.id}
                    bind:group={recordingMode}
                    on:change={() => handleChange('recordingMode', recordingMode)}
                    {disabled}
                  />
                  <span class="radio-content">
                    <strong>{mode.label}</strong>
                    <small>{mode.description}</small>
                  </span>
                </label>
              {/each}
            </div>
          </fieldset>
        </div>
      {/if}
    </svelte:fragment>
  </TabPanel>
</div>

<style>
  .chat-preferences {
    width: 100%;
    height: 100%;
  }
  
  .chat-preferences.disabled {
    opacity: 0.6;
    pointer-events: none;
  }
  
  .preferences-section {
    padding: var(--spacing-lg, 16px);
    display: flex;
    flex-direction: column;
    gap: var(--spacing-lg, 16px);
  }
  
  .compact .preferences-section {
    padding: var(--spacing-md, 12px);
    gap: var(--spacing-md, 12px);
  }
  
  .form-group {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-xs, 4px);
  }
  
  .form-group label {
    font-weight: 500;
    color: var(--color-text-primary, #e6e6e6);
    font-size: var(--font-size-base, 1rem);
  }
  
  .form-group label strong {
    color: var(--color-accent, #58a6ff);
    font-weight: 600;
  }
  
  .param-description {
    font-size: var(--font-size-sm, 0.875rem);
    color: var(--color-text-secondary, #8b949e);
    margin: 0;
    line-height: 1.4;
  }
  
  /* Range inputs */
  input[type="range"] {
    width: 100%;
    height: 8px;
    -webkit-appearance: none;
    appearance: none;
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
    appearance: none;
    width: 20px;
    height: 20px;
    background: var(--color-accent, #58a6ff);
    border-radius: 50%;
    cursor: pointer;
  }

  input[type="range"]::-moz-range-thumb {
    width: 20px;
    height: 20px;
    background: var(--color-accent, #58a6ff);
    border-radius: 50%;
    cursor: pointer;
    border: none;
  }
  
  input[type="range"]:focus-visible {
    outline: 2px solid var(--color-accent, #58a6ff);
    outline-offset: 2px;
  }
  
  .range-labels {
    display: flex;
    justify-content: space-between;
    font-size: var(--font-size-xs, 0.75rem);
    color: var(--color-text-muted, #6e7681);
    margin-top: var(--spacing-xs, 4px);
  }
  
  /* Checkboxes */
  .checkbox-group {
    padding: var(--spacing-sm, 8px) 0;
  }
  
  .checkbox-label {
    display: flex;
    align-items: flex-start;
    gap: var(--spacing-sm, 8px);
    cursor: pointer;
    font-weight: 400 !important;
  }
  
  .checkbox-label input[type="checkbox"] {
    width: 20px;
    height: 20px;
    margin: 0;
    cursor: pointer;
    accent-color: var(--color-accent, #58a6ff);
  }
  
  .checkbox-label input[type="checkbox"]:disabled {
    cursor: not-allowed;
  }
  
  .checkbox-label span {
    flex: 1;
  }
  
  /* Radio buttons */
  fieldset {
    border: 1px solid var(--color-border, #30363d);
    border-radius: var(--border-radius, 6px);
    padding: var(--spacing-md, 12px);
    margin: 0;
  }
  
  fieldset:disabled {
    opacity: 0.5;
  }
  
  fieldset legend {
    font-weight: 600;
    color: var(--color-text-primary, #e6e6e6);
    padding: 0 var(--spacing-xs, 4px);
  }
  
  .radio-group {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-sm, 8px);
    margin-top: var(--spacing-sm, 8px);
  }
  
  .radio-label {
    display: flex;
    align-items: flex-start;
    gap: var(--spacing-sm, 8px);
    cursor: pointer;
    padding: var(--spacing-xs, 4px);
    border-radius: var(--border-radius, 6px);
    transition: background-color var(--transition-fast, 150ms);
  }
  
  .radio-label:hover {
    background-color: var(--color-bg-tertiary, #21262d);
  }
  
  .radio-label input[type="radio"] {
    width: 18px;
    height: 18px;
    margin: 2px 0 0 0;
    cursor: pointer;
    accent-color: var(--color-accent, #58a6ff);
  }
  
  .radio-label input[type="radio"]:disabled {
    cursor: not-allowed;
  }
  
  .radio-content {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  
  .radio-content strong {
    color: var(--color-text-primary, #e6e6e6);
    font-weight: 500;
  }
  
  .radio-content small {
    color: var(--color-text-secondary, #8b949e);
    font-size: var(--font-size-sm, 0.875rem);
  }
  
  /* Responsivo */
  @media (max-width: 480px) {
    .preferences-section {
      padding: var(--spacing-md, 12px);
    }
  }
</style>

