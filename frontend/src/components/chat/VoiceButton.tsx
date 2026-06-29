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
 * Usa o sistema unificado de perfis (profiles.Profile).
 */

import { logger } from '../../utils/logger';
import React, { useCallback, useMemo, useRef, useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useInteractionProfile } from '../../hooks/useInteractionProfile';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { finishSTTSession, requestSTTStart } from '../../services/voiceAccessibility/sttGate';
import { buildVoiceAccessibilityOriginFromTab } from '../../services/voiceAccessibility/types';
import { useWorkspacePanel } from '../workspace/WorkspacePanelContext';
import { profiles } from '../../../wailsjs/go/models';
import './VoiceButton.css';

type TriggerConfig = profiles.TriggerConfig;

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
  const { t } = useTranslation();
  const [isPTTActive, setIsPTTActive] = useState(false);
  const pttTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const { announceRequest } = useAnnouncer();

  // Cascata de perfil: tab.profileOverride.slug → workspace.profile → null (global)
  const workspace = useWorkspaceStore((s) => s.workspace);
  const { tab: panelTab, isActive: isPanelActive } = useWorkspacePanel();
  const tabProfileSlug = panelTab?.type === 'chat'
    ? (panelTab.profileOverride?.slug as string | undefined)
    : undefined;
  const effectiveProfileSlug = tabProfileSlug || workspace?.profile || null;
  const voiceOrigin = useMemo(
    () => (panelTab ? buildVoiceAccessibilityOriginFromTab(panelTab, workspace) : undefined),
    [panelTab, workspace],
  );

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
    isWakewordListening,
  } = useInteractionProfile({
    effectiveProfileSlug,
    onTranscription: (text, _provider) => {
      finishSTTSession(voiceOrigin);
      onTranscription(text);
      requestAnimationFrame(() => {
        if (textareaRef?.current) {
          textareaRef.current.focus();
        } else if (buttonRef.current && !buttonRef.current.disabled) {
          buttonRef.current.focus();
        }
      });
    },
    onError: (message) => {
      finishSTTSession(voiceOrigin);
      logger.error('[VoiceButton] Erro:', message);
    },
  });

  // Acessa triggers do perfil unificado
  const inputTriggers = activeProfile?.input?.triggers;

  // Determina o modo de interação baseado nos triggers do perfil
  const getInteractionMode = useCallback((): InteractionMode => {
    if (!inputTriggers) return 'ptt';
    
    const enabledTriggers = inputTriggers.filter((t: TriggerConfig) => t.enabled);
    
    // Prioridade: wakeword > vad > button_ptt > button_toggle
    const hasWakeword = enabledTriggers.some((t: TriggerConfig) => t.type === 'wakeword');
    if (hasWakeword) return 'wakeword';
    
    const hasVAD = enabledTriggers.some((t: TriggerConfig) => t.type === 'vad');
    if (hasVAD) return 'vad';
    
    const hasPTT = enabledTriggers.some((t: TriggerConfig) => t.type === 'button_ptt');
    if (hasPTT) return 'ptt';
    
    const hasToggle = enabledTriggers.some((t: TriggerConfig) => t.type === 'button_toggle');
    if (hasToggle) return 'toggle';
    
    // Default: PTT
    return 'ptt';
  }, [inputTriggers]);

  const mode = getInteractionMode();
  
  // Estado de escuta ativa (para VAD e Wakeword)
  // isWakewordListening é específico para quando está escutando a palavra de ativação
  const isListeningState = isListening || (mode === 'vad' && isActive) || isWakewordListening;
  const isDisabled = disabled || !isPanelActive;

  useEffect(() => {
    if (isPanelActive) return;
    if (!isActive && !isListeningState && !isPTTActive) return;
    cancelInteraction();
    finishSTTSession(voiceOrigin);
    setIsPTTActive(false);
  }, [cancelInteraction, isActive, isListeningState, isPTTActive, isPanelActive, voiceOrigin]);

  useEffect(() => {
    if (!interimText.trim()) return;
    announceRequest({
      message: interimText,
      origin: voiceOrigin,
      eventType: 'progress',
    });
  }, [announceRequest, interimText, voiceOrigin]);

  const startInteractionWithGate = useCallback((): boolean => {
    if (!requestSTTStart({ origin: voiceOrigin, cancel: cancelInteraction })) return false;
    void startInteraction();
    return true;
  }, [cancelInteraction, startInteraction, voiceOrigin]);

  const stopInteractionWithGate = useCallback(() => {
    stopInteraction();
    finishSTTSession(voiceOrigin);
  }, [stopInteraction, voiceOrigin]);

  const cancelInteractionWithGate = useCallback(() => {
    cancelInteraction();
    finishSTTSession(voiceOrigin);
  }, [cancelInteraction, voiceOrigin]);

  const toggleInteractionWithGate = useCallback(() => {
    if (isActive || isListeningState) {
      toggleInteraction();
      finishSTTSession(voiceOrigin);
      return;
    }

    if (!requestSTTStart({ origin: voiceOrigin, cancel: cancelInteraction })) return;
    toggleInteraction();
  }, [cancelInteraction, isActive, isListeningState, toggleInteraction, voiceOrigin]);

  // === Handlers para modo PTT ===
  
  const handlePointerDown = useCallback((e: React.PointerEvent) => {
    if (isDisabled || mode !== 'ptt') return;
    
    if (!startInteractionWithGate()) return;

    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    setIsPTTActive(true);
  }, [isDisabled, mode, startInteractionWithGate]);

  const handlePointerUp = useCallback((e: React.PointerEvent) => {
    if (mode !== 'ptt' || !isPTTActive) return;
    
    e.currentTarget.releasePointerCapture(e.pointerId);
    setIsPTTActive(false);
    stopInteractionWithGate();
  }, [mode, isPTTActive, stopInteractionWithGate]);

  const handlePointerLeave = useCallback((e: React.PointerEvent) => {
    // Se saiu do botão durante PTT, cancela
    if (mode === 'ptt' && isPTTActive) {
      e.currentTarget.releasePointerCapture(e.pointerId);
      setIsPTTActive(false);
      cancelInteractionWithGate();
    }
  }, [mode, isPTTActive, cancelInteractionWithGate]);

  // === Handler para modos Toggle, VAD e Wakeword ===
  
  const handleClick = useCallback(() => {
    if (isDisabled) return;
    
    // Toggle, VAD e Wakeword usam o mesmo comportamento de toggle
    if (mode === 'toggle' || mode === 'vad' || mode === 'wakeword') {
      toggleInteractionWithGate();
    }
    // No modo PTT, o click é tratado pelo pointer events
  }, [isDisabled, mode, toggleInteractionWithGate]);

  // === Keyboard handlers ===
  
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (isDisabled) return;
    
    // Ignora eventos de repetição de tecla (key repeat do OS)
    if (e.repeat) return;
    
    // Espaço ou Enter
    if (e.key === ' ' || e.key === 'Enter') {
      e.preventDefault();
      
      if (mode === 'ptt') {
        if (!isPTTActive) {
          if (startInteractionWithGate()) {
            setIsPTTActive(true);
          }
        }
      } else {
        // Toggle, VAD e Wakeword usam toggle
        toggleInteractionWithGate();
      }
    }
    
    // Escape cancela
    if (e.key === 'Escape' && (isActive || isListeningState)) {
      e.preventDefault();
      cancelInteractionWithGate();
      setIsPTTActive(false);
    }
  }, [isDisabled, mode, isPTTActive, isActive, isListeningState, startInteractionWithGate, toggleInteractionWithGate, cancelInteractionWithGate]);

  const handleKeyUp = useCallback((e: React.KeyboardEvent) => {
    if (mode === 'ptt' && isPTTActive && (e.key === ' ' || e.key === 'Enter')) {
      e.preventDefault();
      setIsPTTActive(false);
      stopInteractionWithGate();
      
      // Restaura foco após PTT - prefere textarea, senão volta para o botão
      requestAnimationFrame(() => {
        if (textareaRef?.current) {
          textareaRef.current.focus();
        } else if (buttonRef.current && !buttonRef.current.disabled) {
          buttonRef.current.focus();
        }
      });
    }
  }, [mode, isPTTActive, stopInteractionWithGate, textareaRef]);

  // Cleanup timeout on unmount
  useEffect(() => {
    return () => {
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

  // Labels e descrições por modo
  const wakewordKeyword = inputTriggers?.find((tr: TriggerConfig) => tr.type === 'wakeword')?.wakeword_keyword || t('voice.wakeWord');
  const modeLabels: Record<InteractionMode, { short: string; idle: string; active: string; hint: string }> = {
    ptt: {
      short: 'PTT',
      idle: t('voice.holdToRecord'),
      active: t('voice.recordingRelease'),
      hint: t('voice.hold'),
    },
    toggle: {
      short: '',
      idle: t('voice.clickToRecord'),
      active: t('voice.recordingClickStop'),
      hint: '',
    },
    vad: {
      short: 'VAD',
      idle: t('voice.clickContinuous'),
      active: isRecordingState ? t('voice.recordingSilence') : t('voice.listeningSpeakTo'),
      hint: t('voice.vadHint'),
    },
    wakeword: {
      short: '',
      idle: t('voice.clickWakeWord'),
      active: isRecordingState ? t('voice.recording') : `${t('voice.listeningSay')} "${wakewordKeyword}"`,
      hint: t('voice.wakeHint'),
    },
  };

  // Label de acessibilidade
  const getAriaLabel = () => {
    if (isProcessingState) return t('voice.processingTranscription');
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
        <div className="voice-button__interim">
          {interimText}
        </div>
      )}

      <button
        ref={buttonRef}
        type="button"
        className={`voice-button ${isRecordingState ? 'voice-button--recording' : ''} ${isListeningState && !isRecordingState ? 'voice-button--listening' : ''} ${isProcessingState ? 'voice-button--processing' : ''} ${getModeClass()}`}
        disabled={isDisabled || isProcessingState}
        aria-label={getAriaLabel()}
        aria-pressed={isRecordingState || isListeningState}
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
