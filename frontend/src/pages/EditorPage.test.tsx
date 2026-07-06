import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { act, fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { EditorGetFileInfo, EditorReadFile, EditorWriteFile, GetProfile } from '@wailsjs/go/app/App';

const openToolbarMenuSpy = vi.fn();
const editorPageMocks = vi.hoisted(() => {
  const revealSlideMarkdown = '## Slide 2\nselected rich text';
  const runtimeHandlers: Record<string, Array<(data: unknown) => void>> = {};
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
    markdownHasFocus: true,
    markdownSelectionListener: null as (() => void) | null,
    richSelectionListener: null as (() => void) | null,
    markdownEditor: null as unknown,
    richEditor: null as unknown,
    requestOpen: vi.fn(),
    closeModal: vi.fn(),
    setAdapterError: vi.fn(),
    bumpFocus: vi.fn(),
    waitForChatDone: vi.fn(),
    waitForEditorPatch: vi.fn(),
    getMaxMessageId: vi.fn(),
    runtimeHandlers,
    emitRuntimeEvent: (name: string, data: unknown) => {
      for (const handler of [...(runtimeHandlers[name] ?? [])]) {
        handler(data);
      }
    },
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
    hasTextFocus: () => state.markdownHasFocus,
    onDidChangeCursorSelection: (listener: () => void) => {
      state.markdownSelectionListener = listener;
      return {
        dispose: () => {
          if (state.markdownSelectionListener === listener) state.markdownSelectionListener = null;
        },
      };
    },
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
    view: { focus: vi.fn(), hasFocus: () => true },
    isFocused: true,
    on: (event: string, listener: () => void) => {
      if (event === 'selectionUpdate') state.richSelectionListener = listener;
    },
    off: (event: string, listener: () => void) => {
      if (event === 'selectionUpdate' && state.richSelectionListener === listener) {
        state.richSelectionListener = null;
      }
    },
  };
  return state;
});

