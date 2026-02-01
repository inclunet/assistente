import React from 'react';
import { SHORTCUTS } from '../../constants/chat';
import './KeyboardShortcutsHelp.css';

export interface KeyboardShortcutsHelpProps {
  isOpen: boolean;
  onClose: () => void;
}

export function KeyboardShortcutsHelp({ isOpen, onClose }: KeyboardShortcutsHelpProps) {
  React.useEffect(() => {
    if (!isOpen) return;

    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  const shortcuts = [
    { keys: SHORTCUTS.NEW_TAB, description: 'Nova conversa' },
    { keys: SHORTCUTS.PREV_TAB, description: 'Navegar entre conversas' },
    { keys: SHORTCUTS.HISTORY, description: 'Abrir histórico' },
    { keys: SHORTCUTS.MODELS, description: 'Selecionar modelo' },
    { keys: SHORTCUTS.PROFILES, description: 'Perfis de interação' },
    { keys: SHORTCUTS.SPEAK_MESSAGE, description: 'Reproduzir áudio (mensagem focada)' },
    { keys: SHORTCUTS.MESSAGE_DETAILS, description: 'Ver detalhes (mensagem focada)' },
    { keys: 'Shift+F10', description: 'Menu de contexto (mensagem focada)' },
    { keys: '↑', description: 'Focar mensagem anterior' },
    { keys: '↓', description: 'Focar próxima mensagem' },
    { keys: 'Ctrl+Enter', description: 'Enviar mensagem' },
    { keys: SHORTCUTS.HELP, description: 'Mostrar esta ajuda' },
  ];

  return (
    <div
      className="keyboard-shortcuts-overlay"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="shortcuts-title"
    >
      <div
        className="keyboard-shortcuts-panel"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="keyboard-shortcuts-header">
          <h2 id="shortcuts-title">Atalhos do Teclado</h2>
          <button
            className="keyboard-shortcuts-close"
            onClick={onClose}
            aria-label="Fechar"
          >
            ✕
          </button>
        </div>

        <div className="keyboard-shortcuts-content">
          {shortcuts.map((shortcut, index) => (
            <div key={index} className="keyboard-shortcut-item">
              <kbd className="keyboard-shortcut-keys">{shortcut.keys}</kbd>
              <span className="keyboard-shortcut-description">{shortcut.description}</span>
            </div>
          ))}
        </div>

        <div className="keyboard-shortcuts-footer">
          <p>Pressione <kbd>Esc</kbd> para fechar</p>
        </div>
      </div>
    </div>
  );
}
