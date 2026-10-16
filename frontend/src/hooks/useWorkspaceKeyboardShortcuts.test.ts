import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook } from '@testing-library/react';

type MockTab = { id: string; type: 'chat' | 'editor' | 'terminal' | 'tasklist' };

const toggle = vi.fn();
const addTab = vi.fn(() => Promise.resolve());
// removeTab simula o backend: remove a aba e promove a sucessora na mesma
// posição (ou a anterior, se era a última), atualizando `activeTabId`.
const removeTab = vi.fn((tabId: string) => {
  const ws = workspaceState.workspace;
  const idx = ws.tabs.findIndex((tab) => tab.id === tabId);
  if (idx !== -1) ws.tabs.splice(idx, 1);
  if (ws.activeTabId === tabId) {
    const nextIdx = Math.min(idx, ws.tabs.length - 1);
    ws.activeTabId = nextIdx >= 0 ? ws.tabs[nextIdx].id : '';
  }
  return Promise.resolve();
});
const requestOpen = vi.fn(() => Promise.resolve());
const modalOpen = vi.fn(() => false);
const setActiveTab = vi.fn();
const createWorkspaceTab = vi.fn((_type: unknown, _title: unknown) => Promise.resolve('tab-new'));
const editorDocuments: Record<string, { readOnly?: boolean }> = {};

function canonicalWorkspace(): { tabs: MockTab[]; activeTabId: string } {
  return {
    tabs: [
      { id: 't1', type: 'editor' },
      { id: 't2', type: 'chat' },
    ],
    activeTabId: 't1',
  };
}

const workspaceState = {
  workspace: canonicalWorkspace(),
  addTab,
  removeTab,
  setActiveTab,
  createWorkspace: vi.fn(),
};

function resetWorkspaceState() {
  workspaceState.workspace = canonicalWorkspace();
}
const errorMocks = vi.hoisted(() => ({
  addToast: vi.fn(),
  logError: vi.fn(),
}));

vi.mock('zustand/shallow', () => ({
  useShallow: <T,>(fn: T) => fn,
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector?: (s: typeof workspaceState) => unknown) =>
      selector ? selector(workspaceState) : workspaceState,
    { getState: () => workspaceState },
  ),
}));

vi.mock('../store/editorStore', () => ({
  useEditorStore: { getState: () => ({ documents: editorDocuments }) },
}));

vi.mock('../store/workspaceChatModalStore', () => ({
  useWorkspaceChatModalStore: { getState: () => ({ requestOpen }) },
}));

vi.mock('../store/shortcutsHelpStore', () => ({
  useShortcutsHelpStore: { getState: () => ({ toggle }) },
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: { getState: () => ({ addToast: errorMocks.addToast }) },
}));

vi.mock('../utils/logger', () => ({
  logger: { error: errorMocks.logError },
}));

vi.mock('../components/ui/Modal', () => ({
  isModalOpen: () => modalOpen(),
}));

vi.mock('./useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: vi.fn() }),
}));

vi.mock('../lib/createWorkspaceTab', () => ({
  createWorkspaceTab: (type: unknown, title: unknown) => createWorkspaceTab(type, title),
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
    createWorkspaceTab.mockClear();
    errorMocks.addToast.mockClear();
    errorMocks.logError.mockClear();
    removeTab.mockClear();
    setActiveTab.mockClear();
    modalOpen.mockReturnValue(false);
    resetWorkspaceState();
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
    createWorkspaceTab.mockClear();
    errorMocks.addToast.mockClear();
    errorMocks.logError.mockClear();
    removeTab.mockClear();
    setActiveTab.mockClear();
    requestOpen.mockClear();
    delete editorDocuments.t1;
    modalOpen.mockReturnValue(false);
    resetWorkspaceState();
  });

  it('Ctrl+Shift+I apenas reserva o default do DevTools sem fallback legado', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ ctrlKey: true, shiftKey: true, key: 'I', code: 'KeyI' });

    expect(event.defaultPrevented).toBe(true);
    expect(requestOpen).not.toHaveBeenCalled();
  });

  it('reserva Ctrl+Shift+I mesmo quando o editor interrompe a propagação no filho', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());
    const editor = document.createElement('div');
    editor.addEventListener('keydown', (event) => event.stopPropagation());
    document.body.appendChild(editor);

    try {
      const event = new KeyboardEvent('keydown', {
        bubbles: true,
        cancelable: true,
        ctrlKey: true,
        shiftKey: true,
        key: 'I',
        code: 'KeyI',
      });
      editor.dispatchEvent(event);

      expect(event.defaultPrevented).toBe(true);
      expect(requestOpen).not.toHaveBeenCalled();
    } finally {
      editor.remove();
    }
  });

  it('Ctrl+Shift+I previne o default (DevTools) mas NÃO abre o chat modal com modal aberto', () => {
    modalOpen.mockReturnValue(true);
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ ctrlKey: true, shiftKey: true, key: 'I', code: 'KeyI' });

    expect(event.defaultPrevented).toBe(true);
    expect(requestOpen).not.toHaveBeenCalled();
  });

  it('Ctrl+Shift+I não abre chat para documento somente leitura', () => {
    editorDocuments.t1 = { readOnly: true };
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ ctrlKey: true, shiftKey: true, key: 'I', code: 'KeyI' });

    expect(event.defaultPrevented).toBe(true);
    expect(requestOpen).not.toHaveBeenCalled();
  });

  it('Ctrl+Shift+N não chama createWorkspace no hook legado', () => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ ctrlKey: true, shiftKey: true, key: 'N', code: 'KeyN' });

    expect(event.defaultPrevented).toBe(false);
    expect(workspaceState.createWorkspace).not.toHaveBeenCalled();
    expect(addTab).not.toHaveBeenCalled();
  });

  it.each([
    ['w', 'Ctrl+W'],
    ['F4', 'Ctrl+F4'],
  ])('%s não é mais interceptado pelo hook legado', (_key, _label) => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey({ ctrlKey: true, key: _key });

    expect(createWorkspaceTab).not.toHaveBeenCalled();
    expect(addTab).not.toHaveBeenCalled();
    expect(removeTab).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(false);
  });

  it('com um modal aberto (ex.: painel de atalhos), Ctrl+W não age na UI de fundo', () => {
    modalOpen.mockReturnValue(true);
    renderHook(() => useWorkspaceKeyboardShortcuts());

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

});

describe('useWorkspaceKeyboardShortcuts - navegação migrada para o Topbar local_ui', () => {
  it.each([
    ['Ctrl+Tab', { ctrlKey: true, key: 'Tab' }],
    ['Ctrl+Shift+Tab', { ctrlKey: true, shiftKey: true, key: 'Tab' }],
    ['Ctrl+PageDown', { ctrlKey: true, key: 'PageDown' }],
    ['Ctrl+PageUp', { ctrlKey: true, key: 'PageUp' }],
    ...Array.from({ length: 9 }, (_, index) => [`Ctrl+${index + 1}`, { ctrlKey: true, key: String(index + 1) }] as [string, KeyboardEventInit]),
  ])('%s não é interceptado pelo hook legado', (_label, init) => {
    renderHook(() => useWorkspaceKeyboardShortcuts());

    const event = dispatchKey(init);

    expect(event.defaultPrevented).toBe(false);
    expect(setActiveTab).not.toHaveBeenCalled();
  });
});
