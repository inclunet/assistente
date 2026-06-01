import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';

const toggle = vi.fn();
const addTab = vi.fn(() => Promise.resolve());
const removeTab = vi.fn(() => Promise.resolve());
const shortcutsState = { isOpen: false, toggle };

vi.mock('zustand/shallow', () => ({
  useShallow: <T,>(fn: T) => fn,
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: (selector: (s: unknown) => unknown) =>
    selector({
      workspace: { tabs: [{ id: 't1' }, { id: 't2' }], activeTabId: 't1' },
      addTab,
      removeTab,
      setActiveTab: vi.fn(),
      createWorkspace: vi.fn(),
    }),
}));

vi.mock('../store/workspaceChatModalStore', () => ({
  useWorkspaceChatModalStore: { getState: () => ({ requestOpen: vi.fn() }) },
}));

vi.mock('../store/shortcutsHelpStore', () => ({
  useShortcutsHelpStore: { getState: () => shortcutsState },
}));

vi.mock('../components/ui/Modal', () => ({
  isModalOpen: () => false,
}));

vi.mock('./useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: vi.fn() }),
}));

vi.mock('./useDefaultFocus', () => ({
  restoreDefaultFocus: vi.fn(),
}));

import { useWorkspaceKeyboardShortcuts } from './useWorkspaceKeyboardShortcuts';

function dispatchKey(init: KeyboardEventInit): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init });
  document.body.dispatchEvent(event);
  return event;
}

describe('useWorkspaceKeyboardShortcuts - atalho Ctrl+?', () => {
  beforeEach(() => {
    toggle.mockClear();
    addTab.mockClear();
    removeTab.mockClear();
    shortcutsState.isOpen = false;
  });

  it('abre o painel com Ctrl+? (caractere já reflete Shift)', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ ctrlKey: true, key: '?' });

    expect(toggle).toHaveBeenCalledTimes(1);
    expect(event.defaultPrevented).toBe(true);
  });

  it('abre o painel com Ctrl+Shift+/ (Slash com Shift)', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ ctrlKey: true, shiftKey: true, key: '/', code: 'Slash' });

    expect(toggle).toHaveBeenCalledTimes(1);
    expect(event.defaultPrevented).toBe(true);
  });

  it('NÃO intercepta Ctrl+/ puro (sem Shift) e não chama preventDefault', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ ctrlKey: true, key: '/', code: 'Slash' });

    expect(toggle).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(false);
  });
});

describe('useWorkspaceKeyboardShortcuts - painel aberto bloqueia atalhos de fundo', () => {
  beforeEach(() => {
    toggle.mockClear();
    addTab.mockClear();
    removeTab.mockClear();
    shortcutsState.isOpen = false;
  });

  it('com o painel fechado, Ctrl+T cria aba e Ctrl+W fecha aba', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    dispatchKey({ ctrlKey: true, key: 't' });
    dispatchKey({ ctrlKey: true, key: 'w' });

    expect(addTab).toHaveBeenCalledTimes(1);
    expect(removeTab).toHaveBeenCalledTimes(1);
  });

  it('com o painel ABERTO, Ctrl+T e Ctrl+W não agem na UI de fundo', () => {
    shortcutsState.isOpen = true;
    renderHook(() => useWorkspaceKeyboardShortcuts());

    dispatchKey({ ctrlKey: true, key: 't' });
    dispatchKey({ ctrlKey: true, key: 'w' });

    expect(addTab).not.toHaveBeenCalled();
    expect(removeTab).not.toHaveBeenCalled();
  });

  it('com o painel ABERTO, Ctrl+? ainda alterna (permite fechar)', () => {
    shortcutsState.isOpen = true;
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ ctrlKey: true, key: '?' });

    expect(toggle).toHaveBeenCalledTimes(1);
    expect(event.defaultPrevented).toBe(true);
  });

  it('ao fechar o painel, os atalhos de fundo voltam a funcionar', () => {
    shortcutsState.isOpen = true;
    const { rerender } = renderHook(() => useWorkspaceKeyboardShortcuts());

    dispatchKey({ ctrlKey: true, key: 't' });
    expect(addTab).not.toHaveBeenCalled();

    shortcutsState.isOpen = false;
    rerender();

    dispatchKey({ ctrlKey: true, key: 't' });
    expect(addTab).toHaveBeenCalledTimes(1);
  });
});
