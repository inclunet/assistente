/**
 * useWakewordDetection - Hook para detecção de palavra de ativação usando WebSpeech API
 * 
 * Funciona como um serviço de escuta contínua que detecta uma keyword específica
 * e emite um evento quando detectada.
 */

import { useCallback, useRef, useState, useEffect } from 'react';

// Usamos any para o SpeechRecognition pois os tipos do DOM podem conflitar
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type SpeechRecognitionInstance = any;

export interface UseWakewordDetectionOptions {
  /** Palavra-chave para detectar (case insensitive) */
  keyword: string;
  /** Callback quando wakeword é detectado */
  onDetected?: (keyword: string, fullText: string) => void;
  /** Callback de erro */
  onError?: (message: string) => void;
  /** Sensibilidade (não usado diretamente, mas disponível para futuras implementações) */
  sensitivity?: number;
  /** Idioma para reconhecimento */
  language?: string;
}

export interface UseWakewordDetectionReturn {
  /** Se está escutando por wakeword */
  isListening: boolean;
  /** Inicia escuta por wakeword */
  startListening: () => void;
  /** Para escuta por wakeword */
  stopListening: () => void;
  /** Toggle escuta */
  toggleListening: () => void;
  /** Se WebSpeech está disponível */
  isSupported: boolean;
  /** Último texto reconhecido (para debug) */
  lastRecognizedText: string;
}

// Singleton para evitar múltiplas instâncias de reconhecimento
let globalRecognition: SpeechRecognitionInstance | null = null;
let globalIsListening = false;

export function useWakewordDetection(options: UseWakewordDetectionOptions): UseWakewordDetectionReturn {
  const {
    keyword,
    onDetected,
    onError,
    // sensitivity = 0.5, // Reservado para futuras implementações
    language = 'pt-BR',
  } = options;

  const [isListening, setIsListening] = useState(false);
  const [lastRecognizedText, setLastRecognizedText] = useState('');
  
  const keywordRef = useRef(keyword.toLowerCase());
  const onDetectedRef = useRef(onDetected);
  const onErrorRef = useRef(onError);
  const shouldRestartRef = useRef(false);
  const isListeningRef = useRef(false);

  // Atualiza refs quando mudam
  useEffect(() => {
    keywordRef.current = keyword.toLowerCase();
  }, [keyword]);

  useEffect(() => {
    onDetectedRef.current = onDetected;
  }, [onDetected]);

  useEffect(() => {
    onErrorRef.current = onError;
  }, [onError]);

  // Verifica suporte
  const isSupported = typeof window !== 'undefined' && 
    ('SpeechRecognition' in window || 'webkitSpeechRecognition' in window);

  // Cria instância de reconhecimento
  const createRecognition = useCallback(() => {
    if (!isSupported) return null;

    const SpeechRecognition = window.SpeechRecognition || (window as any).webkitSpeechRecognition;
    const recognition = new SpeechRecognition();

    recognition.continuous = true;
    recognition.interimResults = true;
    recognition.lang = language;
    recognition.maxAlternatives = 1;

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    recognition.onresult = (event: any) => {
      let transcript = '';
      
      // Pega o texto de todos os resultados
      for (let i = event.resultIndex; i < event.results.length; i++) {
        transcript += event.results[i][0].transcript;
      }
      
      const normalizedTranscript = transcript.toLowerCase().trim();
      setLastRecognizedText(normalizedTranscript);
      
      console.log('[WakewordDetection] Recognized:', normalizedTranscript);
      
      // Verifica se contém a keyword
      if (normalizedTranscript.includes(keywordRef.current)) {
        console.log('[WakewordDetection] 🎯 Keyword detected:', keywordRef.current);
        onDetectedRef.current?.(keywordRef.current, normalizedTranscript);
      }
    };

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    recognition.onerror = (event: any) => {
      console.error('[WakewordDetection] Error:', event.error);
      
      // Alguns erros são esperados e não devem parar a escuta
      if (event.error === 'no-speech' || event.error === 'aborted') {
        // Reinicia automaticamente se ainda deveria estar escutando
        if (shouldRestartRef.current && isListeningRef.current) {
          console.log('[WakewordDetection] Restarting after:', event.error);
          setTimeout(() => {
            if (isListeningRef.current && globalRecognition) {
              try {
                globalRecognition.start();
              } catch (e) {
                // Ignora se já está rodando
              }
            }
          }, 100);
        }
        return;
      }
      
      if (event.error === 'not-allowed') {
        onErrorRef.current?.('Permissão de microfone negada');
        setIsListening(false);
        isListeningRef.current = false;
      } else {
        onErrorRef.current?.(`Erro de reconhecimento: ${event.error}`);
      }
    };

    recognition.onend = () => {
      console.log('[WakewordDetection] Recognition ended, shouldRestart:', shouldRestartRef.current);
      
      // Reinicia automaticamente se ainda deveria estar escutando
      if (shouldRestartRef.current && isListeningRef.current) {
        console.log('[WakewordDetection] Auto-restarting...');
        setTimeout(() => {
          if (isListeningRef.current && globalRecognition) {
            try {
              globalRecognition.start();
            } catch (e) {
              // Ignora se já está rodando
            }
          }
        }, 100);
      }
    };

    return recognition;
  }, [isSupported, language]);

  // Inicia escuta
  const startListening = useCallback(() => {
    if (!isSupported) {
      onErrorRef.current?.('WebSpeech não suportado neste navegador');
      return;
    }

    if (globalIsListening) {
      console.log('[WakewordDetection] Already listening globally');
      return;
    }

    console.log('[WakewordDetection] Starting wakeword detection for:', keywordRef.current);
    
    // Cria nova instância se necessário
    if (!globalRecognition) {
      globalRecognition = createRecognition();
    }

    if (!globalRecognition) {
      onErrorRef.current?.('Falha ao criar reconhecimento de voz');
      return;
    }

    shouldRestartRef.current = true;
    isListeningRef.current = true;
    globalIsListening = true;

    try {
      globalRecognition.start();
      setIsListening(true);
      console.log('[WakewordDetection] ✅ Wakeword detection started');
    } catch (e) {
      console.error('[WakewordDetection] Failed to start:', e);
      onErrorRef.current?.('Falha ao iniciar reconhecimento');
      shouldRestartRef.current = false;
      isListeningRef.current = false;
      globalIsListening = false;
    }
  }, [isSupported, createRecognition]);

  // Para escuta
  const stopListening = useCallback(() => {
    console.log('[WakewordDetection] Stopping wakeword detection');
    
    shouldRestartRef.current = false;
    isListeningRef.current = false;
    globalIsListening = false;

    if (globalRecognition) {
      try {
        globalRecognition.stop();
      } catch (e) {
        // Ignora erros ao parar
      }
      globalRecognition = null;
    }

    setIsListening(false);
    setLastRecognizedText('');
    console.log('[WakewordDetection] ✅ Wakeword detection stopped');
  }, []);

  // Toggle
  const toggleListening = useCallback(() => {
    if (isListeningRef.current) {
      stopListening();
    } else {
      startListening();
    }
  }, [startListening, stopListening]);

  // Cleanup ao desmontar
  useEffect(() => {
    return () => {
      if (isListeningRef.current) {
        stopListening();
      }
    };
  }, [stopListening]);

  return {
    isListening,
    startListening,
    stopListening,
    toggleListening,
    isSupported,
    lastRecognizedText,
  };
}

export default useWakewordDetection;
