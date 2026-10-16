import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { KeyboardShortcutsHelp } from './KeyboardShortcutsHelp';
import { isModalOpen } from './Modal';

const mocks = vi.hoisted(() => {
  let projectedHints: Record<string, string | undefined> = {};
  const useCommandShortcutHints = vi.fn((surfaceType?: string) => {
    void surfaceType;
    return (commandId: string) => projectedHints[commandId];
  });
  const useCommandSequencePrefixHint = vi.fn(() => undefined as string | undefined);
  return {
    useCommandShortcutHints,
    useCommandSequencePrefixHint,
    setProjectedHints: (hints: Record<string, string | undefined>) => { projectedHints = hints; },
  };
});

vi.mock('../../lib/commandShortcutHints', () => ({
  useCommandSequencePrefixHint: mocks.useCommandSequencePrefixHint,
  useCommandShortcutHints: mocks.useCommandShortcutHints,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

describe('KeyboardShortcutsHelp', () => {
  it('renderiza quando aberto e fecha no Escape', () => {
    const onClose = vi.fn();
    render(<KeyboardShortcutsHelp isOpen={true} onClose={onClose} />);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });

  it('nao renderiza quando fechado', () => {
    render(<KeyboardShortcutsHelp isOpen={false} onClose={() => {}} />);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('agrupa as categorias e preserva os gestos invariantes', () => {
    render(<KeyboardShortcutsHelp isOpen={true} onClose={() => {}} />);
    expect(screen.getByText('ui.shortcuts.categories.navigation')).toBeInTheDocument();
    expect(screen.getByText('ui.shortcuts.categories.chat')).toBeInTheDocument();
    expect(screen.getByText('ui.shortcuts.categories.general')).toBeInTheDocument();
    expect(screen.getByText('Ctrl+?')).toBeInTheDocument();
    expect(screen.getByText('ui.shortcuts.sendMessage')).toBeInTheDocument();
    expect(screen.getByText('ui.shortcuts.decisionAffirmCurrent')).toBeInTheDocument();
    expect(document.querySelectorAll('.keyboard-shortcut-native-label').length).toBeGreaterThan(0);
  });

  it('mostra remapeamento e não anuncia associação suprimida', () => {
    mocks.setProjectedHints({
      'chat.message.send': 'Ctrl+Shift+Enter',
      'chat.model.open': 'Alt+Shift+M',
      'chat.profile.open': undefined,
    });
    render(<KeyboardShortcutsHelp isOpen={true} onClose={() => {}} surfaceType="chat" />);
    expect(screen.getByText('Alt+Shift+M')).toBeInTheDocument();
    expect(screen.getAllByText('Ctrl+Shift+Enter')).toHaveLength(2);
    expect(screen.getAllByText('Ctrl+Enter')).toHaveLength(1);
    expect(screen.queryByText('Ctrl+M')).toBeNull();
    expect(screen.getAllByText('ui.shortcuts.unavailable').length).toBeGreaterThan(0);
    expect(screen.getByText('ui.shortcuts.nativeGesturesNote')).toBeInTheDocument();
    expect(mocks.useCommandShortcutHints).toHaveBeenLastCalledWith('chat');
  });

  it('usa apenas o prefixo efetivo das sequências para o menu de nova aba', () => {
    mocks.useCommandSequencePrefixHint.mockReturnValue('Ctrl+N');
    render(<KeyboardShortcutsHelp isOpen={true} onClose={() => {}} surfaceType="navigation" />);
    expect(screen.getByText('Ctrl+N')).toBeInTheDocument();
    expect(screen.queryByText('Ctrl+N C')).toBeNull();
    expect(mocks.useCommandSequencePrefixHint).toHaveBeenCalledWith([
      'workspace.tab.chat.create', 'workspace.tab.editor.create',
      'workspace.tab.terminal.create', 'workspace.tab.tasklist.create',
    ], 'navigation');
  });

  it('representa carregamento ou mapa inválido sem inventar defaults', () => {
    mocks.setProjectedHints({});
    render(<KeyboardShortcutsHelp isOpen={true} onClose={() => {}} surfaceType="editor" />);
    expect(screen.queryByText('Ctrl+M')).toBeNull();
    expect(screen.queryByText('Alt+M')).toBeNull();
    expect(screen.getAllByText('ui.shortcuts.unavailable').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Ctrl+Enter')).toHaveLength(1);
    expect(mocks.useCommandShortcutHints).toHaveBeenLastCalledWith('editor');
  });

  it('registra-se no stack de modal compartilhado enquanto aberto', () => {
    expect(isModalOpen()).toBe(false);
    const { rerender } = render(<KeyboardShortcutsHelp isOpen={true} onClose={() => {}} />);
    expect(isModalOpen()).toBe(true);
    rerender(<KeyboardShortcutsHelp isOpen={false} onClose={() => {}} />);
    expect(isModalOpen()).toBe(false);
  });
});
