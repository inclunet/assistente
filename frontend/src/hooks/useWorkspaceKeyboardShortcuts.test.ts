import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';

const toggle = vi.fn();

vi.mock('zustand/shallow', () => ({
  useShallow: <T,>(fn: T) => fn,
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: (selector: (s: unknown) => unknown) =>
    selector({
      workspace: { tabs: [], activeTabId: null },
      addTab: vi.fn(),
      removeTab: vi.fn(),
      setActiveTab: vi.fn(),
      createWorkspace: vi.fn(),
    }),
}));

vi.mock('../store/workspaceChatModalStore', () => ({
  useWorkspaceChatModalStore: { getState: () => ({ requestOpen: vi.fn() }) },
}));

vi.mock('../store/shortcutsHelpStore', () => ({
  useShortcutsHelpStore: { getState: () => ({ toggle }) },
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
