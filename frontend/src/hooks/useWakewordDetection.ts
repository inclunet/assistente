/**
 * useWakewordDetection - Hook para detecção de palavra de ativação usando WebSpeech API
 * 
 * Funciona como um serviço de escuta contínua que detecta uma keyword específica
 * e emite um evento quando detectada.
 */

import { useCallback, useRef, useState, useEffect } from 'react';

type SpeechRecognitionAlternative = { transcript: string; confidence: number };
type SpeechRecognitionResult = {
  isFinal: boolean;
  length: number;
  item: (index: number) => SpeechRecognitionAlternative;
  [index: number]: SpeechRecognitionAlternative;
};
type SpeechRecognitionResultList = {
  length: number;
  item: (index: number) => SpeechRecognitionResult;
  [index: number]: SpeechRecognitionResult;
};
type SpeechRecognitionEvent = Event & {
  resultIndex: number;
  results: SpeechRecognitionResultList;
};
type SpeechRecognitionErrorEvent = Event & { error: string };
type SpeechRecognitionInstance = {
  continuous: boolean;
  interimResults: boolean;
  lang: string;
  maxAlternatives: number;
  onresult: ((this: SpeechRecognitionInstance, event: SpeechRecognitionEvent) => void) | null;
  onerror: ((this: SpeechRecognitionInstance, event: SpeechRecognitionErrorEvent) => void) | null;
  onend: ((this: SpeechRecognitionInstance, event: Event) => void) | null;
  start: () => void;
  stop: () => void;
};
type SpeechRecognitionConstructor = new () => SpeechRecognitionInstance;
type SpeechRecognitionWindow = Window & {
  SpeechRecognition?: SpeechRecognitionConstructor;
  webkitSpeechRecognition?: SpeechRecognitionConstructor;
};

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
  const recognitionWindow = window as SpeechRecognitionWindow;
  const isSupported = typeof window !== 'undefined' && 
    (!!recognitionWindow.SpeechRecognition || !!recognitionWindow.webkitSpeechRecognition);

  // Cria instância de reconhecimento
  const createRecognition = useCallback((): SpeechRecognitionInstance | null => {
    if (!isSupported) return null;

    const SpeechRecognition = recognitionWindow.SpeechRecognition || recognitionWindow.webkitSpeechRecognition;
    if (!SpeechRecognition) return null;
    const recognition = new SpeechRecognition();
    const instance = recognition as SpeechRecognitionInstance;

    instance.continuous = true;
    instance.interimResults = true;
    instance.lang = language;
    instance.maxAlternatives = 1;

    instance.onresult = (event: SpeechRecognitionEvent) => {
      let transcript = '';
      
      // Pega o texto de todos os resultados
      for (let i = event.resultIndex; i < event.results.length; i++) {
        transcript += event.results[i][0].transcript;
      }
      
      const normalizedTranscript = transcript.toLowerCase().trim();
      setLastRecognizedText(normalizedTranscript);
      
      // Verifica se contém a keyword
      if (normalizedTranscript.includes(keywordRef.current)) {
        onDetectedRef.current?.(keywordRef.current, normalizedTranscript);
      }
    };

    instance.onerror = (event: SpeechRecognitionErrorEvent) => {
      // Alguns erros são esperados e não devem parar a escuta
      if (event.error === 'no-speech' || event.error === 'aborted') {
        // Reinicia automaticamente se ainda deveria estar escutando
        if (shouldRestartRef.current && isListeningRef.current) {
          setTimeout(() => {
            if (isListeningRef.current && globalRecognition) {
              try {
                globalRecognition.start();
              } catch {
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

    instance.onend = () => {
      // Reinicia automaticamente se ainda deveria estar escutando
      if (shouldRestartRef.current && isListeningRef.current) {
        setTimeout(() => {
          if (isListeningRef.current && globalRecognition) {
            try {
              globalRecognition.start();
            } catch {
              // Ignora se já está rodando
            }
          }
        }, 100);
      }
    };

    return instance;
  }, [isSupported, language]);

  // Inicia escuta
  const startListening = useCallback(() => {
    if (!isSupported) {
      onErrorRef.current?.('WebSpeech não suportado neste navegador');
      return;
    }

    if (globalIsListening) {
      return;
    }
    
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
    } catch {
      onErrorRef.current?.('Falha ao iniciar reconhecimento');
      shouldRestartRef.current = false;
      isListeningRef.current = false;
      globalIsListening = false;
    }
  }, [isSupported, createRecognition]);

  // Para escuta
  const stopListening = useCallback(() => {
    shouldRestartRef.current = false;
    isListeningRef.current = false;
    globalIsListening = false;

    if (globalRecognition) {
      try {
        globalRecognition.stop();
      } catch {
        // Ignora erros ao parar
      }
      globalRecognition = null;
    }

    setIsListening(false);
    setLastRecognizedText('');
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
