<script context="module">
  /**
   * Modos de gravação de áudio
   */
  export const RECORDING_MODES = {
    PTT: 'ptt',           // Push-to-talk: segura para gravar
    TOGGLE: 'toggle',     // Clique para iniciar/parar
    VAD_SILENCE: 'vad_silence',   // Clique + detecta silêncio para parar
    VAD_ACTIVITY: 'vad_activity'  // Full auto: detecta início e fim de fala
  };

  /**
   * Labels para modos de gravação
   */
  export const RECORDING_MODE_LABELS = {
    [RECORDING_MODES.PTT]: 'Push-to-Talk (segurar)',
    [RECORDING_MODES.TOGGLE]: 'Toggle (clique início/fim)',
    [RECORDING_MODES.VAD_SILENCE]: 'VAD Silêncio (clique + auto-stop)',
    [RECORDING_MODES.VAD_ACTIVITY]: 'VAD Atividade (full auto)'
  };

  /**
   * Ícones para modos de gravação
   */
  export const RECORDING_MODE_ICONS = {
    [RECORDING_MODES.PTT]: '🎤',
    [RECORDING_MODES.TOGGLE]: '⏺️',
    [RECORDING_MODES.VAD_SILENCE]: '🔇',
    [RECORDING_MODES.VAD_ACTIVITY]: '🎯'
  };

  /**
   * Tipos de ação do menu
   */
  export const MENU_ACTIONS = {
    FILE: 'file',
    SCREENSHOT: 'screenshot',
    WEBCAM: 'webcam',
    RECORDING_MODE: 'recording_mode'
  };
</script>

