<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { SpeechRecognitionManager, SpeechSynthesisManager, AudioRecorder } from '../lib/speech/index.js';
  import { VoiceActivityDetector } from '../lib/speech/vad.js';
  import { TranscribeWhisper } from '../../wailsjs/go/main/App.js';

  const dispatch = createEventDispatcher();

  // Constantes de provedores STT
  const STT_WEBSPEECH = 'webspeech';
  const STT_WHISPER = 'whisper';

  // Modos de gravação
  export const RECORDING_MODES = {
    PTT: 'ptt',           // Push-to-talk: segura para gravar
    TOGGLE: 'toggle',     // Clique para iniciar/parar
    VAD_SILENCE: 'vad_silence',   // Clique + detecta silêncio para parar
    VAD_ACTIVITY: 'vad_activity'  // Full auto: detecta início e fim de fala
  };

  // Props
  export let disabled = false;
  export let language = 'pt-BR';
  export let autoSpeak = true;
  export let sttProvider = STT_WEBSPEECH;
  
  /**
   * Modo de operação:
   * - 'ptt': Push-to-Talk - grava voz enquanto segura botão
   * - 'toggle': Clique para iniciar, clique para parar
   * - 'vad_silence': Clique para iniciar, detecta silêncio para parar
   * - 'vad_activity': Full auto - detecta início e fim de fala
   * - 'record_audio': Grava áudio como arquivo (não transcreve)
   */
  export let mode = 'ptt';
  
  /**
   * Duração de silêncio para parar gravação (ms)
   */
  export let silenceDuration = 1500;

  // Estado
  let state = 'idle'; // 'idle' | 'listening' | 'recording' | 'processing' | 'speaking' | 'error'
  let errorMessage = '';
  let interimText = '';
  let currentVolume = 0; // 0-1 para visualização
  
  // Managers
  let sttManager = null;
  let ttsManager = null;
  let audioRecorder = null;
  let vadDetector = null;
  
  // Audio feedback
  let audioContext = null;
  
  // Acessibilidade
  let liveMessage = '';
  
  // Referência do botão
  let buttonElement;
  
  // Verifica suporte
  $: isSTTSupported = SpeechRecognitionManager.isSupported() || sttProvider === STT_WHISPER;
  $: isTTSSupported = SpeechSynthesisManager.isSupported();
  $: isSupported = isSTTSupported;

  onMount(() => {
    initSTT();
    initTTS();
    initAudioRecorder();
  });

  onDestroy(() => {
    cleanup();
  });

  function cleanup() {
    if (sttManager) sttManager.abort();
    if (ttsManager) ttsManager.stop();
    if (audioRecorder) audioRecorder.stop();
    if (vadDetector) vadDetector.destroy();
  }

  async function initAudioRecorder() {
    audioRecorder = new AudioRecorder({
      mimeType: 'audio/webm',
      onStart: () => {
        console.log('AudioRecorder started');
      },
      onStop: async (blob) => {
        if (blob.size > 0) {
          if (mode === 'record_audio') {
            // Modo gravar áudio: dispara evento com o arquivo
            const file = new File([blob], `audio-${Date.now()}.webm`, { type: 'audio/webm' });
            dispatch('audiofile', { file, blob });
            state = 'idle';
          } else if (sttProvider === STT_WHISPER) {
            // Modo PTT com Whisper: transcreve
            await transcribeWithWhisper(blob);
          }
        }
      },
      onError: (error) => {
        console.error('AudioRecorder error:', error);
        state = 'error';
        errorMessage = 'Erro ao gravar áudio';
        liveMessage = errorMessage;
      }
    });
    
    await audioRecorder.init();
  }

  async function initVAD() {
    if (vadDetector) {
      vadDetector.destroy();
    }

    vadDetector = new VoiceActivityDetector({
      silenceDuration: silenceDuration,
      silenceThreshold: 0.01,
      activityThreshold: 0.02,
      activityDuration: 200,
      
      onVolumeChange: (volume) => {
        currentVolume = volume;
      },
      
      onActivityStart: () => {
        console.log('[VAD] Atividade de voz detectada');
        
        if (mode === RECORDING_MODES.VAD_ACTIVITY && state === 'listening') {
          // Modo full auto: inicia gravação quando detecta voz
          startActualRecording();
        }
      },
      
      onActivityEnd: () => {
        console.log('[VAD] Fim de atividade de voz');
        
        if ((mode === RECORDING_MODES.VAD_SILENCE || mode === RECORDING_MODES.VAD_ACTIVITY) && state === 'recording') {
          // Para a gravação quando detecta silêncio prolongado
          stopRecording();
        }
      },
      
      onSilenceStart: () => {
        console.log('[VAD] Silêncio detectado');
      },
      
      onSilenceEnd: () => {
        console.log('[VAD] Silêncio terminado');
      }
    });
  }

  async function transcribeWithWhisper(audioBlob) {
    try {
      state = 'processing';
      liveMessage = 'Transcrevendo com Whisper...';
      
      const reader = new FileReader();
      const base64Promise = new Promise((resolve, reject) => {
        reader.onloadend = () => {
          const base64 = reader.result.split(',')[1];
          resolve(base64);
        };
        reader.onerror = reject;
      });
      reader.readAsDataURL(audioBlob);
      
      const audioBase64 = await base64Promise;
      const result = await TranscribeWhisper(audioBase64, 'audio.webm');
      
      if (result && result.text && result.text.trim()) {
        liveMessage = `Transcrição: ${result.text}`;
        dispatch('transcript', { text: result.text });
      } else {
        liveMessage = 'Nenhuma fala detectada.';
      }
      
      state = 'idle';
    } catch (error) {
      console.error('Whisper transcription error:', error);
      state = 'error';
      errorMessage = 'Erro na transcrição Whisper';
      liveMessage = errorMessage;
      
      setTimeout(() => {
        if (state === 'error') {
          state = 'idle';
          errorMessage = '';
        }
      }, 3000);
    }
  }

  function initSTT() {
    if (!SpeechRecognitionManager.isSupported()) {
      console.warn('SpeechRecognition não suportado');
      return;
    }

    sttManager = new SpeechRecognitionManager({
      language,
      continuous: false,
      interimResults: true,
      
      onStart: () => {
        state = 'recording';
        interimText = '';
        liveMessage = 'Gravando. Fale agora.';
        playSound('start');
      },
      
      onEnd: (transcript) => {
        // Para o VAD
        if (vadDetector && vadDetector.active) {
          vadDetector.stop();
        }
        
        if (transcript && transcript.trim()) {
          state = 'processing';
          liveMessage = `Enviando: ${transcript}`;
          dispatch('transcript', { text: transcript });
        } else {
          state = 'idle';
          liveMessage = 'Nenhuma fala detectada.';
        }
        interimText = '';
      },
      
      onResult: (transcript) => {
        interimText = transcript;
      },
      
      onInterim: (text) => {
        interimText = text;
      },
      
      onError: (message, errorType) => {
        state = 'error';
        errorMessage = message;
        liveMessage = message;
        playSound('error');
        
        // Para o VAD
        if (vadDetector && vadDetector.active) {
          vadDetector.stop();
        }
        
        setTimeout(() => {
          if (state === 'error') {
            state = 'idle';
            errorMessage = '';
          }
        }, 3000);
      }
    });
  }

  function initTTS() {
    if (!isTTSSupported) {
      console.warn('SpeechSynthesis não suportado');
      return;
    }

    ttsManager = new SpeechSynthesisManager({
      language,
      rate: 1.0,
      pitch: 1.0,
      volume: 1.0,
      
      onStart: () => {
        state = 'speaking';
        liveMessage = 'Reproduzindo resposta.';
      },
      
      onEnd: () => {
        state = 'idle';
        liveMessage = 'Resposta finalizada.';
        playSound('end');
      },
      
      onError: (error) => {
        console.error('TTS error:', error);
        state = 'idle';
      }
    });
  }

  function getAudioContext() {
    if (!audioContext) {
      audioContext = new (window.AudioContext || window.webkitAudioContext)();
    }
    return audioContext;
  }

  function playSound(type) {
    try {
      const ctx = getAudioContext();
      const oscillator = ctx.createOscillator();
      const gainNode = ctx.createGain();
      
      oscillator.connect(gainNode);
      gainNode.connect(ctx.destination);
      
      if (type === 'start') {
        oscillator.frequency.setValueAtTime(440, ctx.currentTime);
        oscillator.frequency.setValueAtTime(880, ctx.currentTime + 0.1);
        gainNode.gain.setValueAtTime(0.2, ctx.currentTime);
        gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.15);
        oscillator.start(ctx.currentTime);
        oscillator.stop(ctx.currentTime + 0.15);
      } else if (type === 'end') {
        oscillator.frequency.setValueAtTime(660, ctx.currentTime);
        oscillator.frequency.linearRampToValueAtTime(440, ctx.currentTime + 0.1);
        gainNode.gain.setValueAtTime(0.2, ctx.currentTime);
        gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.15);
        oscillator.start(ctx.currentTime);
        oscillator.stop(ctx.currentTime + 0.15);
      } else if (type === 'error') {
        oscillator.frequency.setValueAtTime(200, ctx.currentTime);
        gainNode.gain.setValueAtTime(0.3, ctx.currentTime);
        gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.3);
        oscillator.start(ctx.currentTime);
        oscillator.stop(ctx.currentTime + 0.3);
      } else if (type === 'listening') {
        // Som suave de "esperando"
        oscillator.frequency.setValueAtTime(330, ctx.currentTime);
        oscillator.frequency.setValueAtTime(440, ctx.currentTime + 0.05);
        oscillator.frequency.setValueAtTime(550, ctx.currentTime + 0.1);
        gainNode.gain.setValueAtTime(0.15, ctx.currentTime);
        gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.2);
        oscillator.start(ctx.currentTime);
        oscillator.stop(ctx.currentTime + 0.2);
      }
    } catch (e) {
      // Ignora erros de áudio
    }
  }

  /**
   * Inicia o processo de gravação baseado no modo
   */
  export async function startRecording() {
    if (disabled || state === 'recording' || state === 'listening') return;
    
    // Para TTS se estiver falando
    if (ttsManager && state === 'speaking') {
      ttsManager.stop();
    }
    
    switch (mode) {
      case RECORDING_MODES.PTT:
      case 'record_audio':
        // PTT ou record_audio: inicia gravação imediatamente
        startActualRecording();
        break;
        
      case RECORDING_MODES.TOGGLE:
        // Toggle: inicia gravação imediatamente
        startActualRecording();
        break;
        
      case RECORDING_MODES.VAD_SILENCE:
        // VAD Silence: inicia gravação imediatamente + ativa VAD para detectar fim
        await startWithVAD(false);
        break;
        
      case RECORDING_MODES.VAD_ACTIVITY:
        // VAD Activity: entra em modo "escutando" e espera atividade de voz
        await startListening();
        break;
    }
  }

  /**
   * Inicia a gravação real (sem esperar atividade)
   */
  function startActualRecording() {
    if (mode === 'record_audio' || sttProvider === STT_WHISPER) {
      // Usa AudioRecorder
      if (audioRecorder) {
        state = 'recording';
        interimText = '';
        liveMessage = mode === 'record_audio' 
          ? 'Gravando áudio. Solte para anexar.' 
          : 'Gravando para Whisper. Fale agora.';
        playSound('start');
        audioRecorder.start();
      }
    } else {
      // Usa WebSpeech
      if (sttManager) {
        sttManager.start();
      }
    }
  }

  /**
   * Inicia com VAD ativo
   */
  async function startWithVAD(waitForActivity) {
    try {
      // Inicializa VAD
      await initVAD();
      
      // Obtém stream para VAD
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      await vadDetector.init(stream);
      vadDetector.start();
      
      if (waitForActivity) {
        // Modo VAD Activity: entra em "listening" e espera atividade
        state = 'listening';
        liveMessage = 'Esperando você falar...';
        playSound('listening');
      } else {
        // Modo VAD Silence: inicia gravação imediata
        startActualRecording();
      }
    } catch (error) {
      console.error('[VAD] Erro ao inicializar:', error);
      // Fallback para gravação normal
      startActualRecording();
    }
  }

  /**
   * Inicia modo "escutando" para VAD Activity
   */
  async function startListening() {
    try {
      await initVAD();
      
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      await vadDetector.init(stream);
      vadDetector.start();
      
      state = 'listening';
      liveMessage = 'Esperando você falar...';
      playSound('listening');
      
      // Inicia o audioRecorder mas pausa
      if (sttProvider === STT_WHISPER) {
        await audioRecorder.init();
      }
    } catch (error) {
      console.error('[VAD] Erro ao iniciar listening:', error);
      state = 'error';
      errorMessage = 'Erro ao acessar microfone';
      liveMessage = errorMessage;
    }
  }

  /**
   * Para a gravação
   */
  function stopRecording() {
    if (state !== 'recording' && state !== 'listening') return;
    
    playSound('end');
    
    // Para o VAD
    if (vadDetector && vadDetector.active) {
      vadDetector.stop();
    }
    
    if (state === 'listening') {
      // Estava só escutando, não iniciou gravação
      state = 'idle';
      liveMessage = 'Gravação cancelada.';
      return;
    }
    
    if (mode === 'record_audio' || sttProvider === STT_WHISPER) {
      if (audioRecorder) {
        audioRecorder.stop();
      }
    } else {
      if (sttManager) {
        sttManager.stop();
      }
    }
  }

  /**
   * Cancela a gravação
   */
  function cancelRecording() {
    if (sttManager) {
      sttManager.abort();
    }
    if (audioRecorder) {
      audioRecorder.stop();
    }
    if (vadDetector && vadDetector.active) {
      vadDetector.stop();
    }
    
    state = 'idle';
    interimText = '';
    liveMessage = 'Gravação cancelada.';
  }

  /**
   * Toggle de gravação (para modos Toggle e VAD)
   */
  function toggleRecording() {
    if (state === 'recording' || state === 'listening') {
      stopRecording();
    } else {
      startRecording();
    }
  }

  // === Mouse/Touch handlers ===
  
  function handlePointerDown(event) {
    if (disabled || state === 'recording' || state === 'listening') return;
    
    if (event.pointerType === 'touch') {
      event.preventDefault();
    }
    
    // Só PTT usa pointerdown
    if (mode === RECORDING_MODES.PTT || mode === 'record_audio') {
      startRecording();
    }
  }
  
  function handlePointerUp(event) {
    // Só PTT usa pointerup
    if (mode === RECORDING_MODES.PTT || mode === 'record_audio') {
      if (state === 'recording') {
        stopRecording();
      }
    }
  }
  
  function handlePointerLeave(event) {
    // Só PTT usa pointerleave
    if (mode === RECORDING_MODES.PTT || mode === 'record_audio') {
      if (state === 'recording') {
        stopRecording();
      }
    }
  }
  
  function handlePointerCancel(event) {
    if (state === 'recording' || state === 'listening') {
      cancelRecording();
    }
  }

  function handleClick(event) {
    // Toggle, VAD Silence e VAD Activity usam click
    if (mode === RECORDING_MODES.TOGGLE || 
        mode === RECORDING_MODES.VAD_SILENCE || 
        mode === RECORDING_MODES.VAD_ACTIVITY) {
      toggleRecording();
    }
  }

  // === Keyboard handlers ===
  
  function handleKeyDown(event) {
    if (event.code === 'Space' && !event.repeat) {
      event.preventDefault();
      event.stopPropagation();
      
      if (mode === RECORDING_MODES.PTT || mode === 'record_audio') {
        startRecording();
      } else {
        toggleRecording();
      }
    }
    if (event.code === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      cancelRecording();
    }
  }

  function handleKeyUp(event) {
    if (event.code === 'Space') {
      event.preventDefault();
      event.stopPropagation();
      
      // Só PTT para no keyup
      if (mode === RECORDING_MODES.PTT || mode === 'record_audio') {
        stopRecording();
      }
    }
  }

  // === Métodos públicos ===

  export function speak(text) {
    if (!autoSpeak || !ttsManager || !text) return;
    ttsManager.speak(text);
  }

  export function stopSpeaking() {
    if (ttsManager) ttsManager.stop();
  }

  export function getState() {
    return state;
  }

  export function setIdle() {
    if (state === 'processing') {
      state = 'idle';
    }
  }

  // Ícone e label baseado no modo e estado
  $: icon = state === 'listening'
    ? '👂'
    : state === 'recording' 
      ? '🔴' 
      : state === 'processing' 
        ? '⏳' 
        : state === 'speaking' 
          ? '🔊' 
          : state === 'error' 
            ? '❌' 
            : mode === 'record_audio' 
              ? '🎙️' 
              : mode === RECORDING_MODES.VAD_ACTIVITY
                ? '🎯'
                : mode === RECORDING_MODES.VAD_SILENCE
                  ? '🔇'
                  : mode === RECORDING_MODES.TOGGLE
                    ? '⏺️'
                    : '🎤';

  $: ariaLabel = state === 'listening'
    ? 'Esperando você falar...'
    : state === 'recording' 
      ? (mode === 'record_audio' ? 'Gravando áudio... solte para anexar' : 'Gravando... solte para enviar')
      : state === 'processing' 
        ? 'Processando sua mensagem' 
        : state === 'speaking' 
          ? 'Reproduzindo resposta' 
          : state === 'error' 
            ? (errorMessage || 'Erro')
            : mode === 'record_audio'
              ? 'Segure para gravar áudio'
              : mode === RECORDING_MODES.PTT
                ? 'Segure para falar'
                : mode === RECORDING_MODES.TOGGLE
                  ? 'Clique para gravar'
                  : mode === RECORDING_MODES.VAD_SILENCE
                    ? 'Clique para gravar (para ao silêncio)'
                    : mode === RECORDING_MODES.VAD_ACTIVITY
                      ? 'Clique para ativar (detecta voz automaticamente)'
                      : 'Segure para falar';

  $: buttonClass = `voice-btn voice-btn--${state}`;
  
  // Indicador visual de volume
  $: volumePercent = Math.min(100, currentVolume * 500);
