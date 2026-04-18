import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const openToolbarMenuSpy = vi.fn();

const editorStoreState = {
  documents: {} as Record<string, { id: string; title: string; markdown: string; mode: string }>,
  activeDocumentId: null as string | null,
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
  getActiveDocument: vi.fn(),
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
    }),
    { getState: () => ({ workspace: { tabs: [] }, addTab: vi.fn(), getActiveTab: () => undefined }), subscribe: () => () => {} }
  ),
}));

vi.mock('../store/chatStore', () => ({
  useChatStore: Object.assign(
    (selector?: (s: Record<string, unknown>) => unknown) => {
      const state: Record<string, unknown> = {
        sendMessage: vi.fn(),
        isLoading: false,
        activeConversationId: null,
        createConversation: vi.fn(),
        getMessages: () => [],
      };
      return typeof selector === 'function' ? selector(state) : state;
    },
    { getState: () => ({ sendMessage: vi.fn(), activeConversationId: null, createConversation: vi.fn(), getMessages: () => [] }) },
  ),
}));

vi.mock('../store/questionnaireUIStore', () => ({
  useQuestionnaireUIStore: () => ({
    request: vi.fn(),
  }),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: () => ({
    addToast: vi.fn(),
  }),
}));

vi.mock('../hooks/useEditorTabsKeyboardShortcuts', () => ({
  useEditorTabsKeyboardShortcuts: () => {},
}));

vi.mock('../hooks/useEditorInlineChatPatch', () => ({
  useEditorInlineChatPatch: () => ({
    waitForChatDone: vi.fn(),
    waitForEditorPatch: vi.fn(),
    getMaxNumericMessageId: vi.fn(),
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

vi.mock('../components/editor/RichTextEditor', () => ({
  RichTextEditor: () => <div>Rich</div>,
}));

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

vi.mock('@wailsjs/go/main/App', () => ({
  EditorDeleteDraft: vi.fn(),
  EditorGetFileInfo: vi.fn(),
  GetProfile: vi.fn(),
  EditorLoadSession: vi.fn(),
  EditorOpenFile: vi.fn(),
  EditorReadDraft: vi.fn(),
  EditorReadFile: vi.fn(),
  EditorSaveFileDialog: vi.fn(),
  EditorSaveSession: vi.fn(),
  EditorUnwatchFile: vi.fn(),
  EditorWatchFile: vi.fn(),
  EditorWriteDraft: vi.fn(),
  EditorWriteFile: vi.fn(),
}));

import EditorPage from './EditorPage';

describe('EditorPage', () => {
  beforeEach(() => {
    editorStoreState.documents = {};
    editorStoreState.activeDocumentId = null;
    openToolbarMenuSpy.mockReset();
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
    editorStoreState.activeDocumentId = 'tab-1';

    render(<EditorPage />);

    expect(screen.getByRole('button', { name: 'editor.buttons.insert' })).toBeDisabled();
  });
});