const editorStoreState = {
  documents: {} as Record<
    string,
    { id: string; title: string; markdown: string; mode: string; filePath?: string | null; draftId?: string | null; isDirty?: boolean }
  >,
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
  useQuestionnaireUIStore: (selector?: (s: { request: () => void }) => unknown) => {
    const state = { request: vi.fn() };
    return selector ? selector(state) : state;
  },
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
    waitForChatDone: editorPageMocks.waitForChatDone,
    waitForEditorPatch: editorPageMocks.waitForEditorPatch,
    getMaxMessageId: editorPageMocks.getMaxMessageId,
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
    Toolbar: ({ left, right, actions }: { left?: ReactNode; right?: ReactNode; actions?: Array<{ key: string; label: string; onClick?: () => void; onMouseDown?: () => void; disabled?: boolean }> }) => (
      <div>
        {left}
        {right}
        {actions?.map((action) => (
          <button key={action.key} onClick={action.onClick} onMouseDown={action.onMouseDown} disabled={action.disabled}>
            {action.label}
          </button>
        ))}
      </div>
    ),
    ToolbarButton: React.forwardRef<HTMLButtonElement, { label: string; onClick?: (event: React.MouseEvent<HTMLButtonElement>) => void; onMouseDown?: (event: React.MouseEvent<HTMLButtonElement>) => void; disabled?: boolean }>(
      ({ label, onClick, onMouseDown, disabled }, ref) => (
        <button ref={ref} type="button" onClick={onClick} onMouseDown={onMouseDown} disabled={disabled}>
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
    {
      getState: () => ({
        isOpen: true,
        requestOpen: editorPageMocks.requestOpen,
        close: editorPageMocks.closeModal,
        setAdapterError: editorPageMocks.setAdapterError,
        bumpFocus: editorPageMocks.bumpFocus,
      }),
    },
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
  EventsOn: (name: string, handler: (data: unknown) => void) => {
    const handlers = editorPageMocks.runtimeHandlers[name] ?? [];
    handlers.push(handler);
    editorPageMocks.runtimeHandlers[name] = handlers;
    return () => {
      const current = editorPageMocks.runtimeHandlers[name] ?? [];
      editorPageMocks.runtimeHandlers[name] = current.filter((item) => item !== handler);
    };
  },
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
  EditorSaveState: vi.fn().mockResolvedValue(undefined),
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
    editorPageMocks.markdownHasFocus = true;
    editorPageMocks.markdownSelectionListener = null;
    editorPageMocks.richSelectionListener = null;
    editorPageMocks.requestOpen.mockReset();
    editorPageMocks.closeModal.mockReset();
    editorPageMocks.setAdapterError.mockReset();
    editorPageMocks.bumpFocus.mockReset();
    editorPageMocks.waitForChatDone.mockReset();
    editorPageMocks.waitForChatDone.mockResolvedValue('conv-1');
    editorPageMocks.waitForEditorPatch.mockReset();
    editorPageMocks.waitForEditorPatch.mockResolvedValue({ ok: false, error: 'Nenhum patch encontrado' });
    editorPageMocks.getMaxMessageId.mockReset();
    editorPageMocks.getMaxMessageId.mockReturnValue('');
    for (const key of Object.keys(editorPageMocks.runtimeHandlers)) {
      delete editorPageMocks.runtimeHandlers[key];
    }
    const richEditor = editorPageMocks.richEditor as {
      state: { selection: { from: number; to: number; empty: boolean } };
      isFocused: boolean;
      view: { hasFocus: () => boolean };
    };
    richEditor.state.selection.from = 1;
    richEditor.state.selection.to = 19;
    richEditor.state.selection.empty = false;
    richEditor.isFocused = true;
    richEditor.view.hasFocus = () => true;
    openToolbarMenuSpy.mockReset();
    editorStoreState.setDocMarkdown.mockReset();
    editorStoreState.setDocDirty.mockReset();
    editorStoreState.setDocMode.mockReset();
    vi.mocked(EditorReadFile).mockReset();
    vi.mocked(EditorReadFile).mockResolvedValue('Alpha\nselected markdown\nOmega' as never);
    vi.mocked(EditorWriteFile).mockReset();
    vi.mocked(EditorGetFileInfo).mockReset();
    vi.mocked(EditorGetFileInfo).mockResolvedValue({ exists: true, isDir: false, size: 20, modTimeMs: 2000 } as never);
    vi.mocked(GetProfile).mockReset();
    vi.mocked(GetProfile).mockResolvedValue({ chat: { disable_tools: false } } as Awaited<ReturnType<typeof GetProfile>>);
  });

  async function createEditorChatSendPlan(markdown = 'Alpha\nselected markdown\nOmega') {
    const selectedText = 'selected markdown';
    const selectionStart = markdown.indexOf(selectedText);
    editorPageMocks.markdownModelValue = markdown;
    editorPageMocks.markdownSelectionStartOffset = selectionStart;
    editorPageMocks.markdownSelectionEndOffset = selectionStart + selectedText.length;
    editorPageMocks.markdownCursorOffset = selectionStart;
    editorStoreState.documents = {
      'tab-1': {
        id: 'tab-1',
        title: 'Doc',
        markdown,
        mode: 'markdown',
        filePath: 'doc.md',
      },
    };

    render(
      <EditorPage
        workspaceTab={{
          id: 'tab-1',
          type: 'editor',
          title: 'Doc',
          position: 0,
          conversationId: 'conv-1',
          state: { filePath: 'doc.md' },
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
      ) => Promise<{ afterSend?: () => Promise<void> } | null>;
    };
    const prepared = await adapter.prepare();
    const plan = await adapter.send('Altere o trecho', undefined, prepared.meta, {
      tabId: 'tab-1',
      conversationId: 'conv-1',
    });

    expect(plan?.afterSend).toBeDefined();
    return plan!;
  }

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

  it('envia a seleção Markdown no SurfaceContext ao abrir pelo botão mesmo após perda de foco', async () => {
    const markdown = 'Alpha\nselected markdown\nOmega';
    const selectedText = 'selected markdown';
    const selectionStart = markdown.indexOf(selectedText);
    editorPageMocks.markdownModelValue = markdown;
    editorPageMocks.markdownSelectionStartOffset = selectionStart;
    editorPageMocks.markdownSelectionEndOffset = selectionStart + selectedText.length;
    editorPageMocks.markdownCursorOffset = selectionStart;
    editorStoreState.documents = {
      'tab-1': {
        id: 'tab-1',
        title: 'Doc',
        markdown,
        mode: 'markdown',
        filePath: 'doc.md',
      },
    };

    render(
      <EditorPage
        workspaceTab={{
          id: 'tab-1',
          type: 'editor',
          title: 'Doc',
          position: 0,
          conversationId: 'conv-1',
          state: { filePath: 'doc.md' },
        }}
      />
    );

    fireEvent.mouseDown(screen.getByRole('button', { name: 'editor.actions.askChat' }));
    editorPageMocks.markdownHasFocus = false;
    editorPageMocks.markdownSelectionEndOffset = editorPageMocks.markdownSelectionStartOffset;

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
    const plan = await adapter.send('Explique este trecho', undefined, prepared.meta, {
      tabId: 'tab-1',
      conversationId: 'conv-1',
    });
    const surfaceContext = JSON.parse(String(plan?.paramsOverride?.surfaceContextJson || '{}'));

    expect(surfaceContext.selection).toMatchObject({
      kind: 'text',
      text: selectedText,
      isEmpty: false,
      explicit: true,
    });
    expect(surfaceContext.selection.range.startOffset).toBe(selectionStart);
    expect(surfaceContext.selection.range.endOffset).toBe(selectionStart + selectedText.length);
  });

  it('envia a seleção Markdown preservada pelo listener de seleção usado no atalho', async () => {
    const markdown = 'Antes\ntexto do atalho\nDepois';
    const selectedText = 'texto do atalho';
    const selectionStart = markdown.indexOf(selectedText);
    editorPageMocks.markdownModelValue = markdown;
    editorPageMocks.markdownSelectionStartOffset = selectionStart;
    editorPageMocks.markdownSelectionEndOffset = selectionStart + selectedText.length;
    editorPageMocks.markdownCursorOffset = selectionStart;
    editorStoreState.documents = {
      'tab-1': {
        id: 'tab-1',
        title: 'Doc',
        markdown,
        mode: 'markdown',
        filePath: 'doc.md',
      },
    };

    render(
      <EditorPage
        workspaceTab={{
          id: 'tab-1',
          type: 'editor',
          title: 'Doc',
          position: 0,
          conversationId: 'conv-1',
          state: { filePath: 'doc.md' },
        }}
      />
    );
    await vi.waitFor(() => expect(editorPageMocks.markdownSelectionListener).toBeTruthy());
    act(() => {
      editorPageMocks.markdownSelectionListener?.();
    });
    editorPageMocks.markdownHasFocus = false;
    editorPageMocks.markdownSelectionEndOffset = editorPageMocks.markdownSelectionStartOffset;

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
    const plan = await adapter.send('Explique este trecho', undefined, prepared.meta, {
      tabId: 'tab-1',
      conversationId: 'conv-1',
    });
    const surfaceContext = JSON.parse(String(plan?.paramsOverride?.surfaceContextJson || '{}'));

    expect(surfaceContext.selection.text).toBe(selectedText);
    expect(surfaceContext.selection.explicit).toBe(true);
  });

  it('não reutiliza seleção Markdown antiga quando o editor está focado no cursor', async () => {
    const markdown = 'Antes\ntexto antigo\nDepois';
    const selectedText = 'texto antigo';
    const selectionStart = markdown.indexOf(selectedText);
    editorPageMocks.markdownModelValue = markdown;
    editorPageMocks.markdownSelectionStartOffset = selectionStart;
    editorPageMocks.markdownSelectionEndOffset = selectionStart + selectedText.length;
    editorPageMocks.markdownCursorOffset = selectionStart;
    editorStoreState.documents = {
      'tab-1': {
        id: 'tab-1',
        title: 'Doc',
        markdown,
        mode: 'markdown',
        filePath: 'doc.md',
      },
    };

    render(
      <EditorPage
        workspaceTab={{
          id: 'tab-1',
          type: 'editor',
          title: 'Doc',
          position: 0,
          conversationId: 'conv-1',
          state: { filePath: 'doc.md' },
        }}
      />
    );
    await vi.waitFor(() => expect(editorPageMocks.markdownSelectionListener).toBeTruthy());
    act(() => {
      editorPageMocks.markdownSelectionListener?.();
    });

    editorPageMocks.markdownHasFocus = true;
    editorPageMocks.markdownSelectionEndOffset = editorPageMocks.markdownSelectionStartOffset;

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
    const plan = await adapter.send('Explique o cursor', undefined, prepared.meta, {
      tabId: 'tab-1',
      conversationId: 'conv-1',
    });
    const surfaceContext = JSON.parse(String(plan?.paramsOverride?.surfaceContextJson || '{}'));

    expect(surfaceContext.selection.text).toBe('');
    expect(surfaceContext.selection.explicit).toBe(false);
  });

  it('envia a seleção rica preservada antes da perda de foco', async () => {
    editorStoreState.documents = {
      'tab-1': {
        id: 'tab-1',
        title: 'Doc',
        markdown: '## Slide 2\nselected rich text',
        mode: 'rich',
        filePath: 'doc.md',
      },
    };

    render(
      <EditorPage
        workspaceTab={{
          id: 'tab-1',
          type: 'editor',
          title: 'Doc',
          position: 0,
          conversationId: 'conv-1',
          state: { filePath: 'doc.md' },
        }}
      />
    );
    await vi.waitFor(() => expect(editorPageMocks.richSelectionListener).toBeTruthy());
    act(() => {
      editorPageMocks.richSelectionListener?.();
    });

    const richEditor = editorPageMocks.richEditor as {
      state: { selection: { from: number; to: number; empty: boolean } };
      isFocused: boolean;
      view: { hasFocus: () => boolean };
    };
    richEditor.isFocused = false;
    richEditor.view.hasFocus = () => false;
    richEditor.state.selection.to = richEditor.state.selection.from;
    richEditor.state.selection.empty = true;

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
    const plan = await adapter.send('Explique este trecho', undefined, prepared.meta, {
      tabId: 'tab-1',
      conversationId: 'conv-1',
    });
    const surfaceContext = JSON.parse(String(plan?.paramsOverride?.surfaceContextJson || '{}'));

    expect(surfaceContext.selection).toMatchObject({
      kind: 'text',
      text: 'selected rich text',
      markdown: '## Slide 2\nselected rich text',
      isEmpty: false,
      explicit: true,
    });
  });

  it('mantém o slide Reveal rico e o total do deck capturados no prepare ao enviar', async () => {
    editorPageMocks.initialRevealSlideIndex = 1;
    const richEditor = editorPageMocks.richEditor as {
      state: { selection: { from: number; to: number; empty: boolean } };
    };
    richEditor.state.selection.from = 1;
    richEditor.state.selection.to = 1;
    richEditor.state.selection.empty = true;
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
    const selection = prepared.meta as { revealSlideIndex?: number; revealSlideMarkdown?: string; revealSlideCount?: number };

    expect(selection.revealSlideIndex).toBe(1);
    expect(selection.revealSlideMarkdown).toContain('Slide 2');
    expect(selection.revealSlideCount).toBe(3);

    act(() => {
      editorStoreState.documents['tab-1'].markdown = '# Documento comum\n\nO conteúdo vivo deixou de ser um deck Reveal.';
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
    expect(surfaceContext.metadata.slideCount).toBe(3);
    expect(surfaceContext.focus.entity.slideIndex).toBe(1);
    expect(surfaceContext.content.markdown).toContain('Slide 2');
    expect(surfaceContext.content.markdown).not.toContain('Documento comum');
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

  it('mantém o chat modal aberto quando o turno com tools termina com erro', async () => {
    editorPageMocks.waitForChatDone.mockRejectedValueOnce(new Error('falha do modelo'));
    const plan = await createEditorChatSendPlan();

    await act(async () => {
      await plan.afterSend?.();
    });

    expect(editorPageMocks.closeModal).not.toHaveBeenCalled();
    expect(editorPageMocks.setAdapterError).toHaveBeenCalledWith('falha do modelo');
  });

  it('mantém o chat modal aberto em resposta textual sem edição no editor', async () => {
    const plan = await createEditorChatSendPlan();

    await act(async () => {
      await plan.afterSend?.();
    });

    expect(editorPageMocks.closeModal).not.toHaveBeenCalled();
    expect(editorPageMocks.bumpFocus).toHaveBeenCalled();
  });

  it('mantém o chat modal aberto quando a tool termina sem alteração no arquivo', async () => {
    const plan = await createEditorChatSendPlan();
    editorPageMocks.emitRuntimeEvent('chat:tool_start', {
      conversationId: 'conv-1',
      name: 'edit_file',
    });
    editorPageMocks.emitRuntimeEvent('chat:tool_end', {
      conversationId: 'conv-1',
      name: 'edit_file',
      status: 'done',
    });

    await act(async () => {
      await plan.afterSend?.();
    });

    expect(editorPageMocks.closeModal).not.toHaveBeenCalled();
    expect(editorPageMocks.bumpFocus).toHaveBeenCalled();
  });

  it('fecha o chat modal quando a edição aprovada altera o documento do editor', async () => {
    const plan = await createEditorChatSendPlan();
    editorPageMocks.emitRuntimeEvent('chat:tool_start', {
      conversationId: 'conv-1',
      name: 'edit_file',
    });
    editorStoreState.documents['tab-1'].markdown = 'Alpha\nselected markdown alterado\nOmega';
    editorPageMocks.emitRuntimeEvent('editor:fileChanged', {
      path: 'doc.md',
      origin: 'assistant_tool',
      assisted: true,
    });

    await act(async () => {
      await plan.afterSend?.();
    });

    expect(editorPageMocks.closeModal).toHaveBeenCalledTimes(1);
    expect(editorPageMocks.setAdapterError).toHaveBeenCalledWith(null);
  });

  it('fecha o chat modal quando a edição aprovada altera o arquivo mesmo sem auto-reload do editor', async () => {
    const plan = await createEditorChatSendPlan();
    editorPageMocks.emitRuntimeEvent('chat:tool_start', {
      conversationId: 'conv-1',
      name: 'edit_file',
    });
    editorPageMocks.emitRuntimeEvent('editor:fileChanged', {
      path: 'doc.md',
      origin: 'assistant_tool',
      assisted: true,
    });

    await act(async () => {
      await plan.afterSend?.();
    });

    expect(editorPageMocks.closeModal).toHaveBeenCalledTimes(1);
    expect(editorStoreState.documents['tab-1'].markdown).toBe('Alpha\nselected markdown\nOmega');
  });

  it('ignora evento assistido tardio depois de tool sem alteração no arquivo', async () => {
    const plan = await createEditorChatSendPlan();
    editorPageMocks.emitRuntimeEvent('chat:tool_start', {
      conversationId: 'conv-1',
      name: 'edit_file',
    });
    editorPageMocks.emitRuntimeEvent('chat:tool_end', {
      conversationId: 'conv-1',
      name: 'edit_file',
      status: 'done',
    });

    await act(async () => {
      await plan.afterSend?.();
    });

    expect(editorPageMocks.closeModal).not.toHaveBeenCalled();

    act(() => {
      editorPageMocks.emitRuntimeEvent('editor:fileChanged', {
        path: 'doc.md',
        origin: 'assistant_tool',
        assisted: true,
      });
    });

    expect(editorPageMocks.closeModal).not.toHaveBeenCalled();
  });

  it('mantém o chat modal aberto quando a confirmação da edição é rejeitada', async () => {
    const plan = await createEditorChatSendPlan();
    editorPageMocks.emitRuntimeEvent('chat:tool_start', {
      conversationId: 'conv-1',
      name: 'edit_file',
    });
    editorPageMocks.emitRuntimeEvent('chat:tool_end', {
      conversationId: 'conv-1',
      name: 'edit_file',
      status: 'error',
    });

    await act(async () => {
      await plan.afterSend?.();
    });

    editorPageMocks.emitRuntimeEvent('editor:fileChanged', {
      path: 'doc.md',
      origin: 'assistant_tool',
      assisted: true,
    });

    expect(editorPageMocks.closeModal).not.toHaveBeenCalled();
    expect(editorStoreState.documents['tab-1'].markdown).toBe('Alpha\nselected markdown\nOmega');
  });

  it('mantém o chat modal aberto quando a policy não permite edit_file e nada muda', async () => {
    vi.mocked(GetProfile).mockResolvedValueOnce({
      chat: { disable_tools: false },
      enabled_tools: [],
    } as unknown as Awaited<ReturnType<typeof GetProfile>>);
    const plan = await createEditorChatSendPlan();

    await act(async () => {
      await plan.afterSend?.();
    });

    expect(editorPageMocks.closeModal).not.toHaveBeenCalled();
    expect(editorPageMocks.bumpFocus).toHaveBeenCalled();
  });

  it('sincroniza edição por tool antes de fechar o chat modal', async () => {
    editorPageMocks.markdownModelValue = 'antes da tool';
    editorStoreState.documents = {
      'tab-1': {
        id: 'tab-1',
        title: 'Doc',
        markdown: 'antes da tool',
        mode: 'markdown',
        filePath: 'doc.md',
        isDirty: false,
      },
    };
    vi.mocked(EditorReadFile).mockResolvedValue('depois da tool' as never);

    render(
      <EditorPage
        workspaceTab={{
          id: 'tab-1',
          type: 'editor',
          title: 'Doc',
          position: 0,
          conversationId: 'conv-1',
          state: { filePath: 'doc.md' },
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
      ) => Promise<{ afterSend?: () => Promise<void> } | null>;
    };
    const prepared = await adapter.prepare();
    const plan = await adapter.send('Reescreva', undefined, prepared.meta, {
      tabId: 'tab-1',
      conversationId: 'conv-1',
    });
    editorPageMocks.emitRuntimeEvent('chat:tool_start', {
      conversationId: 'conv-1',
      name: 'edit_file',
    });
    editorPageMocks.emitRuntimeEvent('chat:tool_end', {
      conversationId: 'conv-1',
      name: 'edit_file',
      status: 'done',
    });

    await act(async () => {
      await plan?.afterSend?.();
    });

    expect(editorStoreState.setDocMarkdown).toHaveBeenCalledWith('tab-1', 'depois da tool');
    expect(editorPageMocks.closeModal).toHaveBeenCalled();
    expect(editorStoreState.setDocMarkdown.mock.invocationCallOrder[0]).toBeLessThan(
      editorPageMocks.closeModal.mock.invocationCallOrder[0]
    );
  });
});
