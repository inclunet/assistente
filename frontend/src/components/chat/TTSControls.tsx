/**
 * TTSControls Component
 * Controles de Text-to-Speech para a toolbar de chat
 */

import React from 'react';
import { useTTS } from '../../hooks/useTTS';
import './TTSControls.css';

export const TTSControls: React.FC = () => {
  const {
    isEnabled,
    isAutoReadEnabled,
    isSpeaking,
    setEnabled,
    setAutoRead,
    stop,
    isSupported,
  } = useTTS();
  
  if (!isSupported) {
    return null;
  }
  
  const handleToggle = () => {
    if (isEnabled) {
      // Desabilita e para qualquer fala em andamento
      stop();
      setEnabled(false);
    } else {
      setEnabled(true);
    }
  };
  
  const handleAutoReadToggle = () => {
    setAutoRead(!isAutoReadEnabled);
  };
  
  const handleStop = () => {
    stop();
  };
  
  return (
    <div className="tts-controls">
      <button
        className={`tts-button ${isEnabled ? 'active' : ''}`}
        onClick={handleToggle}
        title={isEnabled ? 'Desabilitar TTS' : 'Habilitar TTS'}
        aria-label={isEnabled ? 'Desabilitar leitura de texto' : 'Habilitar leitura de texto'}
        tabIndex={-1}
      >
        <svg
          width="20"
          height="20"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          {isEnabled ? (
            <>
              <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
              <path d="M15.54 8.46a5 5 0 0 1 0 7.07" />
              <path d="M19.07 4.93a10 10 0 0 1 0 14.14" />
            </>
          ) : (
            <>
              <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
              <line x1="23" y1="9" x2="17" y2="15" />
              <line x1="17" y1="9" x2="23" y2="15" />
            </>
          )}
        </svg>
      </button>
      
      {isEnabled && (
        <>
          <button
            className={`tts-button auto-read ${isAutoReadEnabled ? 'active' : ''}`}
            onClick={handleAutoReadToggle}
            title={isAutoReadEnabled ? 'Desabilitar leitura automática' : 'Habilitar leitura automática'}
            aria-label={isAutoReadEnabled ? 'Desabilitar leitura automática de respostas' : 'Habilitar leitura automática de respostas'}
            tabIndex={-1}
          >
            <svg
              width="20"
              height="20"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path d="M12 2v10l4.5 4.5" />
              <circle cx="12" cy="12" r="10" />
            </svg>
          </button>
          
          {isSpeaking && (
            <button
              className="tts-button stop"
              onClick={handleStop}
              title="Parar leitura"
              aria-label="Parar leitura de texto"
              tabIndex={-1}
            >
              <svg
                width="20"
                height="20"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <rect x="6" y="6" width="12" height="12" />
              </svg>
            </button>
          )}
        </>
      )}
    </div>
  );
};