</script>

{#if isSupported}
  <button
    bind:this={buttonElement}
    class={buttonClass}
    {disabled}
    on:pointerdown={handlePointerDown}
    on:pointerup={handlePointerUp}
    on:pointerleave={handlePointerLeave}
    on:pointercancel={handlePointerCancel}
    on:click={handleClick}
    on:keydown={handleKeyDown}
    on:keyup={handleKeyUp}
    aria-label={ariaLabel}
    aria-busy={state === 'recording' || state === 'processing' || state === 'listening'}
    title={ariaLabel}
  >
    <span class="voice-btn__icon" aria-hidden="true">
      {#if state === 'recording'}
        <span class="pulse-ring"></span>
      {/if}
      {#if state === 'listening' || state === 'recording'}
        <span class="volume-indicator" style="transform: scale({1 + volumePercent / 100})"></span>
      {/if}
      {icon}
    </span>
    
    {#if state === 'recording' && interimText}
      <span class="voice-btn__preview">{interimText}</span>
    {/if}
    
    {#if state === 'listening'}
      <span class="voice-btn__status">Escutando...</span>
    {/if}
  </button>
{:else}
  <button
    class="voice-btn voice-btn--unsupported"
    disabled
    aria-label="Reconhecimento de voz não suportado neste navegador"
    title="Reconhecimento de voz não suportado"
  >
    <span class="voice-btn__icon" aria-hidden="true">🎤</span>
  </button>
{/if}

<!-- Live region para acessibilidade -->
<div 
  class="visually-hidden"
  role="status"
  aria-live="polite"
  aria-atomic="true"
>{liveMessage}</div>

<style>
  .voice-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    min-width: 44px;
    min-height: 44px;
    padding: var(--spacing-sm);
    border: none;
    border-radius: var(--border-radius);
    background: var(--color-accent, #58a6ff);
    color: white;
    cursor: pointer;
    transition: all 0.15s ease;
    position: relative;
    overflow: visible;
  }

  .voice-btn:hover:not(:disabled) {
    background: var(--color-accent-hover, #79b8ff);
    transform: scale(1.02);
  }

  .voice-btn:focus {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
  }

  .voice-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .voice-btn:active:not(:disabled) {
    transform: scale(0.98);
  }

  /* Estados */
  .voice-btn--idle {
    background: var(--color-accent, #58a6ff);
  }

  .voice-btn--listening {
    background: var(--color-warning, #d29922);
    animation: pulse-listening 2s infinite;
  }

  .voice-btn--recording {
    background: var(--color-error, #f85149);
    animation: pulse 1s infinite;
  }

  .voice-btn--processing {
    background: var(--color-warning, #d29922);
  }

  .voice-btn--speaking {
    background: var(--color-success, #3fb950);
  }

  .voice-btn--error {
    background: var(--color-error, #f85149);
  }

  .voice-btn--unsupported {
    background: var(--color-text-muted, #6e7681);
  }

  /* Ícone */
  .voice-btn__icon {
    font-size: 1.25rem;
    position: relative;
    z-index: 1;
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

  /* Anel pulsante durante gravação */
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
    0%, 100% { opacity: 1; box-shadow: 0 0 0 0 rgba(210, 153, 34, 0.4); }
    50% { opacity: 0.9; box-shadow: 0 0 0 8px rgba(210, 153, 34, 0); }
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

  /* Preview do texto sendo reconhecido */
  .voice-btn__preview {
    position: absolute;
    bottom: calc(100% + 8px);
    left: 50%;
    transform: translateX(-50%);
    background: var(--color-bg-secondary, #1e1e1e);
    color: var(--color-text-primary, #fff);
    padding: var(--spacing-xs) var(--spacing-sm);
    border-radius: var(--border-radius);
    font-size: var(--font-size-sm);
    white-space: nowrap;
    max-width: 300px;
    overflow: hidden;
    text-overflow: ellipsis;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
    z-index: 100;
  }

  .voice-btn__preview::after {
    content: '';
    position: absolute;
    top: 100%;
    left: 50%;
    transform: translateX(-50%);
    border: 6px solid transparent;
    border-top-color: var(--color-bg-secondary, #1e1e1e);
  }

  /* Status de escutando */
  .voice-btn__status {
    position: absolute;
    bottom: calc(100% + 8px);
    left: 50%;
    transform: translateX(-50%);
    background: var(--color-warning, #d29922);
    color: white;
    padding: var(--spacing-xs) var(--spacing-sm);
    border-radius: var(--border-radius);
    font-size: var(--font-size-xs);
    white-space: nowrap;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
    z-index: 100;
    animation: fade-pulse 1.5s infinite;
  }

  @keyframes fade-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.6; }
  }

  .voice-btn__status::after {
    content: '';
    position: absolute;
    top: 100%;
    left: 50%;
    transform: translateX(-50%);
    border: 6px solid transparent;
    border-top-color: var(--color-warning, #d29922);
  }

  @media (prefers-reduced-motion: reduce) {
    .voice-btn--recording { animation: none; }
    .voice-btn--listening { animation: none; }
    .pulse-ring { animation: none; opacity: 0.5; }
    .voice-btn__status { animation: none; }
  }
  
  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>
