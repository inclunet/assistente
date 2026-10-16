import { useTranslation } from 'react-i18next';
import {
  useCommandSequencePrefixHint,
  useCommandShortcutHints,
} from '../../lib/commandShortcutHints';
import { Modal } from './Modal';
import './KeyboardShortcutsHelp.css';

export interface KeyboardShortcutsHelpProps {
  isOpen: boolean;
  onClose: () => void;
  surfaceType?: string;
}

interface ShortcutEntry {
  commandIds?: readonly string[];
  keys?: string;
  description: string;
  native?: boolean;
}

interface ShortcutCategory {
  id: string;
  title: string;
  items: ShortcutEntry[];
}

const ESC_HINT_ID = 'keyboard-shortcuts-esc-hint';
const UNAVAILABLE_KEY = 'ui.shortcuts.unavailable';
const NEW_TAB_SEQUENCE_COMMANDS = [
  'workspace.tab.chat.create', 'workspace.tab.editor.create',
  'workspace.tab.terminal.create', 'workspace.tab.tasklist.create',
] as const;

const command = (commandId: string, description: string): ShortcutEntry => ({
  commandIds: [commandId],
  description,
});

const commands = (commandIds: readonly string[], description: string): ShortcutEntry => ({
  commandIds,
  description,
});

const nativeGesture = (keys: string, description: string): ShortcutEntry => ({
  keys,
  description,
  native: true,
});

const legacyGesture = (keys: string, description: string): ShortcutEntry => ({
  keys,
  description,
});

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
export function KeyboardShortcutsHelp({ isOpen, onClose, surfaceType }: KeyboardShortcutsHelpProps) {
  const { t } = useTranslation();
  const shortcutHint = useCommandShortcutHints(surfaceType);
  const sequencePrefixHint = useCommandSequencePrefixHint(NEW_TAB_SEQUENCE_COMMANDS, surfaceType);

  const project = (entry: ShortcutEntry): string => {
    if (!entry.commandIds) return entry.keys ?? t(UNAVAILABLE_KEY);
    const hints = [...new Set(entry.commandIds.map(shortcutHint).filter((hint): hint is string => Boolean(hint)))];
    return hints.length > 0 ? hints.join(', ') : t(UNAVAILABLE_KEY);
  };

  const tabNumberCommands = Array.from({ length: 9 }, (_, index) => `workspace.tab.${[
    'first', 'second', 'third', 'fourth', 'fifth', 'sixth', 'seventh', 'eighth', 'ninth',
  ][index]}`);

  const categories: ShortcutCategory[] = [
    {
      id: 'navigation',
      title: t('ui.shortcuts.categories.navigation'),
      items: [
        command('workspace.tab.chat.create', t('ui.shortcuts.newChatTab')),
        { keys: sequencePrefixHint ?? t(UNAVAILABLE_KEY), description: t('ui.shortcuts.openNewTabMenu') },
        commands(['workspace.tab.close'], t('ui.shortcuts.closeTab')),
        commands(['workspace.tab.next'], t('ui.shortcuts.nextTab')),
        commands(['workspace.tab.previous'], t('ui.shortcuts.previousTab')),
        commands(tabNumberCommands, t('ui.shortcuts.goToTab')),
        commands(['workspace.tab.next', 'workspace.tab.previous'], t('ui.shortcuts.navigateTabs')),
      ],
    },
    {
      id: 'chat',
      title: t('ui.shortcuts.categories.chat'),
      items: [
        command('chat.message.send', t('ui.shortcuts.sendMessage')),
        command('chat.conversation.clear', t('ui.shortcuts.clearConversation')),
        command('chat.history.open', t('ui.shortcuts.openHistory')),
        command('chat.model.open', t('ui.shortcuts.selectModel')),
        command('chat.profile.open', t('ui.shortcuts.interactionProfiles')),
        nativeGesture('Space', t('ui.shortcuts.playAudio')),
        nativeGesture('Enter', t('ui.shortcuts.viewDetails')),
        nativeGesture('Shift+F10', t('ui.shortcuts.contextMenu')),
        nativeGesture('↑', t('ui.shortcuts.prevMessage')),
        nativeGesture('↓', t('ui.shortcuts.nextMessage')),
      ],
    },
    {
      id: 'decision',
      title: t('ui.shortcuts.categories.decision'),
      items: [
        nativeGesture('Ctrl+Enter', t('ui.shortcuts.decisionAffirmCurrent')),
        nativeGesture('Ctrl+Backspace', t('ui.shortcuts.decisionRejectCurrent')),
        nativeGesture('Shift+Enter', t('ui.shortcuts.decisionAffirmConversation')),
        nativeGesture('Shift+Backspace', t('ui.shortcuts.decisionRejectConversation')),
        nativeGesture('Ctrl+Shift+Enter', t('ui.shortcuts.decisionAffirmPersistent')),
        nativeGesture('Ctrl+Shift+Backspace', t('ui.shortcuts.decisionRejectPersistent')),
        nativeGesture('Alt+A…Z', t('ui.shortcuts.decisionMnemonic')),
        nativeGesture('Ctrl+Shift+R', t('ui.shortcuts.repeatDecisionPrompt')),
      ],
    },
    {
      id: 'general',
      title: t('ui.shortcuts.categories.general'),
      items: [
        command('navigation.palette.open', t('commandPalette.title', { defaultValue: 'Command Palette' })),
        legacyGesture('Ctrl+?', t('ui.shortcuts.showHelp')),
        command('navigation.help.open', t('ui.shortcuts.openHelpPage')),
        command('navigation.menu.open', t('ui.shortcuts.openMenu')),
        command('navigation.workspace.open', t('ui.shortcuts.goToWorkspace')),
        command('navigation.settings.open', t('ui.shortcuts.goToSettings')),
        command('navigation.history.open', t('ui.shortcuts.goToHistory')),
        command('navigation.memories.open', t('ui.shortcuts.goToMemories')),
        command('navigation.tasklists.open', t('ui.shortcuts.goToTasklists')),
        command('navigation.jobs.open', t('ui.shortcuts.goToJobs')),
        command('navigation.profiles.open', t('ui.shortcuts.goToProfiles')),
        nativeGesture('Esc', t('ui.shortcuts.closeDialog')),
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
        <p className="keyboard-shortcuts-native-note">
          {t('ui.shortcuts.nativeGesturesNote')}
        </p>
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
                <kbd className="keyboard-shortcut-keys">{project(shortcut)}</kbd>
                <span className="keyboard-shortcut-description">
                  {shortcut.description}
                  {shortcut.native && (
                    <span className="keyboard-shortcut-native-label"> ({t('ui.shortcuts.componentGesture')})</span>
                  )}
                </span>
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
