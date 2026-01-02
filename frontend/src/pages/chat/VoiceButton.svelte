<script>
  /**
   * VoiceButton - Botão de entrada de voz (Smart Component)
   * 
   * Usa o VoiceRecordButton para UI e o sttService para lógica.
   * Este componente conecta a UI com o serviço de STT.
   * 
   * Eventos:
   *   - transcript: Transcrição finalizada { text }
   *   - audiofile: Arquivo de áudio gravado { file, blob }
   */
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { sttService, STT_PROVIDERS, RECORDING_MODES, STT_STATES } from '../../lib/speech/index.js';
  import { VoiceRecordButton } from '../../components/chat';

  const dispatch = createEventDispatcher();

  // Re-exporta constantes para compatibilidade
  export { RECORDING_MODES };

  // Props
  export let disabled = false;
  export let language = 'pt-BR';
  export let sttProvider = STT_PROVIDERS.WEBSPEECH;
  export let mode = RECORDING_MODES.PTT;
  export let silenceDuration = 1500;

  // Estado local (sincronizado com sttService)
  let state = STT_STATES.IDLE;
  let interimText = '';
  let currentVolume = 0;
  let errorMessage = '';
  
  // Acessibilidade
  let liveMessage = '';
  
  // Audio feedback
  let audioContext = null;

  // Sincroniza configurações com o serviço
  $: {
    sttService.setProvider(sttProvider);
    sttService.setMode(mode);
    sttService.setLanguage(language);
    sttService.setSilenceDuration(silenceDuration);
  }

  onMount(() => {
    // Registra listeners do sttService
    sttService.addEventListener('stateChange', handleStateChange);
    sttService.addEventListener('interimResult', handleInterimResult);
    sttService.addEventListener('transcription', handleTranscription);
    sttService.addEventListener('audioFile', handleAudioFile);
    sttService.addEventListener('volumeChange', handleVolumeChange);
    sttService.addEventListener('error', handleError);
    
    // Inicializa o serviço
    sttService.ready();
  });

  onDestroy(() => {
    // Remove listeners
    sttService.removeEventListener('stateChange', handleStateChange);
    sttService.removeEventListener('interimResult', handleInterimResult);
    sttService.removeEventListener('transcription', handleTranscription);
    sttService.removeEventListener('audioFile', handleAudioFile);
    sttService.removeEventListener('volumeChange', handleVolumeChange);
    sttService.removeEventListener('error', handleError);
  });

  // === Event Handlers do sttService ===
  
  function handleStateChange(event) {
    const { state: newState, message } = event.detail;
    const previousState = state;
    state = newState;
    
    if (message) {
      liveMessage = message;
    }
    
    // Feedback de áudio nas transições
    if (newState === STT_STATES.RECORDING && previousState !== STT_STATES.RECORDING) {
      playSound('start');
    } else if (newState === STT_STATES.LISTENING && previousState !== STT_STATES.LISTENING) {
      playSound('listening');
    } else if (newState === STT_STATES.IDLE && previousState === STT_STATES.RECORDING) {
      playSound('end');
    } else if (newState === STT_STATES.ERROR) {
      playSound('error');
    }
  }
  
  function handleInterimResult(event) {
    interimText = event.detail.text;
  }
  
  function handleTranscription(event) {
    const { text, isFinal } = event.detail;
    if (isFinal && text.trim()) {
      liveMessage = `Transcrição: ${text}`;
      dispatch('transcript', { text });
    }
    interimText = '';
  }
  
  function handleAudioFile(event) {
    const { file, blob } = event.detail;
    dispatch('audiofile', { file, blob });
  }
  
  function handleVolumeChange(event) {
    currentVolume = event.detail.volume;
  }
  
  function handleError(event) {
    errorMessage = event.detail.message;
    liveMessage = errorMessage;
  }

  // === Handlers do VoiceRecordButton ===
  
  function handleRecordStart() {
    if (disabled) return;
    sttService.startRecording();
  }
  
  function handleRecordStop() {
    sttService.stopRecording();
  }
  
  function handleRecordCancel() {
    sttService.cancel();
    liveMessage = 'Gravação cancelada.';
  }

  // === Métodos públicos ===

  export async function startRecording() {
    if (disabled || state === STT_STATES.RECORDING || state === STT_STATES.LISTENING) return;
    await sttService.startRecording();
  }

  export function stopRecording() {
    sttService.stopRecording();
  }

  export function cancelRecording() {
    sttService.cancel();
    liveMessage = 'Gravação cancelada.';
  }

  export function getState() {
    return state;
  }

  export function setIdle() {
    if (state === STT_STATES.PROCESSING) {
      state = STT_STATES.IDLE;
    }
  }

  // === Audio Feedback ===
  
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
      
      switch (type) {
        case 'start':
          oscillator.frequency.setValueAtTime(440, ctx.currentTime);
          oscillator.frequency.setValueAtTime(880, ctx.currentTime + 0.1);
          gainNode.gain.setValueAtTime(0.2, ctx.currentTime);
          gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.15);
          oscillator.start(ctx.currentTime);
          oscillator.stop(ctx.currentTime + 0.15);
          break;
          
        case 'end':
          oscillator.frequency.setValueAtTime(660, ctx.currentTime);
          oscillator.frequency.linearRampToValueAtTime(440, ctx.currentTime + 0.1);
          gainNode.gain.setValueAtTime(0.2, ctx.currentTime);
          gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.15);
          oscillator.start(ctx.currentTime);
          oscillator.stop(ctx.currentTime + 0.15);
          break;
          
        case 'error':
          oscillator.frequency.setValueAtTime(200, ctx.currentTime);
          gainNode.gain.setValueAtTime(0.3, ctx.currentTime);
          gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.3);
          oscillator.start(ctx.currentTime);
          oscillator.stop(ctx.currentTime + 0.3);
          break;
          
        case 'listening':
          oscillator.frequency.setValueAtTime(330, ctx.currentTime);
          oscillator.frequency.setValueAtTime(440, ctx.currentTime + 0.05);
          oscillator.frequency.setValueAtTime(550, ctx.currentTime + 0.1);
          gainNode.gain.setValueAtTime(0.15, ctx.currentTime);
          gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.2);
          oscillator.start(ctx.currentTime);
          oscillator.stop(ctx.currentTime + 0.2);
          break;
      }
    } catch (e) {
      // Ignora erros de áudio
    }
  }

  // Suporte
  $: isSupported = sttService.isSupported;
</script>

{#if isSupported}
  <VoiceRecordButton
    {disabled}
    {state}
    {mode}
    volume={currentVolume}
    previewText={interimText}
    {errorMessage}
    on:recordStart={handleRecordStart}
    on:recordStop={handleRecordStop}
    on:recordCancel={handleRecordCancel}
  />
{:else}
  <button
    type="button"
    class="voice-btn voice-btn--unsupported"
    disabled
    aria-label="Reconhecimento de voz não suportado neste navegador"
    title="Reconhecimento de voz não suportado"
  >
    <span aria-hidden="true">🎤</span>
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
  .voice-btn--unsupported {
    display: flex;
    align-items: center;
    justify-content: center;
    min-width: 44px;
    min-height: 44px;
    padding: var(--spacing-sm);
    border: none;
    border-radius: var(--border-radius);
    background: var(--color-text-muted, #6e7681);
    color: white;
    cursor: not-allowed;
    opacity: 0.5;
    font-size: 1.25rem;
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
