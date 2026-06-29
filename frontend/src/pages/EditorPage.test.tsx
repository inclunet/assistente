import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { act, fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { EditorWriteFile } from '@wailsjs/go/app/App';

const openToolbarMenuSpy = vi.fn();
const editorPageMocks = vi.hoisted(() => {
  const revealSlideMarkdown = '## Slide 2\nselected rich text';
  const offsetToPosition = (text: string, offset: number) => {
    const safeOffset = Math.max(0, Math.min(offset, text.length));
    const before = text.slice(0, safeOffset);
    const lines = before.split('\n');
    return { lineNumber: lines.length, column: lines[lines.length - 1].length + 1 };
  };
  const positionToOffset = (text: string, position: { lineNumber: number; column: number }) => {
    const lines = text.split('\n');
    let offset = 0;
    for (let i = 0; i < Math.max(0, position.lineNumber - 1); i += 1) {
      offset += (lines[i]?.length ?? 0) + 1;
    }
    return offset + Math.max(0, position.column - 1);
  };
  const state = {
    registeredAdapter: null as unknown,
    editorContentAreaProps: null as Record<string, unknown> | null,
    initialRevealSlideIndex: 0,
    markdownModelValue: '',
    markdownSelectionStartOffset: 0,
    markdownSelectionEndOffset: 0,
    markdownCursorOffset: 0,
    markdownEditor: null as unknown,
    richEditor: null as unknown,
  };
  state.markdownEditor = {
    getModel: () => ({
      getValue: () => state.markdownModelValue,
      getValueInRange: () => state.markdownModelValue.slice(state.markdownSelectionStartOffset, state.markdownSelectionEndOffset),
      getOffsetAt: (position: { lineNumber: number; column: number }) => positionToOffset(state.markdownModelValue, position),
    }),
    getSelection: () => ({
      getStartPosition: () => offsetToPosition(state.markdownModelValue, state.markdownSelectionStartOffset),
      getEndPosition: () => offsetToPosition(state.markdownModelValue, state.markdownSelectionEndOffset),
    }),
    getPosition: () => offsetToPosition(state.markdownModelValue, state.markdownCursorOffset),
    focus: vi.fn(),
  };
  state.richEditor = {
    state: {
      selection: {
        from: 1,
        to: 19,
        empty: false,
      },
      doc: {
        content: { size: revealSlideMarkdown.length },
        textBetween: (from: number, to: number) => {
          if (from === 1 && to === 19) return 'selected rich text';
          return revealSlideMarkdown;
        },
        cut: () => ({ markdown: revealSlideMarkdown }),
      },
    },
    storage: {
      markdown: {
        serializer: {
          serialize: (node: { markdown?: string }) => node.markdown || '',
        },
        getMarkdown: () => revealSlideMarkdown,
      },
    },
    commands: { focus: vi.fn() },
    view: { focus: vi.fn() },
  };
  return state;
});

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

vi.mock('../components/editor/EditorContentArea', async () => {
  const React = await import('react');
  return {
    EditorContentArea: (props: Record<string, unknown>) => {
      editorPageMocks.editorContentAreaProps = props;
      React.useEffect(() => {
        (props.onMonacoMount as ((editor: unknown, monaco: unknown) => void) | undefined)?.(editorPageMocks.markdownEditor, {});
        (props.onRichEditorReady as ((editor: unknown) => void) | undefined)?.(editorPageMocks.richEditor);
        (props.onRevealSlideIndexChange as ((index: number) => void) | undefined)?.(editorPageMocks.initialRevealSlideIndex);
        return () => {
          (props.onRichEditorReady as ((editor: unknown) => void) | undefined)?.(null);
        };
      }, []);
      return <div>Content</div>;
    },
  };
});

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
  useRegisterWorkspaceChatAdapter: vi.fn((_tabId: string | undefined, adapter: unknown) => {
    editorPageMocks.registeredAdapter = adapter;
  }),
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
    editorPageMocks.registeredAdapter = null;
    editorPageMocks.editorContentAreaProps = null;
    editorPageMocks.initialRevealSlideIndex = 0;
    editorPageMocks.markdownModelValue = '';
    editorPageMocks.markdownSelectionStartOffset = 0;
    editorPageMocks.markdownSelectionEndOffset = 0;
    editorPageMocks.markdownCursorOffset = 0;
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

  it('mantém o slide Reveal rico capturado no prepare ao enviar', async () => {
    editorPageMocks.initialRevealSlideIndex = 1;
    editorStoreState.documents = {
      'tab-1': {
        id: 'tab-1',
        title: 'Deck',
        markdown: '# Slide 1\n\n---\n\n## Slide 2\nselected rich text\n\n---\n\n## Slide 3\noutro slide',
        mode: 'rich',
        filePath: 'deck.md',
      },
    };

    render(
      <EditorPage
        workspaceTab={{
          id: 'tab-1',
          type: 'editor',
          title: 'Deck',
          position: 0,
          conversationId: 'conv-1',
          state: { filePath: 'deck.md' },
        }}
      />
    );

    const adapter = editorPageMocks.registeredAdapter as {
      prepare: () => Promise<{ ok: true; meta: unknown }>;
      send: (
        instruction: string,
        media: undefined,
        meta: unknown,
        session: { tabId: string; conversationId: string },
      ) => Promise<{ paramsOverride?: { surfaceContextJson?: string } } | null>;
    };
    const prepared = await adapter.prepare();
    const selection = prepared.meta as { revealSlideIndex?: number; revealSlideMarkdown?: string };

    expect(selection.revealSlideIndex).toBe(1);
    expect(selection.revealSlideMarkdown).toContain('Slide 2');

    act(() => {
      (editorPageMocks.editorContentAreaProps?.onRevealSlideIndexChange as (index: number) => void)(2);
    });
    const plan = await act(async () => {
      return adapter.send('Explique este trecho', undefined, selection, {
        tabId: 'tab-1',
        conversationId: 'conv-1',
      });
    });
    const surfaceContext = JSON.parse(String(plan?.paramsOverride?.surfaceContextJson || '{}'));

    expect(surfaceContext.metadata.currentSlideIndex).toBe(1);
    expect(surfaceContext.focus.entity.slideIndex).toBe(1);
    expect(surfaceContext.content.markdown).toContain('Slide 2');
    expect(surfaceContext.content.markdown).not.toContain('Slide 3');
  });

  it('mantém o slide Reveal Markdown capturado no prepare ao enviar', async () => {
    const initialDeck = '# Slide 1\n\n---\n\n## Slide 2\nselected markdown text\n\n---\n\n## Slide 3\noutro slide';
    const selectedText = 'selected markdown text';
    const selectionStart = initialDeck.indexOf(selectedText);
    editorPageMocks.markdownModelValue = initialDeck;
    editorPageMocks.markdownSelectionStartOffset = selectionStart;
    editorPageMocks.markdownSelectionEndOffset = selectionStart + selectedText.length;
    editorPageMocks.markdownCursorOffset = selectionStart;
    editorPageMocks.initialRevealSlideIndex = 1;
    editorStoreState.documents = {
      'tab-1': {
        id: 'tab-1',
        title: 'Deck',
        markdown: initialDeck,
        mode: 'markdown',
        filePath: 'deck.md',
      },
    };

    render(
      <EditorPage
        workspaceTab={{
          id: 'tab-1',
          type: 'editor',
          title: 'Deck',
          position: 0,
          conversationId: 'conv-1',
          state: { filePath: 'deck.md' },
        }}
      />
    );

    const adapter = editorPageMocks.registeredAdapter as {
      prepare: () => Promise<{ ok: true; meta: unknown }>;
      send: (
        instruction: string,
        media: undefined,
        meta: unknown,
        session: { tabId: string; conversationId: string },
      ) => Promise<{ paramsOverride?: { surfaceContextJson?: string } } | null>;
    };
    const prepared = await adapter.prepare();
    const selection = prepared.meta as { revealSlideIndex?: number; revealSlideMarkdown?: string };

    expect(selection.revealSlideIndex).toBe(1);
    expect(selection.revealSlideMarkdown).toContain('Slide 2');

    act(() => {
      editorStoreState.documents['tab-1'].markdown = '# Slide 1\n\n---\n\n## Slide 3\noutro slide alterado';
      (editorPageMocks.editorContentAreaProps?.onRevealSlideIndexChange as (index: number) => void)(2);
    });
    const plan = await act(async () => {
      return adapter.send('Explique este trecho', undefined, selection, {
        tabId: 'tab-1',
        conversationId: 'conv-1',
      });
    });
    const surfaceContext = JSON.parse(String(plan?.paramsOverride?.surfaceContextJson || '{}'));

    expect(surfaceContext.metadata.currentSlideIndex).toBe(1);
    expect(surfaceContext.focus.entity.slideIndex).toBe(1);
    expect(surfaceContext.content.markdown).toContain('Slide 2');
    expect(surfaceContext.content.markdown).not.toContain('Slide 3');
  });
});
