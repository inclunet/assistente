import { describe, expect, it, vi } from 'vitest';
import { render } from '@testing-library/react';
import { useTabsKeyboardShortcuts } from './useTabsKeyboardShortcuts';

const createTabSpy = vi.fn();
const deleteTabSpy = vi.fn();
const setActiveTabSpy = vi.fn();
const announceSpy = vi.fn();

vi.mock('../store/chatStore', () => ({
  useChatStore: () => ({
    tabs: [{ id: '1', title: 'A' }, { id: '2', title: 'B' }],
    activeTabId: '1',
    createTab: createTabSpy,
    deleteTab: deleteTabSpy,
    setActiveTab: setActiveTabSpy,
  }),
}));

vi.mock('./useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceSpy }),
}));

function Fixture() {
  useTabsKeyboardShortcuts();
  return null;
}

describe('useTabsKeyboardShortcuts', () => {
  it('cria nova aba com Ctrl+N', () => {
    render(<Fixture />);

    const target = document.createElement('div');
    document.body.appendChild(target);

    target.dispatchEvent(new KeyboardEvent('keydown', { key: 'n', ctrlKey: true, bubbles: true }));
    expect(createTabSpy).toHaveBeenCalled();
  });

  it('fecha aba com Ctrl+W', () => {
    render(<Fixture />);

    const target = document.createElement('div');
    document.body.appendChild(target);

    target.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', ctrlKey: true, bubbles: true }));
    expect(deleteTabSpy).toHaveBeenCalledWith('1');
  });
});
