import { afterEach, describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { restoreDefaultFocus } from './useDefaultFocus';

const toggle = vi.fn();
const addTab = vi.fn(() => Promise.resolve());
const removeTab = vi.fn(() => Promise.resolve());
const requestOpen = vi.fn(() => Promise.resolve());
const modalOpen = vi.fn(() => false);
const setActiveTab = vi.fn();

vi.mock('zustand/shallow', () => ({
  useShallow: <T,>(fn: T) => fn,
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: (selector: (s: unknown) => unknown) =>
    selector({
      workspace: { tabs: [{ id: 't1' }, { id: 't2' }], activeTabId: 't1' },
      addTab,
      removeTab,
      setActiveTab,
      createWorkspace: vi.fn(),
    }),
}));

vi.mock('../store/workspaceChatModalStore', () => ({
  useWorkspaceChatModalStore: { getState: () => ({ requestOpen }) },
}));

vi.mock('../store/shortcutsHelpStore', () => ({
  useShortcutsHelpStore: { getState: () => ({ toggle }) },
}));

vi.mock('../components/ui/Modal', () => ({
  isModalOpen: () => modalOpen(),
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
    setActiveTab.mockClear();
    modalOpen.mockReturnValue(false);
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

describe('useWorkspaceKeyboardShortcuts - respeita isModalOpen()', () => {
  beforeEach(() => {
    toggle.mockClear();
    addTab.mockClear();
    removeTab.mockClear();
    setActiveTab.mockClear();
    requestOpen.mockClear();
    vi.mocked(restoreDefaultFocus).mockClear();
    modalOpen.mockReturnValue(false);
  });

  it('Ctrl+Shift+I abre o chat modal quando nenhum modal está aberto', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ ctrlKey: true, shiftKey: true, key: 'I', code: 'KeyI' });

    expect(event.defaultPrevented).toBe(true);
    expect(requestOpen).toHaveBeenCalledTimes(1);
  });

  it('Ctrl+Shift+I previne o default (DevTools) mas NÃO abre o chat modal com modal aberto', () => {
    modalOpen.mockReturnValue(true);
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ ctrlKey: true, shiftKey: true, key: 'I', code: 'KeyI' });

    expect(event.defaultPrevented).toBe(true);
    expect(requestOpen).not.toHaveBeenCalled();
  });

  it('com nenhum modal aberto, Ctrl+T cria aba e Ctrl+W fecha aba', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    dispatchKey({ ctrlKey: true, key: 't' });
    dispatchKey({ ctrlKey: true, key: 'w' });

    expect(addTab).toHaveBeenCalledTimes(1);
    expect(removeTab).toHaveBeenCalledTimes(1);
  });

  it('com um modal aberto (ex.: painel de atalhos), Ctrl+T e Ctrl+W não agem na UI de fundo', () => {
    modalOpen.mockReturnValue(true);
    renderHook(() => useWorkspaceKeyboardShortcuts());

    dispatchKey({ ctrlKey: true, key: 't' });
    dispatchKey({ ctrlKey: true, key: 'w' });

    expect(addTab).not.toHaveBeenCalled();
    expect(removeTab).not.toHaveBeenCalled();
  });

  it('com um modal aberto, Ctrl+? ainda alterna (permite fechar o painel)', () => {
    modalOpen.mockReturnValue(true);
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ ctrlKey: true, key: '?' });

    expect(toggle).toHaveBeenCalledTimes(1);
    expect(event.defaultPrevented).toBe(true);
  });

  it('ao fechar o modal, os atalhos de fundo voltam a funcionar', () => {
    modalOpen.mockReturnValue(true);
    renderHook(() => useWorkspaceKeyboardShortcuts());

    dispatchKey({ ctrlKey: true, key: 't' });
    expect(addTab).not.toHaveBeenCalled();

    modalOpen.mockReturnValue(false);
    dispatchKey({ ctrlKey: true, key: 't' });
    expect(addTab).toHaveBeenCalledTimes(1);
  });
});

describe('useWorkspaceKeyboardShortcuts - foco apos troca global de aba', () => {
  let originalRequestAnimationFrame: typeof window.requestAnimationFrame | undefined;

  beforeEach(async () => {
    setActiveTab.mockClear();
    modalOpen.mockReturnValue(false);
    originalRequestAnimationFrame = window.requestAnimationFrame;
    window.requestAnimationFrame = ((callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    }) as typeof window.requestAnimationFrame;
    await new Promise((resolve) => setTimeout(resolve, 0));
    vi.mocked(restoreDefaultFocus).mockClear();
  });

  afterEach(() => {
    if (originalRequestAnimationFrame) {
      window.requestAnimationFrame = originalRequestAnimationFrame;
    } else {
      Reflect.deleteProperty(window, 'requestAnimationFrame');
    }
  });

  it.each([
    ['Ctrl+Tab', { ctrlKey: true, key: 'Tab' }],
    ['Ctrl+Shift+Tab', { ctrlKey: true, shiftKey: true, key: 'Tab' }],
    ['Ctrl+PageDown', { ctrlKey: true, key: 'PageDown' }],
    ['Ctrl+PageUp', { ctrlKey: true, key: 'PageUp' }],
  ])('%s troca aba e restaura a area default', async (_label, init) => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey(init);

    expect(event.defaultPrevented).toBe(true);
    expect(setActiveTab).toHaveBeenCalledWith('t2');
    await vi.waitFor(() => expect(restoreDefaultFocus).toHaveBeenCalled());
  });

  it('Ctrl+numero troca diretamente sem restaurar a area default', async () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    dispatchKey({ ctrlKey: true, key: '2' });

    expect(setActiveTab).toHaveBeenCalledWith('t2');
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(restoreDefaultFocus).not.toHaveBeenCalled();
  });
});
