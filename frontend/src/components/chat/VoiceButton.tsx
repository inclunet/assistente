/**
 * VoiceButton - Botão de Gravação por Voz
 * 
 * Suporta múltiplos modos de interação baseado no perfil ativo:
 * - PTT (Push-to-Talk): Segura para gravar, solta para enviar
 * - Toggle: Clica para iniciar, clica novamente para parar
 * - VAD: Clica para iniciar escuta contínua, VAD detecta fala e silêncio
 * - Wakeword: Clica para ativar/desativar escuta por palavra de ativação
 * 
 * Integra com useInteractionProfile para VAD, hotkeys e wakeword.
 */

import React, { useCallback, useRef, useState, useEffect } from 'react';
import { useInteractionProfile } from '../../hooks/useInteractionProfile';
import './VoiceButton.css';

// Tipos de modo de interação
type InteractionMode = 'ptt' | 'toggle' | 'vad' | 'wakeword';

export interface VoiceButtonProps {
  /** Callback quando transcrição é finalizada */
  onTranscription: (text: string) => void;
  /** Se o botão está desabilitado */
  disabled?: boolean;
  /** Classe CSS adicional */
  className?: string;
  /** Ref para o textarea para restaurar foco após PTT */
  textareaRef?: React.RefObject<HTMLTextAreaElement>;
}

