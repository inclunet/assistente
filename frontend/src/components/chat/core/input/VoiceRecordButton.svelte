<script>
  /**
   * VoiceRecordButton - Botão de gravação de voz acessível
   * 
   * Componente "dumb" que apenas renderiza UI e dispara eventos.
   * A lógica de gravação deve ser gerenciada pelo componente pai.
   * 
   * Eventos:
   *   - recordStart: Usuário quer iniciar gravação
   *   - recordStop: Usuário quer parar gravação
   *   - recordCancel: Usuário quer cancelar gravação (Escape)
   * 
   * Uso:
   *   <VoiceRecordButton
   *     mode="ptt"
   *     state="idle"
   *     on:recordStart={handleStart}
   *     on:recordStop={handleStop}
   *   />
   */
  import { createEventDispatcher } from 'svelte';
  
  const dispatch = createEventDispatcher();
  
  // === Props ===
  
  /** Se o botão está desabilitado */
  export let disabled = false;
  
  /** 
   * Estado atual do botão:
   * - 'idle': Pronto para gravar
   * - 'listening': Esperando atividade de voz (VAD)
   * - 'recording': Gravando
   * - 'processing': Processando transcrição
   * - 'error': Erro ocorreu
   */
  export let state = 'idle';
  
  /**
   * Modo de operação:
   * - 'ptt': Push-to-Talk - segura para gravar
   * - 'toggle': Clique para iniciar/parar
   * - 'vad_silence': Clique + detecta silêncio para parar
   * - 'vad_activity': Espera atividade de voz para iniciar
   * - 'record_audio': Grava áudio como arquivo
   */
  export let mode = 'ptt';
  
  /** Volume atual (0-1) para indicador visual */
  export let volume = 0;
  
  /** Texto de preview (transcrição parcial) */
  export let previewText = '';
  
  /** Mensagem de erro */
  export let errorMessage = '';
  
  /** Ícone customizado (usa slot se não definido) */
  export let icon = '';
  
  /** Label para acessibilidade (calculado automaticamente se não definido) */
  export let ariaLabel = '';
  
  // === Computed ===
  
  $: isRecording = state === 'recording';
  $: isListening = state === 'listening';
  $: isProcessing = state === 'processing';
  $: isError = state === 'error';
  $: isActive = isRecording || isListening;
  $: isBusy = isRecording || isProcessing || isListening;
  
  $: volumePercent = Math.min(100, volume * 500);
  
  $: computedIcon = icon || getDefaultIcon(state, mode);
  $: computedAriaLabel = ariaLabel || getDefaultAriaLabel(state, mode, errorMessage);
  
  function getDefaultIcon(state, mode) {
    switch (state) {
      case 'listening': return '👂';
      case 'recording': return '🔴';
      case 'processing': return '⏳';
      case 'error': return '❌';
      default:
        switch (mode) {
          case 'record_audio': return '🎙️';
          case 'vad_activity': return '🎯';
          case 'vad_silence': return '🔇';
          case 'toggle': return '⏺️';
          default: return '🎤';
        }
    }
  }
  
  function getDefaultAriaLabel(state, mode, error) {
    switch (state) {
      case 'listening': return 'Esperando você falar...';
      case 'recording': 
        return mode === 'record_audio' 
          ? 'Gravando áudio... solte para anexar' 
          : 'Gravando... solte para enviar';
      case 'processing': return 'Processando sua mensagem';
      case 'error': return error || 'Erro na gravação';
      default:
        switch (mode) {
          case 'record_audio': return 'Segure para gravar áudio';
          case 'ptt': return 'Segure para falar';
          case 'toggle': return 'Clique para gravar';
          case 'vad_silence': return 'Clique para gravar (para ao silêncio)';
          case 'vad_activity': return 'Clique para ativar (detecta voz automaticamente)';
          default: return 'Gravar voz';
        }
    }
  }
  
  // === Event Handlers ===
  
  function handlePointerDown(event) {
    if (disabled || isActive) return;
    
    if (event.pointerType === 'touch') {
      event.preventDefault();
    }
    
    // PTT e record_audio usam pointerdown
    if (mode === 'ptt' || mode === 'record_audio') {
      dispatch('recordStart');
    }
  }
  
  function handlePointerUp(event) {
    // PTT e record_audio usam pointerup
    if (mode === 'ptt' || mode === 'record_audio') {
      if (isRecording) {
        dispatch('recordStop');
      }
    }
  }
  
  function handlePointerLeave(event) {
    // PTT e record_audio param ao sair do botão
    if (mode === 'ptt' || mode === 'record_audio') {
      if (isRecording) {
        dispatch('recordStop');
      }
    }
  }
  
  function handlePointerCancel(event) {
    if (isActive) {
      dispatch('recordCancel');
    }
  }
  
  function handleClick(event) {
    // Toggle e VAD usam click
    if (mode === 'toggle' || mode === 'vad_silence' || mode === 'vad_activity') {
      if (isActive) {
        dispatch('recordStop');
      } else {
        dispatch('recordStart');
      }
    }
  }
  
  function handleKeyDown(event) {
    if (event.code === 'Space' && !event.repeat) {
      event.preventDefault();
      event.stopPropagation();
      
      if (mode === 'ptt' || mode === 'record_audio') {
        if (!isActive) {
          dispatch('recordStart');
        }
      } else {
        // Toggle
        if (isActive) {
          dispatch('recordStop');
        } else {
          dispatch('recordStart');
        }
      }
    }
    
    if (event.code === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      dispatch('recordCancel');
    }
  }
  
  function handleKeyUp(event) {
    if (event.code === 'Space') {
      event.preventDefault();
      event.stopPropagation();
      
      // PTT para no keyup
      if (mode === 'ptt' || mode === 'record_audio') {
        if (isRecording) {
          dispatch('recordStop');
        }
      }
    }
  }
  
  // === Classes ===
  
  $: buttonClass = [
    'voice-record-btn',
    `voice-record-btn--${state}`,
    disabled && 'voice-record-btn--disabled'
  ].filter(Boolean).join(' ');
