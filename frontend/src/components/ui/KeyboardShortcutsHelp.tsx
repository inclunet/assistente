import { useTranslation } from 'react-i18next';
import { SHORTCUTS } from '../../constants/chat';
import { Modal } from './Modal';
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

const ESC_HINT_ID = 'keyboard-shortcuts-esc-hint';

/**
 * Painel de atalhos de teclado.
 *
 * Usa o componente compartilhado `Modal`, de modo que enquanto aberto ele se
 * registra no stack global de modais (`isModalOpen()` passa a retornar true).
 * Assim, todos os handlers globais que respeitam `isModalOpen()` — F6 em
 * `useLandmarkNavigation`, navegação de abas, etc. — deixam de agir na UI de
 * fundo, e o `Modal` cuida de focus trap, ESC no topo do stack e inert/aria
 * no fundo. O contrato da store `shortcutsHelpStore` (isOpen/onClose) é mantido.
 */
export function KeyboardShortcutsHelp({ isOpen, onClose }: KeyboardShortcutsHelpProps) {
  const { t } = useTranslation();

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
        { keys: 'Ctrl+PageDown / Ctrl+PageUp', description: t('ui.shortcuts.navigateTabs') },
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
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('ui.shortcuts.title')}
      size="md"
      className="keyboard-shortcuts-modal"
      ariaDescribedBy={ESC_HINT_ID}
    >
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

      <p id={ESC_HINT_ID} className="keyboard-shortcuts-footer-text">
        {t('ui.shortcuts.escToClose')}
      </p>
    </Modal>
  );
}