export const VoiceButton: React.FC<VoiceButtonProps> = ({
  onTranscription,
  disabled = false,
  className = '',
  textareaRef,
}) => {
  const [isPTTActive, setIsPTTActive] = useState(false);
  const pttTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  const {
    isActive,
    isListening,
    isRecording,
    isProcessing,
    volume,
    interimText,
    activeProfile,
    startInteraction,
    stopInteraction,
    cancelInteraction,
    toggleInteraction,
  } = useInteractionProfile({
    onTranscription: (text, _provider) => {
      onTranscription(text);
      // Restaura foco após transcrição - prefere textarea, senão botão
      requestAnimationFrame(() => {
        if (textareaRef?.current) {
          textareaRef.current.focus();
        } else if (buttonRef.current && !buttonRef.current.disabled) {
          buttonRef.current.focus();
        }
      });
    },
    onError: (message) => {
      console.error('[VoiceButton] Erro:', message);
    },
  });

  // Determina o modo de interação baseado nos triggers do perfil
  const getInteractionMode = useCallback((): InteractionMode => {
    if (!activeProfile?.triggers) return 'ptt';
    
    const enabledTriggers = activeProfile.triggers.filter(t => t.enabled);
    
    // Prioridade: wakeword > vad > button_ptt > button_toggle
    const hasWakeword = enabledTriggers.some(t => t.type === 'wakeword');
    if (hasWakeword) return 'wakeword';
    
    const hasVAD = enabledTriggers.some(t => t.type === 'vad');
    if (hasVAD) return 'vad';
    
    const hasPTT = enabledTriggers.some(t => t.type === 'button_ptt');
    if (hasPTT) return 'ptt';
    
    const hasToggle = enabledTriggers.some(t => t.type === 'button_toggle');
    if (hasToggle) return 'toggle';
    
    // Default: PTT
    return 'ptt';
  }, [activeProfile]);

  const mode = getInteractionMode();
  
  // Estado de escuta ativa (para VAD e Wakeword)
  const isListeningState = isListening || (mode === 'vad' && isActive) || (mode === 'wakeword' && isActive);

  // === Handlers para modo PTT ===
  
  const handlePointerDown = useCallback((e: React.PointerEvent) => {
    if (disabled || mode !== 'ptt') return;
    
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    
    setIsPTTActive(true);
    startInteraction();
  }, [disabled, mode, startInteraction]);

  const handlePointerUp = useCallback((e: React.PointerEvent) => {
    if (mode !== 'ptt' || !isPTTActive) return;
    
    e.currentTarget.releasePointerCapture(e.pointerId);
    setIsPTTActive(false);
    stopInteraction();
  }, [mode, isPTTActive, stopInteraction]);

  const handlePointerLeave = useCallback((e: React.PointerEvent) => {
    // Se saiu do botão durante PTT, cancela
    if (mode === 'ptt' && isPTTActive) {
      e.currentTarget.releasePointerCapture(e.pointerId);
      setIsPTTActive(false);
      cancelInteraction();
    }
  }, [mode, isPTTActive, cancelInteraction]);

  // === Handler para modos Toggle, VAD e Wakeword ===
  
  const handleClick = useCallback(() => {
    if (disabled) return;
    
    // Toggle, VAD e Wakeword usam o mesmo comportamento de toggle
    if (mode === 'toggle' || mode === 'vad' || mode === 'wakeword') {
      toggleInteraction();
    }
    // No modo PTT, o click é tratado pelo pointer events
  }, [disabled, mode, toggleInteraction]);

  // === Keyboard handlers ===
  
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (disabled) return;
    
    // Ignora eventos de repetição de tecla (key repeat do OS)
    if (e.repeat) return;
    
    // Espaço ou Enter
    if (e.key === ' ' || e.key === 'Enter') {
      e.preventDefault();
      
      if (mode === 'ptt') {
        if (!isPTTActive) {
          setIsPTTActive(true);
          startInteraction();
        }
      } else {
        // Toggle, VAD e Wakeword usam toggle
        toggleInteraction();
      }
    }
    
    // Escape cancela
    if (e.key === 'Escape' && (isActive || isListeningState)) {
      e.preventDefault();
      cancelInteraction();
      setIsPTTActive(false);
    }
  }, [disabled, mode, isPTTActive, isActive, isListeningState, startInteraction, toggleInteraction, cancelInteraction]);

  const handleKeyUp = useCallback((e: React.KeyboardEvent) => {
    if (mode === 'ptt' && isPTTActive && (e.key === ' ' || e.key === 'Enter')) {
      e.preventDefault();
      // #region agent log
      fetch('http://127.0.0.1:7242/ingest/c14faa4a-a682-41c0-9f93-65632102ad3e',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({location:'VoiceButton.tsx:handleKeyUp',message:'PTT keyUp triggered',data:{key:e.key,activeElement:document.activeElement?.tagName,activeId:document.activeElement?.id,activeClass:document.activeElement?.className},timestamp:Date.now(),sessionId:'debug-session',hypothesisId:'B'})}).catch(()=>{});
      // #endregion
      setIsPTTActive(false);
      stopInteraction();
      
      // Restaura foco após PTT - prefere textarea, senão volta para o botão
      requestAnimationFrame(() => {
        if (textareaRef?.current) {
          textareaRef.current.focus();
        } else if (buttonRef.current && !buttonRef.current.disabled) {
          buttonRef.current.focus();
        }
      });
    }
  }, [mode, isPTTActive, stopInteraction, textareaRef]);

  // Cleanup timeout on unmount
  useEffect(() => {
    // #region agent log
    fetch('http://127.0.0.1:7242/ingest/c14faa4a-a682-41c0-9f93-65632102ad3e',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({location:'VoiceButton.tsx:mount',message:'VoiceButton mounted',data:{},timestamp:Date.now(),sessionId:'debug-session',hypothesisId:'E'})}).catch(()=>{});
    const focusHandler = (e: FocusEvent) => { fetch('http://127.0.0.1:7242/ingest/c14faa4a-a682-41c0-9f93-65632102ad3e',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({location:'VoiceButton.tsx:focusChange',message:'Focus changed',data:{newFocus:(e.target as HTMLElement)?.tagName,newClass:(e.target as HTMLElement)?.className?.slice(0,100),newId:(e.target as HTMLElement)?.id},timestamp:Date.now(),sessionId:'debug-session',hypothesisId:'D'})}).catch(()=>{}); };
    document.addEventListener('focusin', focusHandler);
    // #endregion
    return () => {
      // #region agent log
      document.removeEventListener('focusin', focusHandler);
      fetch('http://127.0.0.1:7242/ingest/c14faa4a-a682-41c0-9f93-65632102ad3e',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({location:'VoiceButton.tsx:unmount',message:'VoiceButton unmounting',data:{},timestamp:Date.now(),sessionId:'debug-session',hypothesisId:'E'})}).catch(()=>{});
      // #endregion
      if (pttTimeoutRef.current) {
        clearTimeout(pttTimeoutRef.current);
      }
    };
  }, []);

  // Calcula a escala do indicador de volume (1 a 1.3)
  const volumeScale = 1 + (volume * 0.3);

  // Estado visual
  const isRecordingState = isActive || isRecording || isPTTActive;
  const isProcessingState = isProcessing;

  // #region agent log
  useEffect(() => {
    fetch('http://127.0.0.1:7242/ingest/c14faa4a-a682-41c0-9f93-65632102ad3e',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({location:'VoiceButton.tsx:stateChange',message:'State changed',data:{isProcessingState,isRecordingState,disabled,activeElement:document.activeElement?.tagName,activeClass:document.activeElement?.className},timestamp:Date.now(),sessionId:'debug-session',hypothesisId:'A'})}).catch(()=>{});
  }, [isProcessingState, isRecordingState, disabled]);
  // #endregion

  // Labels e descrições por modo
  const modeLabels: Record<InteractionMode, { short: string; idle: string; active: string; hint: string }> = {
    ptt: {
      short: 'PTT',
      idle: 'Segure para gravar mensagem de voz',
      active: 'Gravando... Solte para enviar',
      hint: 'Segure',
    },
    toggle: {
      short: '',
      idle: 'Clique para gravar mensagem de voz',
      active: 'Gravando... Clique para parar',
      hint: '',
    },
    vad: {
      short: 'VAD',
      idle: 'Clique para iniciar escuta contínua',
      active: isRecordingState ? 'Gravando... Aguarde silêncio' : 'Ouvindo... Fale para gravar',
      hint: 'Escuta',
    },
    wakeword: {
      short: '🗣️',
      idle: 'Clique para ativar palavra de ativação',
      active: isRecordingState ? 'Gravando...' : `Ouvindo... Diga "${activeProfile?.triggers?.find(t => t.type === 'wakeword')?.wakeword_keyword || 'assistente'}"`,
      hint: 'Wake',
    },
  };

  // Label de acessibilidade
  const getAriaLabel = () => {
    if (isProcessingState) return 'Processando transcrição...';
    if (isRecordingState || isListeningState) {
      return modeLabels[mode].active;
    }
    return modeLabels[mode].idle;
  };

  // Tooltip
  const getTitle = () => {
    if (isProcessingState) return 'Processando...';
    if (isRecordingState || isListeningState) {
      return modeLabels[mode].active;
    }
    return modeLabels[mode].idle;
  };

  // Determina a classe CSS do modo
  const getModeClass = () => {
    switch (mode) {
      case 'ptt': return 'voice-button--ptt';
      case 'toggle': return 'voice-button--toggle';
      case 'vad': return 'voice-button--vad';
      case 'wakeword': return 'voice-button--wakeword';
      default: return '';
    }
  };

  // Ícone baseado no estado e modo
  const renderIcon = () => {
    // Processando
    if (isProcessingState) {
      return (
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none">
          <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="2" strokeDasharray="31.4" strokeDashoffset="10">
            <animateTransform attributeName="transform" type="rotate" from="0 12 12" to="360 12 12" dur="1s" repeatCount="indefinite"/>
          </circle>
        </svg>
      );
    }

    // Gravando
    if (isRecordingState) {
      // Quadrado para parar (toggle) ou círculo pulsante (outros)
      if (mode === 'toggle' || mode === 'vad') {
        return (
          <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
            <rect x="6" y="6" width="12" height="12" rx="2"/>
          </svg>
        );
      }
      return (
        <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
          <circle cx="12" cy="12" r="8">
            <animate attributeName="r" values="6;8;6" dur="1s" repeatCount="indefinite"/>
          </circle>
        </svg>
      );
    }

    // Ouvindo (VAD ou Wakeword)
    if (isListeningState) {
      return (
        <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
          {/* Ícone de ondas sonoras */}
          <path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z" opacity="0.6"/>
          <path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z" opacity="0.6"/>
          {/* Ondas animadas */}
          <circle cx="12" cy="9" r="6" fill="none" stroke="currentColor" strokeWidth="1" opacity="0.5">
            <animate attributeName="r" values="4;8;4" dur="1.5s" repeatCount="indefinite"/>
            <animate attributeName="opacity" values="0.8;0;0.8" dur="1.5s" repeatCount="indefinite"/>
          </circle>
        </svg>
      );
    }

    // Ícone padrão (microfone) - com variações por modo
    if (mode === 'wakeword') {
      return (
        <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
          {/* Ícone de balão de fala com mic */}
          <path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z"/>
          <path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z"/>
          <circle cx="18" cy="6" r="4" fill="currentColor" opacity="0.6"/>
        </svg>
      );
    }
    
    if (mode === 'vad') {
      return (
        <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
          {/* Ícone de mic com ondas */}
          <path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z"/>
          <path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z"/>
          <path d="M20 9v6M22 7v10" stroke="currentColor" strokeWidth="2" strokeLinecap="round" opacity="0.5"/>
        </svg>
      );
    }

    // Microfone padrão (PTT e Toggle)
    return (
      <svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
        <path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z"/>
        <path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z"/>
      </svg>
    );
  };

  return (
    <div className={`voice-button-container ${className}`}>
      {/* Texto interim (preview da transcrição) */}
      {interimText && (
        <div className="voice-button__interim" aria-live="polite">
          {interimText}
        </div>
      )}

      <button
        ref={buttonRef}
        type="button"
        className={`voice-button ${isRecordingState ? 'voice-button--recording' : ''} ${isListeningState && !isRecordingState ? 'voice-button--listening' : ''} ${isProcessingState ? 'voice-button--processing' : ''} ${getModeClass()}`}
        disabled={disabled || isProcessingState}
        aria-label={getAriaLabel()}
        aria-pressed={isRecordingState || isListeningState}
        title={getTitle()}
        // Pointer events para PTT
        onPointerDown={handlePointerDown}
        onPointerUp={handlePointerUp}
        onPointerLeave={handlePointerLeave}
        onPointerCancel={handlePointerLeave}
        // Click para Toggle, VAD, Wakeword
        onClick={handleClick}
        // Keyboard
        onKeyDown={handleKeyDown}
        onKeyUp={handleKeyUp}
      >
        {/* Indicador de volume (círculo pulsante) */}
        {(isRecordingState || isListeningState) && (
          <span 
            className="voice-button__volume-indicator"
            style={{ transform: `scale(${volumeScale})` }}
            aria-hidden="true"
          />
        )}

        {/* Ícone */}
        <span className="voice-button__icon" aria-hidden="true">
          {renderIcon()}
        </span>

        {/* Label de modo */}
        {modeLabels[mode].short && (
          <span className="voice-button__mode-label">
            {modeLabels[mode].short}
          </span>
        )}
      </button>

      {/* Dica visual de modo */}
      {modeLabels[mode].hint && !isRecordingState && !isListeningState && (
        <span className="voice-button__hint" aria-hidden="true">
          {modeLabels[mode].hint}
        </span>
      )}
    </div>
  );
};

export default VoiceButton;