<script>
  import { createEventDispatcher, tick } from 'svelte';
  import { ContextMenu } from '../../components/contextmenu';
  import { ALL_ACCEPTED_TYPES } from '../../lib/chat/media-service.js';
  
  const dispatch = createEventDispatcher();
  
  // Props
  export let visible = false;
  export let x = 0;
  export let y = 0;
  
  /** Modo de gravação atual */
  export let recordingMode = RECORDING_MODES.PTT;
  
  /** Desabilitar opções específicas */
  export let disabledActions = [];
  
  /** Ocultar opções específicas */
  export let hiddenActions = [];
  
  // Referência ao menu
  let menuComponent;
  
  // Input de arquivo oculto
  let fileInput;
  
  // Submenu de modos de gravação
  $: recordingModeSubmenu = Object.entries(RECORDING_MODES).map(([key, value]) => ({
    id: `mode_${value}`,
    label: RECORDING_MODE_LABELS[value],
    icon: RECORDING_MODE_ICONS[value],
    // Marca o modo atual com checkmark
    shortcut: recordingMode === value ? '✓' : undefined
  }));
  
  // Items do menu
  $: menuItems = [
    { 
      id: MENU_ACTIONS.FILE, 
      label: 'Enviar arquivo', 
      icon: '📎',
      disabled: disabledActions.includes(MENU_ACTIONS.FILE),
      hidden: hiddenActions.includes(MENU_ACTIONS.FILE)
    },
    { 
      id: MENU_ACTIONS.SCREENSHOT, 
      label: 'Capturar tela', 
      icon: '📸',
      disabled: disabledActions.includes(MENU_ACTIONS.SCREENSHOT),
      hidden: hiddenActions.includes(MENU_ACTIONS.SCREENSHOT)
    },
    { 
      id: MENU_ACTIONS.WEBCAM, 
      label: 'Capturar webcam', 
      icon: '📷',
      disabled: disabledActions.includes(MENU_ACTIONS.WEBCAM),
      hidden: hiddenActions.includes(MENU_ACTIONS.WEBCAM)
    },
    { separator: true },
    { 
      id: MENU_ACTIONS.RECORDING_MODE, 
      label: 'Modo de gravação', 
      icon: '🎙️',
      disabled: disabledActions.includes(MENU_ACTIONS.RECORDING_MODE),
      hidden: hiddenActions.includes(MENU_ACTIONS.RECORDING_MODE),
      submenu: recordingModeSubmenu
    }
  ].filter(item => !item.hidden);
  
  export function open(posX, posY) {
    x = posX;
    y = posY;
    visible = true;
  }
  
  export function close() {
    visible = false;
  }
  
  function handleSelect(event) {
    const { id, item } = event.detail;
    
    // Verifica se é seleção de modo de gravação
    if (id.startsWith('mode_')) {
      const mode = id.replace('mode_', '');
      recordingMode = mode;
      dispatch('modechange', { mode });
      return;
    }
    
    switch (id) {
      case MENU_ACTIONS.FILE:
        // Abre seletor de arquivo com todos os tipos aceitos
        fileInput.accept = ALL_ACCEPTED_TYPES;
        fileInput.click();
        break;
        
      case MENU_ACTIONS.SCREENSHOT:
        captureScreen();
        break;
        
      case MENU_ACTIONS.WEBCAM:
        captureWebcam();
        break;
    }
  }
  
  function handleFileSelect(event) {
    const files = event.target.files;
    if (files && files.length > 0) {
      // Emite evento com arquivos - detecção de tipo será feita pelo consumidor
      dispatch('files', { 
        files: Array.from(files) 
      });
    }
    // Limpa o input para permitir selecionar o mesmo arquivo novamente
    event.target.value = '';
  }
  
  async function captureScreen() {
    try {
      // Usa a API de captura de tela do navegador
      const stream = await navigator.mediaDevices.getDisplayMedia({
        video: { mediaSource: 'screen' }
      });
      
      // Cria um video element para capturar o frame
      const video = document.createElement('video');
      video.srcObject = stream;
      await video.play();
      
      // Espera um frame
      await new Promise(resolve => setTimeout(resolve, 100));
      
      // Captura para canvas
      const canvas = document.createElement('canvas');
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      const ctx = canvas.getContext('2d');
      ctx.drawImage(video, 0, 0);
      
      // Para o stream
      stream.getTracks().forEach(track => track.stop());
      
      // Converte para blob
      const blob = await new Promise(resolve => 
        canvas.toBlob(resolve, 'image/png')
      );
      
      const file = new File([blob], `screenshot-${Date.now()}.png`, { 
        type: 'image/png' 
      });
      
      dispatch('files', { 
        files: [file],
        source: 'screenshot'
      });
    } catch (error) {
      console.error('Erro ao capturar tela:', error);
      dispatch('error', { 
        action: MENU_ACTIONS.SCREENSHOT, 
        error: error.message || 'Erro ao capturar tela' 
      });
    }
  }
  
  async function captureWebcam() {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ 
        video: { facingMode: 'user' } 
      });
      
      // Cria um video element
      const video = document.createElement('video');
      video.srcObject = stream;
      await video.play();
      
      // Espera estabilizar
      await new Promise(resolve => setTimeout(resolve, 500));
      
      // Captura para canvas
      const canvas = document.createElement('canvas');
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      const ctx = canvas.getContext('2d');
      ctx.drawImage(video, 0, 0);
      
      // Para o stream
      stream.getTracks().forEach(track => track.stop());
      
      // Converte para blob
      const blob = await new Promise(resolve => 
        canvas.toBlob(resolve, 'image/jpeg', 0.9)
      );
      
      const file = new File([blob], `webcam-${Date.now()}.jpg`, { 
        type: 'image/jpeg' 
      });
      
      dispatch('files', { 
        files: [file],
        source: 'webcam'
      });
    } catch (error) {
      console.error('Erro ao capturar webcam:', error);
      dispatch('error', { 
        action: MENU_ACTIONS.WEBCAM, 
        error: error.message || 'Erro ao acessar webcam' 
      });
    }
  }
  
  function handleClose() {
    visible = false;
    dispatch('close');
  }
</script>

<!-- Input oculto para seleção de arquivos -->
<input
  bind:this={fileInput}
  type="file"
  class="hidden-input"
  on:change={handleFileSelect}
  multiple
/>

<ContextMenu
  bind:this={menuComponent}
  items={menuItems}
  ariaLabel="Menu de mídia"
  bind:visible
  {x}
  {y}
  on:select={handleSelect}
  on:close={handleClose}
/>

<style>
  .hidden-input {
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
