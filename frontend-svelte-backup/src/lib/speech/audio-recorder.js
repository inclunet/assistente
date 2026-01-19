/**
 * Audio Recorder - Grava áudio usando MediaRecorder API
 * Usado para enviar áudio para APIs como Whisper
 */

export class AudioRecorder {
  constructor(options = {}) {
    this.mimeType = options.mimeType || this._getPreferredMimeType();
    this.audioBitsPerSecond = options.audioBitsPerSecond || 128000;
    
    this.mediaRecorder = null;
    this.stream = null;
    this.chunks = [];
    this.isRecording = false;
    
    // Callbacks
    this.onStart = options.onStart || (() => {});
    this.onStop = options.onStop || (() => {});
    this.onData = options.onData || (() => {});
    this.onError = options.onError || (() => {});
    
    // Visualização
    this.analyser = null;
    this.audioContext = null;
  }
  
  _getPreferredMimeType() {
    // Ordem de preferência para compatibilidade com Whisper
    const types = [
      'audio/webm;codecs=opus',
      'audio/webm',
      'audio/ogg;codecs=opus',
      'audio/mp4',
      'audio/wav'
    ];
    
    for (const type of types) {
      if (MediaRecorder.isTypeSupported(type)) {
        return type;
      }
    }
    
    return 'audio/webm';
  }
  
  /**
   * Verifica se o navegador suporta MediaRecorder
   */
  static isSupported() {
    return 'MediaRecorder' in window && 'getUserMedia' in navigator.mediaDevices;
  }
  
  /**
   * Solicita permissão do microfone e prepara para gravar
   */
  async init() {
    try {
      this.stream = await navigator.mediaDevices.getUserMedia({ 
        audio: {
          channelCount: 1,
          sampleRate: 16000,
          echoCancellation: true,
          noiseSuppression: true
        } 
      });
      
      this.mediaRecorder = new MediaRecorder(this.stream, {
        mimeType: this.mimeType,
        audioBitsPerSecond: this.audioBitsPerSecond
      });
      
      this.mediaRecorder.ondataavailable = (event) => {
        if (event.data.size > 0) {
          this.chunks.push(event.data);
          this.onData(event.data);
        }
      };
      
      this.mediaRecorder.onstop = () => {
        const blob = new Blob(this.chunks, { type: this.mimeType });
        this.chunks = [];
        this.isRecording = false;
        this.onStop(blob);
      };
      
      this.mediaRecorder.onerror = (event) => {
        this.isRecording = false;
        this.onError(event.error);
      };
      
      // Setup para visualização de áudio
      this._setupAnalyser();
      
      return true;
    } catch (error) {
      const message = error.name === 'NotAllowedError' 
        ? 'Permissão de microfone negada'
        : 'Erro ao acessar microfone: ' + error.message;
      this.onError(message);
      return false;
    }
  }
  
  _setupAnalyser() {
    try {
      this.audioContext = new (window.AudioContext || window.webkitAudioContext)();
      this.analyser = this.audioContext.createAnalyser();
      this.analyser.fftSize = 256;
      
      const source = this.audioContext.createMediaStreamSource(this.stream);
      source.connect(this.analyser);
    } catch (error) {
      console.warn('Não foi possível criar analyser:', error);
    }
  }
  
  /**
   * Inicia a gravação
   */
  start() {
    if (!this.mediaRecorder) {
      this.onError('Recorder não inicializado. Chame init() primeiro.');
      return false;
    }
    
    if (this.isRecording) {
      return false;
    }
    
    this.chunks = [];
    this.mediaRecorder.start(100); // Envia dados a cada 100ms
    this.isRecording = true;
    this.onStart();
    return true;
  }
  
  /**
   * Para a gravação
   */
  stop() {
    if (!this.mediaRecorder || !this.isRecording) {
      return;
    }
    
    this.mediaRecorder.stop();
  }
  
  /**
   * Retorna nível de áudio atual (0-1)
   * Útil para visualização
   */
  getLevel() {
    if (!this.analyser) {
      return 0;
    }
    
    const dataArray = new Uint8Array(this.analyser.frequencyBinCount);
    this.analyser.getByteFrequencyData(dataArray);
    
    // Média dos valores
    const average = dataArray.reduce((a, b) => a + b, 0) / dataArray.length;
    return average / 255;
  }
  
  /**
   * Retorna dados de frequência para visualização
   */
  getFrequencyData() {
    if (!this.analyser) {
      return new Uint8Array(0);
    }
    
    const dataArray = new Uint8Array(this.analyser.frequencyBinCount);
    this.analyser.getByteFrequencyData(dataArray);
    return dataArray;
  }
  
  /**
   * Libera recursos
   */
  dispose() {
    if (this.mediaRecorder && this.isRecording) {
      this.mediaRecorder.stop();
    }
    
    if (this.stream) {
      this.stream.getTracks().forEach(track => track.stop());
      this.stream = null;
    }
    
    if (this.audioContext) {
      this.audioContext.close();
      this.audioContext = null;
    }
    
    this.mediaRecorder = null;
    this.analyser = null;
  }
}

