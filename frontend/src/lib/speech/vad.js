/**
 * Voice Activity Detection (VAD)
 * 
 * Detecta início e fim de atividade de voz usando análise de volume do áudio.
 */

/**
 * Configurações do VAD
 * @typedef {Object} VADConfig
 * @property {number} silenceThreshold - Limite de volume para considerar silêncio (0-1, default: 0.01)
 * @property {number} silenceDuration - Duração de silêncio para considerar fim de fala (ms, default: 1500)
 * @property {number} activityThreshold - Limite de volume para considerar atividade (0-1, default: 0.02)
 * @property {number} activityDuration - Duração mínima de atividade para considerar início de fala (ms, default: 200)
 * @property {number} checkInterval - Intervalo de verificação (ms, default: 100)
 * @property {function} onSilenceStart - Callback quando silêncio é detectado
 * @property {function} onSilenceEnd - Callback quando silêncio termina
 * @property {function} onActivityStart - Callback quando atividade de voz é detectada
 * @property {function} onActivityEnd - Callback quando atividade de voz termina
 */

export class VoiceActivityDetector {
  constructor(config = {}) {
    this.config = {
      silenceThreshold: config.silenceThreshold ?? 0.01,
      silenceDuration: config.silenceDuration ?? 1500,
      activityThreshold: config.activityThreshold ?? 0.02,
      activityDuration: config.activityDuration ?? 200,
      checkInterval: config.checkInterval ?? 100,
      onSilenceStart: config.onSilenceStart || (() => {}),
      onSilenceEnd: config.onSilenceEnd || (() => {}),
      onActivityStart: config.onActivityStart || (() => {}),
      onActivityEnd: config.onActivityEnd || (() => {}),
      onVolumeChange: config.onVolumeChange || (() => {}),
    };

    this.audioContext = null;
    this.analyser = null;
    this.mediaStream = null;
    this.sourceNode = null;
    this.checkIntervalId = null;
    
    this.isActive = false; // VAD está rodando
    this.isSpeaking = false; // Usuário está falando
    this.silenceStartTime = null; // Quando o silêncio começou
    this.activityStartTime = null; // Quando a atividade começou
    
    // Buffer para análise
    this.dataArray = null;
  }

  /**
   * Inicializa o VAD com um MediaStream
   * @param {MediaStream} stream - Stream de áudio do microfone
   */
  async init(stream = null) {
    try {
      // Obtém o stream do microfone se não fornecido
      if (!stream) {
        stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      }
      this.mediaStream = stream;

      // Cria contexto de áudio
      this.audioContext = new (window.AudioContext || window.webkitAudioContext)();
      
      // Cria analisador
      this.analyser = this.audioContext.createAnalyser();
      this.analyser.fftSize = 256;
      this.analyser.smoothingTimeConstant = 0.3;
      
      // Conecta o stream ao analisador
      this.sourceNode = this.audioContext.createMediaStreamSource(stream);
      this.sourceNode.connect(this.analyser);
      
      // Buffer para dados de frequência
      this.dataArray = new Uint8Array(this.analyser.frequencyBinCount);
      
      console.log('[VAD] Inicializado com sucesso');
      return true;
    } catch (error) {
      console.error('[VAD] Erro ao inicializar:', error);
      throw error;
    }
  }

  /**
   * Inicia a detecção de atividade de voz
   */
  start() {
    if (this.isActive) return;
    if (!this.analyser) {
      console.error('[VAD] Não inicializado. Chame init() primeiro.');
      return;
    }

    this.isActive = true;
    this.isSpeaking = false;
    this.silenceStartTime = null;
    this.activityStartTime = null;

    // Inicia loop de verificação
    this.checkIntervalId = setInterval(() => this.checkVolume(), this.config.checkInterval);
    
    console.log('[VAD] Detecção iniciada');
  }

  /**
   * Para a detecção de atividade de voz
   */
  stop() {
    if (!this.isActive) return;

    this.isActive = false;
    
    if (this.checkIntervalId) {
      clearInterval(this.checkIntervalId);
      this.checkIntervalId = null;
    }

    // Notifica fim de atividade se estava falando
    if (this.isSpeaking) {
      this.isSpeaking = false;
      this.config.onActivityEnd();
    }

    console.log('[VAD] Detecção parada');
  }

  /**
   * Libera recursos
   */
  destroy() {
    this.stop();
    
    if (this.sourceNode) {
      this.sourceNode.disconnect();
      this.sourceNode = null;
    }
    
    if (this.audioContext) {
      this.audioContext.close();
      this.audioContext = null;
    }
    
    if (this.mediaStream) {
      this.mediaStream.getTracks().forEach(track => track.stop());
      this.mediaStream = null;
    }
    
    this.analyser = null;
    this.dataArray = null;
    
    console.log('[VAD] Recursos liberados');
  }

  /**
   * Verifica o volume atual e detecta atividade/silêncio
   */
  checkVolume() {
    if (!this.analyser || !this.dataArray) return;

    // Obtém dados de frequência
    this.analyser.getByteFrequencyData(this.dataArray);

    // Calcula volume médio normalizado (0-1)
    let sum = 0;
    for (let i = 0; i < this.dataArray.length; i++) {
      sum += this.dataArray[i];
    }
    const average = sum / this.dataArray.length;
    const volume = average / 255;

    // Notifica mudança de volume
    this.config.onVolumeChange(volume);

    const now = Date.now();

    if (volume > this.config.activityThreshold) {
      // Há atividade de voz
      
      if (!this.isSpeaking) {
        // Ainda não estava falando, inicia contagem
        if (!this.activityStartTime) {
          this.activityStartTime = now;
        } else if (now - this.activityStartTime >= this.config.activityDuration) {
          // Atividade sustentada por tempo suficiente
          this.isSpeaking = true;
          this.silenceStartTime = null;
          this.activityStartTime = null;
          this.config.onActivityStart();
        }
      } else {
        // Já estava falando, reseta contagem de silêncio
        if (this.silenceStartTime) {
          this.silenceStartTime = null;
          this.config.onSilenceEnd();
        }
      }
    } else {
      // Há silêncio
      this.activityStartTime = null;

      if (this.isSpeaking) {
        // Estava falando, inicia contagem de silêncio
        if (!this.silenceStartTime) {
          this.silenceStartTime = now;
          this.config.onSilenceStart();
        } else if (now - this.silenceStartTime >= this.config.silenceDuration) {
          // Silêncio sustentado por tempo suficiente
          this.isSpeaking = false;
          this.silenceStartTime = null;
          this.config.onActivityEnd();
        }
      }
    }
  }

  /**
   * Retorna o volume atual (0-1)
   */
  getCurrentVolume() {
    if (!this.analyser || !this.dataArray) return 0;

    this.analyser.getByteFrequencyData(this.dataArray);
    
    let sum = 0;
    for (let i = 0; i < this.dataArray.length; i++) {
      sum += this.dataArray[i];
    }
    
    return (sum / this.dataArray.length) / 255;
  }

  /**
   * Verifica se está falando
   */
  get speaking() {
    return this.isSpeaking;
  }

  /**
   * Verifica se está ativo
   */
  get active() {
    return this.isActive;
  }
}

export default VoiceActivityDetector;





