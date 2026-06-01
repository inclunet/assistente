import React from 'react';
import { CloseOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { SHORTCUTS } from '../../constants/chat';
import './KeyboardShortcutsHelp.css';

export interface KeyboardShortcutsHelpProps {
  isOpen: boolean;
  onClose: () => void;
}

interface ShortcutEntry {
  keys: string;
  description: string;
}

interface ShortcutCategory {
  id: string;
  title: string;
  items: ShortcutEntry[];
}

const FOCUSABLE_SELECTOR =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function KeyboardShortcutsHelp({ isOpen, onClose }: KeyboardShortcutsHelpProps) {
  const { t } = useTranslation();
  const panelRef = React.useRef<HTMLDivElement>(null);
  const previousFocusRef = React.useRef<HTMLElement | null>(null);

  React.useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
        return;
      }

      if (e.key === 'Tab') {
        const panel = panelRef.current;
        if (!panel) return;
        const focusable = Array.from(
          panel.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR),
        ).filter((el) => el.offsetParent !== null || el === document.activeElement);
        if (focusable.length === 0) {
          e.preventDefault();
          panel.focus();
          return;
        }
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        const active = document.activeElement as HTMLElement | null;
        if (e.shiftKey && active === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && active === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose]);

  React.useEffect(() => {
    if (!isOpen) return;
    previousFocusRef.current = document.activeElement as HTMLElement | null;
    const panel = panelRef.current;
    const firstFocusable = panel?.querySelector<HTMLElement>(FOCUSABLE_SELECTOR);
    requestAnimationFrame(() => {
      (firstFocusable ?? panel)?.focus();
    });
    return () => {
      previousFocusRef.current?.focus?.();
    };
  }, [isOpen]);

  if (!isOpen) return null;

  const categories: ShortcutCategory[] = [
    {
      id: 'navigation',
      title: t('ui.shortcuts.categories.navigation'),
      items: [
        { keys: 'Ctrl+T', description: t('ui.shortcuts.newChatTab') },
        { keys: SHORTCUTS.NEW_TAB, description: t('ui.shortcuts.newConversation') },
        { keys: 'Ctrl+W', description: t('ui.shortcuts.closeTab') },
        { keys: 'Ctrl+Tab', description: t('ui.shortcuts.nextTab') },
        { keys: 'Ctrl+Shift+Tab', description: t('ui.shortcuts.previousTab') },
        { keys: 'Ctrl+1…9', description: t('ui.shortcuts.goToTab') },
        { keys: SHORTCUTS.PREV_TAB, description: t('ui.shortcuts.navigateTabs') },
      ],
    },
    {
      id: 'chat',
      title: t('ui.shortcuts.categories.chat'),
      items: [
        { keys: 'Ctrl+Enter', description: t('ui.shortcuts.sendMessage') },
        { keys: SHORTCUTS.CLEAR_CONVERSATION, description: t('ui.shortcuts.clearConversation') },
        { keys: SHORTCUTS.HISTORY, description: t('ui.shortcuts.openHistory') },
        { keys: SHORTCUTS.MODELS, description: t('ui.shortcuts.selectModel') },
        { keys: SHORTCUTS.PROFILES, description: t('ui.shortcuts.interactionProfiles') },
        { keys: SHORTCUTS.SPEAK_MESSAGE, description: t('ui.shortcuts.playAudio') },
        { keys: SHORTCUTS.MESSAGE_DETAILS, description: t('ui.shortcuts.viewDetails') },
        { keys: 'Shift+F10', description: t('ui.shortcuts.contextMenu') },
        { keys: '↑', description: t('ui.shortcuts.prevMessage') },
        { keys: '↓', description: t('ui.shortcuts.nextMessage') },
      ],
    },
    {
      id: 'general',
      title: t('ui.shortcuts.categories.general'),
      items: [
        { keys: 'Ctrl+?', description: t('ui.shortcuts.showHelp') },
        { keys: 'F1', description: t('ui.shortcuts.openHelpPage') },
        { keys: 'Alt+M', description: t('ui.shortcuts.openMenu') },
        { keys: 'Esc', description: t('ui.shortcuts.closeDialog') },
      ],
    },
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
        ref={panelRef}
        tabIndex={-1}
      >
        <div className="keyboard-shortcuts-header">
          <h2 id="shortcuts-title">{t('ui.shortcuts.title')}</h2>
          <button
            className="keyboard-shortcuts-close"
            onClick={onClose}
            aria-label={t('ui.shortcuts.close')}
          >
            <CloseOutlined aria-hidden="true" />
          </button>
        </div>

        <div className="keyboard-shortcuts-content">
          {categories.map((category) => (
            <section
              key={category.id}
              className="keyboard-shortcuts-category"
              aria-labelledby={`shortcuts-category-${category.id}`}
            >
              <h3
                id={`shortcuts-category-${category.id}`}
                className="keyboard-shortcuts-category-title"
              >
                {category.title}
              </h3>
              {category.items.map((shortcut, index) => (
                <div key={index} className="keyboard-shortcut-item">
                  <kbd className="keyboard-shortcut-keys">{shortcut.keys}</kbd>
                  <span className="keyboard-shortcut-description">{shortcut.description}</span>
                </div>
              ))}
            </section>
          ))}
        </div>

        <div className="keyboard-shortcuts-footer">
          <p>{t('ui.shortcuts.escToClose')}</p>
        </div>
      </div>
    </div>
  );
}
