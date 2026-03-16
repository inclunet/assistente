/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';

import { useEditorTabsKeyboardShortcuts } from './useEditorTabsKeyboardShortcuts';

const announceMock = vi.fn();
const createTabMock = vi.fn();
const closeTabMock = vi.fn();
const setActiveTabMock = vi.fn();

let editorState = {
  tabs: [
    { id: '1', title: 'Tab 1' },
    { id: '2', title: 'Tab 2' },
  ],
  activeTabId: '1',
  createTab: createTabMock,
  closeTab: closeTabMock,
  setActiveTab: setActiveTabMock,
};

vi.mock('./useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceMock }),
}));

vi.mock('../store/editorStore', () => ({
  useEditorStore: (selector: (state: typeof editorState) => unknown) => selector(editorState),
}));

vi.mock('../components/ui/Modal', () => ({
  isModalOpen: () => false,
}));

describe('useEditorTabsKeyboardShortcuts', () => {
  beforeEach(() => {
    createTabMock.mockClear();
    closeTabMock.mockClear();
    setActiveTabMock.mockClear();
    announceMock.mockClear();
  });

  afterEach(() => {
    editorState = {
      tabs: [
        { id: '1', title: 'Tab 1' },
        { id: '2', title: 'Tab 2' },
      ],
      activeTabId: '1',
      createTab: createTabMock,
      closeTab: closeTabMock,
      setActiveTab: setActiveTabMock,
    };
  });

  it('cria nova aba com Ctrl+T', () => {
    renderHook(() => useEditorTabsKeyboardShortcuts());

    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 't', ctrlKey: true }));
    });

    expect(createTabMock).toHaveBeenCalled();
    expect(announceMock).toHaveBeenCalled();
  });

  it('navega entre abas com Ctrl+Tab', () => {
    renderHook(() => useEditorTabsKeyboardShortcuts());

    act(() => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', ctrlKey: true }));
    });

    expect(setActiveTabMock).toHaveBeenCalledWith('2');
    expect(announceMock).toHaveBeenCalledWith('Tab 2, 2 de 2');
  });
});
