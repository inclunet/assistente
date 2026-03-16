import { describe, expect, it, vi } from 'vitest';
import { render } from '@testing-library/react';
import { useDocumentTitle } from './useDocumentTitle';

const setTitleSpy = vi.fn();

vi.mock('react-router-dom', () => ({
  useLocation: () => ({ pathname: '/' }),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  WindowSetTitle: (title: string) => setTitleSpy(title),
}));

type ChatStoreState = {
  tabs: Array<{ id: string; title: string }>;
  activeTabId: string | null;
};

vi.mock('../store/chatStore', () => ({
  useChatStore: (selector: (state: ChatStoreState) => unknown) => selector({
    tabs: [{ id: '1', title: 'Conversa A' }],
    activeTabId: '1',
  }),
}));

function Fixture() {
  useDocumentTitle();
  return null;
}

describe('useDocumentTitle', () => {
  it('define titulo para chat ativo', () => {
    render(<Fixture />);

    expect(document.title).toBe('Conversa A - Assistente IA');
    expect(setTitleSpy).toHaveBeenCalledWith('Conversa A - Assistente IA');
  });
});
