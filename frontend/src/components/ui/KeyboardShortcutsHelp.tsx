import React from 'react';
import { useTranslation } from 'react-i18next';
import { SHORTCUTS } from '../../constants/chat';
import './KeyboardShortcutsHelp.css';

export interface KeyboardShortcutsHelpProps {
  isOpen: boolean;
  onClose: () => void;
}

export function KeyboardShortcutsHelp({ isOpen, onClose }: KeyboardShortcutsHelpProps) {
  const { t } = useTranslation();
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
    { keys: SHORTCUTS.NEW_TAB, description: t('ui.shortcuts.newConversation') },
    { keys: SHORTCUTS.CLEAR_CONVERSATION, description: t('ui.shortcuts.clearConversation') },
    { keys: SHORTCUTS.PREV_TAB, description: t('ui.shortcuts.navigateTabs') },
    { keys: SHORTCUTS.HISTORY, description: t('ui.shortcuts.openHistory') },
    { keys: SHORTCUTS.MODELS, description: t('ui.shortcuts.selectModel') },
    { keys: SHORTCUTS.PROFILES, description: t('ui.shortcuts.interactionProfiles') },
    { keys: SHORTCUTS.SPEAK_MESSAGE, description: t('ui.shortcuts.playAudio') },
    { keys: SHORTCUTS.MESSAGE_DETAILS, description: t('ui.shortcuts.viewDetails') },
    { keys: 'Shift+F10', description: t('ui.shortcuts.contextMenu') },
    { keys: '↑', description: t('ui.shortcuts.prevMessage') },
    { keys: '↓', description: t('ui.shortcuts.nextMessage') },
    { keys: 'Ctrl+Enter', description: t('ui.shortcuts.sendMessage') },
    { keys: SHORTCUTS.HELP, description: t('ui.shortcuts.showHelp') },
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
          <h2 id="shortcuts-title">{t('ui.shortcuts.title')}</h2>
          <button
            className="keyboard-shortcuts-close"
            onClick={onClose}
            aria-label={t('ui.shortcuts.close')}
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
          <p>{t('ui.shortcuts.escToClose')}</p>
        </div>
      </div>
    </div>
  );
}
