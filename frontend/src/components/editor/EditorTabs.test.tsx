import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { EditorTabs } from './EditorTabs';

const closeTabSpy = vi.fn();
const renameTabSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@wailsjs/go/main/App', () => ({
  EditorRenameFile: vi.fn(),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: vi.fn() }),
}));

vi.mock('../../store/uiStore', () => ({
  useUIStore: () => ({ addToast: vi.fn() }),
}));

type EditorStoreState = {
  tabs: Array<{ id: string; title: string; isDirty: boolean }>;
  activeTabId: string | null;
  setActiveTab: (id: string) => void;
  closeTab: (id: string) => void;
  renameTab: (id: string, title: string) => void;
  setTabFilePath: (id: string, path: string | null) => void;
};

vi.mock('../../store/editorStore', () => ({
  useEditorStore: (selector: (state: EditorStoreState) => unknown) => selector({
    tabs: [
      { id: 'a', title: 'Doc A', isDirty: false },
      { id: 'b', title: 'Doc B', isDirty: false },
    ],
    activeTabId: 'a',
    setActiveTab: vi.fn(),
    closeTab: closeTabSpy,
    renameTab: renameTabSpy,
    setTabFilePath: vi.fn(),
  }),
}));

vi.mock('../ui/tabs', () => ({
  Tabs: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  TabList: ({ children, listRef, onKeyDown }: { children: React.ReactNode; listRef?: React.Ref<HTMLDivElement>; onKeyDown?: (e: React.KeyboardEvent<HTMLDivElement>) => void }) => (
    <div role="tablist" ref={listRef} onKeyDown={onKeyDown}>{children}</div>
  ),
  Tab: ({ children, value, onClick, onDoubleClick }: { children: React.ReactNode; value: string; onClick?: () => void; onDoubleClick?: () => void }) => (
    <button role="tab" data-tab-value={value} onClick={onClick} onDoubleClick={onDoubleClick}>{children}</button>
  ),
}));

describe('EditorTabs', () => {
  it('renderiza tabs e fecha ao clicar no X', () => {
    render(<EditorTabs />);

    fireEvent.click(screen.getByRole('button', { name: 'editor.tabs.close Doc A' }));
    expect(closeTabSpy).toHaveBeenCalledWith('a');
  });

  it('entra em modo de edicao ao double click', () => {
    render(<EditorTabs />);

    fireEvent.doubleClick(screen.getByRole('tab', { name: 'Doc A' }));
    expect(screen.getByRole('textbox', { name: 'editor.tabs.editTitle' })).toBeInTheDocument();
  });
});
