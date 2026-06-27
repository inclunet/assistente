import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { EditorWriteFile } from '@wailsjs/go/app/App';

const openToolbarMenuSpy = vi.fn();

const editorStoreState = {
  documents: {} as Record<string, { id: string; title: string; markdown: string; mode: string; filePath?: string | null; draftId?: string | null }>,
  autoSaveEnabled: true,
  editorProfileSlug: 'editor-texto',
  createDocument: vi.fn(),
  setDocMarkdown: vi.fn(),
  renameDocument: vi.fn(),
  setDocFilePath: vi.fn(),
  setDocDraftId: vi.fn(),
  setDocDirty: vi.fn(),
  toggleAutoSave: vi.fn(),
  setEditorProfileSlug: vi.fn(),
  hydrate: vi.fn(),
  setDocMode: vi.fn(),
  consumePendingInsert: vi.fn().mockReturnValue(null),
  getDocument: vi.fn(),
  removeDocument: vi.fn(),
};

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

vi.mock('../store/editorStore', () => ({
  useEditorStore: Object.assign(
    (selector: (state: typeof editorStoreState) => unknown) => selector(editorStoreState),
    { getState: () => editorStoreState, subscribe: () => () => {} }
  ),
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector: (state: Record<string, unknown>) => unknown) => selector({
      addTab: vi.fn(),
      updateTab: vi.fn(),
      workspace: { tabs: [], profile: undefined },
      getActiveTab: () => undefined,
      isInitialized: true,
    }),
    { getState: () => ({ workspace: { tabs: [] }, addTab: vi.fn(), getActiveTab: () => undefined }), subscribe: () => () => {} }
  ),
  useActiveTab: () => undefined,
}));

vi.mock('../store/chatStore', () => ({
  useChatStore: Object.assign(
    (selector?: (s: Record<string, unknown>) => unknown) => {
      const state: Record<string, unknown> = {
        createConversation: vi.fn(),
        getConversationMessages: () => [],
      };
      return typeof selector === 'function' ? selector(state) : state;
    },
    { getState: () => ({ createConversation: vi.fn(), getConversationMessages: () => [] }) },
  ),
}));

vi.mock('../store/questionnaireUIStore', () => ({
  useQuestionnaireUIStore: () => ({
    request: vi.fn(),
  }),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { addToast: vi.fn() };
    return selector ? selector(s) : s;
  },
}));

vi.mock('../hooks/useEditorTabsKeyboardShortcuts', () => ({
  useEditorTabsKeyboardShortcuts: () => {},
}));

vi.mock('../hooks/useEditorInlineChatPatch', () => ({
  useEditorInlineChatPatch: () => ({
    waitForChatDone: vi.fn(),
    waitForEditorPatch: vi.fn(),
    getMaxMessageId: vi.fn(),
  }),
}));

vi.mock('./useRichEditorFlushEvents', () => ({
  useRichEditorFlushEvents: () => {},
}));

vi.mock('../hooks/useDebouncedValue', () => ({
  useDebouncedValue: (value: unknown) => value,
}));

vi.mock('../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({
    menu: { visible: false, x: 0, y: 0, items: [], ariaLabel: 'Menu' },
    openForTrigger: openToolbarMenuSpy,
    closeMenu: vi.fn(),
    onSelectItem: vi.fn(),
  }),
}));

vi.mock('../components/ui/Toolbar', async () => {
  const React = await import('react');
  return {
    Toolbar: ({ left, right, actions }: { left?: ReactNode; right?: ReactNode; actions?: Array<{ key: string; label: string; onClick?: () => void; disabled?: boolean }> }) => (
      <div>
        {left}
        {right}
        {actions?.map((action) => (
          <button key={action.key} onClick={action.onClick} disabled={action.disabled}>
            {action.label}
          </button>
        ))}
      </div>
    ),
    ToolbarButton: React.forwardRef<HTMLButtonElement, { label: string; onClick?: (event: React.MouseEvent<HTMLButtonElement>) => void; disabled?: boolean }>(
      ({ label, onClick, disabled }, ref) => (
        <button ref={ref} type="button" onClick={onClick} disabled={disabled}>
          {label}
        </button>
      )
    ),
  };
});

vi.mock('../components/editor/EditorTabs', () => ({
  EditorTabs: () => <div>Tabs</div>,
}));

vi.mock('../components/pickers/ProfilePicker', () => ({
  ProfilePicker: ({ label }: { label: string }) => <div>{label}</div>,
}));

vi.mock('../components/ui/CodeEditor', () => ({
  CodeEditor: () => <div>Editor</div>,
}));

vi.mock('../components/editor/RichTextEditor', async () => {
  const React = await import('react');
  return {
    RichTextEditor: React.forwardRef<HTMLDivElement>(() => <div>Rich</div>),
  };
});