</script>

<button
  type="button"
  class={buttonClass}
  {disabled}
  aria-label={computedAriaLabel}
  aria-busy={isBusy}
  aria-pressed={isRecording}
  title={computedAriaLabel}
  on:pointerdown={handlePointerDown}
  on:pointerup={handlePointerUp}
  on:pointerleave={handlePointerLeave}
  on:pointercancel={handlePointerCancel}
  on:click={handleClick}
  on:keydown={handleKeyDown}
  on:keyup={handleKeyUp}
>
  <span class="voice-record-btn__icon" aria-hidden="true">
    {#if isRecording}
      <span class="pulse-ring"></span>
    {/if}
    {#if isActive}
      <span 
        class="volume-indicator" 
        style="transform: scale({1 + volumePercent / 100})"
      ></span>
    {/if}
    <slot name="icon">
      {computedIcon}
    </slot>
  </span>
  
  {#if isRecording && previewText}
    <span class="voice-record-btn__preview" role="status">
      {previewText}
    </span>
  {/if}
  
  {#if isListening}
    <span class="voice-record-btn__status" role="status">
      Escutando...
    </span>
  {/if}
  
  <slot />
</button>

<style>
  .voice-record-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    min-width: 44px;
    min-height: 44px;
    padding: var(--chat-space-2, 0.5rem);
    border: none;
    border-radius: var(--chat-radius-full, 9999px);
    background: var(--chat-btn-primary-bg, var(--color-accent, #58a6ff));
    color: var(--chat-btn-primary-text, white);
    cursor: pointer;
    transition: all 0.15s ease;
    position: relative;
    overflow: visible;
    font-size: 1.25rem;
  }

  .voice-record-btn:hover:not(:disabled) {
    background: var(--chat-btn-primary-hover, var(--color-accent-hover, #79b8ff));
    transform: scale(1.02);
  }

  .voice-record-btn:focus-visible {
    outline: 2px solid var(--chat-color-border-focus, var(--color-accent, #58a6ff));
    outline-offset: 2px;
  }

  .voice-record-btn:disabled,
  .voice-record-btn--disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .voice-record-btn:active:not(:disabled) {
    transform: scale(0.98);
  }

  /* Estados */
  .voice-record-btn--idle {
    background: var(--chat-btn-primary-bg, var(--color-accent, #58a6ff));
  }

  .voice-record-btn--listening {
    background: var(--chat-color-warning, var(--color-warning, #d29922));
    animation: pulse-listening 2s infinite;
  }

  .voice-record-btn--recording {
    background: var(--chat-color-error, var(--color-error, #f85149));
    animation: pulse 1s infinite;
  }

  .voice-record-btn--processing {
    background: var(--chat-color-warning, var(--color-warning, #d29922));
  }

  .voice-record-btn--error {
    background: var(--chat-color-error, var(--color-error, #f85149));
  }

  /* Ícone */
  .voice-record-btn__icon {
    position: relative;
    z-index: 1;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  /* Indicador de volume */
  .volume-indicator {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    width: 100%;
    height: 100%;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.2);
    transition: transform 0.1s ease;
    pointer-events: none;
    z-index: 0;
  }

  /* Anel pulsante */
  .pulse-ring {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    width: 100%;
    height: 100%;
    border: 2px solid currentColor;
    border-radius: 50%;
    animation: pulse-ring 1.5s infinite;
    pointer-events: none;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.8; }
  }

  @keyframes pulse-listening {
    0%, 100% { 
      opacity: 1; 
      box-shadow: 0 0 0 0 rgba(210, 153, 34, 0.4); 
    }
    50% { 
      opacity: 0.9; 
      box-shadow: 0 0 0 8px rgba(210, 153, 34, 0); 
    }
  }

  @keyframes pulse-ring {
    0% {
      transform: translate(-50%, -50%) scale(1);
      opacity: 1;
    }
    100% {
      transform: translate(-50%, -50%) scale(2);
      opacity: 0;
    }
  }

  /* Preview do texto */
  .voice-record-btn__preview {
    position: absolute;
    bottom: calc(100% + 8px);
    left: 50%;
    transform: translateX(-50%);
    background: var(--chat-color-surface, var(--color-bg-secondary, #1e1e1e));
    color: var(--chat-color-text, var(--color-text-primary, #fff));
    padding: var(--chat-space-1, 0.25rem) var(--chat-space-2, 0.5rem);
    border-radius: var(--chat-radius-md, 0.5rem);
    font-size: var(--chat-font-size-sm, 0.875rem);
    white-space: nowrap;
    max-width: 300px;
    overflow: hidden;
    text-overflow: ellipsis;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
    z-index: 100;
  }

  .voice-record-btn__preview::after {
    content: '';
    position: absolute;
    top: 100%;
    left: 50%;
    transform: translateX(-50%);
    border: 6px solid transparent;
    border-top-color: var(--chat-color-surface, var(--color-bg-secondary, #1e1e1e));
  }

  /* Status de escutando */
  .voice-record-btn__status {
    position: absolute;
    bottom: calc(100% + 8px);
    left: 50%;
    transform: translateX(-50%);
    background: var(--chat-color-warning, var(--color-warning, #d29922));
    color: white;
    padding: var(--chat-space-1, 0.25rem) var(--chat-space-2, 0.5rem);
    border-radius: var(--chat-radius-md, 0.5rem);
    font-size: var(--chat-font-size-xs, 0.75rem);
    white-space: nowrap;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
    z-index: 100;
    animation: fade-pulse 1.5s infinite;
  }

  @keyframes fade-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.6; }
  }

  .voice-record-btn__status::after {
    content: '';
    position: absolute;
    top: 100%;
    left: 50%;
    transform: translateX(-50%);
    border: 6px solid transparent;
    border-top-color: var(--chat-color-warning, var(--color-warning, #d29922));
  }

  @media (prefers-reduced-motion: reduce) {
    .voice-record-btn--recording,
    .voice-record-btn--listening { animation: none; }
    .pulse-ring { animation: none; opacity: 0.5; }
    .voice-record-btn__status { animation: none; }
  }
</style>