vi.mock('../components/ui/MarkdownRenderer', () => ({
  MarkdownRenderer: () => <div>Preview</div>,
}));

vi.mock('../components/editor/MermaidEditorModal', () => ({
  MermaidEditorModal: () => null,
}));

vi.mock('../store/workspaceChatModalStore', () => ({
  useWorkspaceChatModalStore: Object.assign(
    (selector?: (s: { isOpen: boolean }) => unknown) => {
      const state = { isOpen: false };
      return typeof selector === 'function' ? selector(state) : state;
    },
    { getState: () => ({ requestOpen: vi.fn(), close: vi.fn(), setAdapterError: vi.fn(), bumpFocus: vi.fn() }) },
  ),
}));

vi.mock('../hooks/useRegisterWorkspaceChatAdapter', () => ({
  useRegisterWorkspaceChatAdapter: vi.fn(),
}));

vi.mock('../components/menu', () => ({
  Menu: () => null,
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: () => () => {},
}));

vi.mock('@wailsjs/go/app/App', () => ({
  EditorDeleteDraft: vi.fn(),
  EditorGetFileInfo: vi.fn(),
  GetProfile: vi.fn(),
  EditorLoadSession: vi.fn(),
  EditorOpenFile: vi.fn(),
  EditorReadDraft: vi.fn(),
  EditorReadFile: vi.fn(),
  EditorSaveFileDialog: vi.fn(),
  EditorSaveSession: vi.fn(),
  EditorUnwatchFile: vi.fn().mockResolvedValue(undefined),
  EditorWatchFile: vi.fn().mockResolvedValue(undefined),
  EditorWriteDraft: vi.fn(),
  EditorWriteFile: vi.fn(),
}));

import EditorPage from './EditorPage';

describe('EditorPage', () => {
  beforeEach(() => {
    editorStoreState.documents = {};
    openToolbarMenuSpy.mockReset();
    editorStoreState.setDocMode.mockReset();
    vi.mocked(EditorWriteFile).mockReset();
  });

  it('desabilita botoes de formato/inserir/modo sem aba ativa', () => {
    render(<EditorPage />);

    expect(screen.getByRole('button', { name: 'editor.buttons.format' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'editor.buttons.insert' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'editor.buttons.mode' })).toBeDisabled();
  });

  it('abre menu de arquivo ao clicar no botao', async () => {
    const user = userEvent.setup();
    render(<EditorPage />);

    const fileButton = screen.getByRole('button', { name: 'editor.buttons.file' });
    await user.click(fileButton);

    expect(openToolbarMenuSpy).toHaveBeenCalled();
  });

  it('desabilita inserir quando modo view', () => {
    editorStoreState.documents = {
      'tab-1': { id: 'tab-1', title: 'Doc', markdown: 'text', mode: 'view' },
    };

    render(<EditorPage documentId="tab-1" />);

    expect(screen.getByRole('button', { name: 'editor.buttons.insert' })).toBeDisabled();
  });

  it('abre menu Inserir com Alt+I quando disponível', () => {
    editorStoreState.documents = {
      'tab-1': { id: 'tab-1', title: 'Doc', markdown: 'text', mode: 'markdown' },
    };

    render(<EditorPage documentId="tab-1" />);

    const insertButton = screen.getByRole('button', { name: 'editor.buttons.insert' });
    fireEvent.keyDown(window, { key: 'i', altKey: true });

    expect(openToolbarMenuSpy).toHaveBeenCalledWith(
      insertButton,
      'editor.aria.insertMenu',
      expect.any(Array)
    );
  });

  it('alterna modos principais com Alt+1, Alt+2 e Alt+3', () => {
    editorStoreState.documents = {
      'tab-1': { id: 'tab-1', title: 'Doc', markdown: 'text', mode: 'rich' },
    };

    render(<EditorPage documentId="tab-1" />);

    fireEvent.keyDown(window, { key: '1', altKey: true });
    fireEvent.keyDown(window, { key: '2', altKey: true });
    fireEvent.keyDown(window, { key: '3', altKey: true });

    expect(editorStoreState.setDocMode).toHaveBeenNthCalledWith(1, 'tab-1', 'markdown');
    expect(editorStoreState.setDocMode).toHaveBeenNthCalledWith(2, 'tab-1', 'rich');
    expect(editorStoreState.setDocMode).toHaveBeenNthCalledWith(3, 'tab-1', 'view');
  });

  it('não executa atalhos de arquivo quando o painel está inativo', async () => {
    const user = userEvent.setup();
    editorStoreState.documents = {
      'tab-1': {
        id: 'tab-1',
        title: 'Doc',
        markdown: 'text',
        mode: 'markdown',
        filePath: 'doc.md',
      },
    };

    render(<EditorPage documentId="tab-1" isPanelActive={false} />);

    await user.keyboard('{Control>}s{/Control}');

    expect(EditorWriteFile).not.toHaveBeenCalled();
  });
});
