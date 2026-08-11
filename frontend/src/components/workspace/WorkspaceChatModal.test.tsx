import { fireEvent, render, screen } from '@testing-library/react';
import type React from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useLandmarkNavigation, type Landmark } from '../../hooks/useLandmarkNavigation';

const hoisted = vi.hoisted(() => {
  const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
  const mockSetBoundConversation = vi.fn();
  const isWorkspaceModalTopmost = vi.fn(() => true);
  const boundSendMock = vi.fn();
  const sendChatSurfaceMessageMock = vi.fn();
  const workspaceChatModalState = {
    isOpen: true,
    boundTabId: 'tab-editor',
    boundConversationId: conversationId,
    boundSurface: {
      conversationId,
      sessionKey: `modal:workspace-chat:tab-editor:${conversationId}`,
      surfaceId: 'modal:workspace-chat:tab-editor',
      surfaceType: 'modal' as const,
      tabId: 'tab-editor',
    },
    contextDisplay: 'contexto',
    sessionMeta: null as unknown,
    boundSend: boundSendMock,
    focusNonce: 1,
    adapterError: null,
    close: vi.fn(),
    setBoundConversation: mockSetBoundConversation,
  };
  const activeTab = { id: 'tab-editor', type: 'editor' as const, title: 'Editor', position: 0 };
  const capturedChatPanelProps: {
    onRequestConversationChange?: (id: string, conversation: { title?: string }) => void;
    onSend?: (content: string, mediaFiles: undefined, context: { origin: typeof workspaceChatModalState.boundSurface }) => Promise<void>;
  } = {};
  return {
    activeTab,
    boundSendMock,
    capturedChatPanelProps,
    conversationId,
    isWorkspaceModalTopmost,
    mockSetBoundConversation,
    sendChatSurfaceMessageMock,
    workspaceChatModalState,
  };
});

const {
  activeTab,
  boundSendMock,
  capturedChatPanelProps,
  conversationId,
  isWorkspaceModalTopmost,
  mockSetBoundConversation,
  sendChatSurfaceMessageMock,
  workspaceChatModalState,
} = hoisted;

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock('../ui/Modal', () => ({
  Modal: ({ isOpen, title, children }: { isOpen: boolean; title: string; children: React.ReactNode }) => (
    isOpen ? (
      <section aria-label={title} className="modal-overlay">
        <h1>{title}</h1>
        {children}
      </section>
    ) : null
  ),
  useModalIsTopmost: () => hoisted.isWorkspaceModalTopmost,
  isModalOpen: () => true,
}));

vi.mock('../chat/ChatPanel', () => ({
  useEffectiveProfileSlug: () => undefined,
  ChatPanel: ({ surface, onRequestConversationChange, onSend }: {
    surface: { sessionKey: string };
    onRequestConversationChange?: (id: string, conversation: { title?: string }) => void;
    onSend?: (content: string, mediaFiles: undefined, context: { origin: typeof workspaceChatModalState.boundSurface }) => Promise<void>;
  }) => {
    hoisted.capturedChatPanelProps.onRequestConversationChange = onRequestConversationChange;
    hoisted.capturedChatPanelProps.onSend = onSend;
    return (
      <div data-session-key={surface.sessionKey}>
        chat-panel
        <div className="ws-content-toolbar">
          <button type="button">Ação da toolbar</button>
        </div>
        <div className="ws-content-area">
          <div className="message-list">
            <div className="message-list__list" tabIndex={0} aria-label="Lista de mensagens">
              mensagens
              <button
                type="button"
                onKeyDown={(event) => {
                  if (event.key === 'Escape') {
                    event.preventDefault();
                  }
                }}
              >
                Mensagem com Escape local
              </button>
            </div>
          </div>
          <div className="chat-input">
            <textarea aria-label="Mensagem" className="chat-input__textarea" />
          </div>
        </div>
      </div>
    );
  },
}));

vi.mock('../chat/ChatSurfaceController', () => ({
  sendChatSurfaceMessage: hoisted.sendChatSurfaceMessageMock,
  useChatConversationTimeline: () => ({
    id: hoisted.conversationId,
    title: 'Título da timeline',
    threadedMessages: [],
  }),
}));

vi.mock('../../store/workspaceChatModalStore', () => ({
  useWorkspaceChatModalStore: Object.assign(
    (selector?: (state: typeof workspaceChatModalState) => unknown) => (
      typeof selector === 'function' ? selector(hoisted.workspaceChatModalState) : hoisted.workspaceChatModalState
    ),
    {
      getState: () => hoisted.workspaceChatModalState,
    },
  ),
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector?: (state: { workspace: { tabs: Array<typeof activeTab> } }) => unknown) => {
      const state = { workspace: { tabs: [hoisted.activeTab] } };
      return typeof selector === 'function' ? selector(state) : state;
    },
    {
      getState: () => ({ workspace: { tabs: [hoisted.activeTab] } }),
    },
  ),
  useActiveTab: () => hoisted.activeTab,
  useWorkspaceTabs: () => [hoisted.activeTab],
}));

vi.mock('../../store/chatStore', () => ({
  useChatStore: (selector?: (state: {
    sessionsByConversationId: Record<string, never>;
    timelinesByConversationId: Record<string, { id: string; title: string; threadedMessages: never[] }>;
  }) => unknown) => {
    const state = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        [hoisted.conversationId]: {
          id: hoisted.conversationId,
          title: 'Título da timeline',
          threadedMessages: [],
        },
      },
    };
    return typeof selector === 'function' ? selector(state) : state;
  },
}));

vi.mock('../../store/uiStore', () => ({
  useUIStore: {
    getState: () => ({ addToast: vi.fn() }),
  },
}));

vi.mock('../../lib/workspaceConversation', () => ({
  ensureWorkspaceTabConversationId: vi.fn(),
}));

import { WorkspaceChatModal } from './WorkspaceChatModal';

function BackgroundLandmarks({ onFocus }: { onFocus: () => boolean }) {
  const landmarks: Landmark[] = [{
    id: 'workspaceToolbar',
    label: 'Workspace',
    focus: onFocus,
    contains: () => false,
  }];
  useLandmarkNavigation({ landmarks, defaultLandmarkId: 'workspaceToolbar' });
  return <button type="button">Workspace atrás</button>;
}

describe('WorkspaceChatModal', () => {
  beforeEach(() => {
    capturedChatPanelProps.onRequestConversationChange = undefined;
    capturedChatPanelProps.onSend = undefined;
    mockSetBoundConversation.mockClear();
    boundSendMock.mockReset();
    sendChatSurfaceMessageMock.mockReset();
    workspaceChatModalState.sessionMeta = null;
    workspaceChatModalState.boundSend = boundSendMock;
    workspaceChatModalState.close.mockClear();
    isWorkspaceModalTopmost.mockClear();
    isWorkspaceModalTopmost.mockReturnValue(true);
  });

  it('usa timeline canônica no título quando sessão legada não existe', () => {
    render(<WorkspaceChatModal />);

    expect(screen.getByRole('heading', {
      name: 'editor.chatModal.title — Título da timeline',
    })).toBeInTheDocument();
  });

  it('renderiza o chat com a superfície vinculada ao painel de origem', () => {
    render(<WorkspaceChatModal />);

    expect(screen.getByText('chat-panel')).toHaveAttribute(
      'data-session-key',
      `modal:workspace-chat:tab-editor:${conversationId}`,
    );
  });

  it('troca de conversa no chat embutido delega para setBoundConversation', () => {
    render(<WorkspaceChatModal />);

    // Garante que a prop foi de fato capturada neste render (evita falso-positivo
    // por callback "stale" de outro teste, já que o handle é um global mutável).
    expect(capturedChatPanelProps.onRequestConversationChange).toBeDefined();

    capturedChatPanelProps.onRequestConversationChange?.('nova-conversa', { title: 'Outra' });

    expect(mockSetBoundConversation).toHaveBeenCalledWith('nova-conversa');
  });

  it('encaminha paramsOverride do adapter para o envio do chat', async () => {
    const paramsOverride = {
      tabType: 'editor',
      surfaceContextJson: JSON.stringify({
        surfaceType: 'editor',
        surfaceId: 'tab-editor',
        snapshotVersion: 'editor:tab-editor:1',
        selection: {
          kind: 'text',
          text: 'texto selecionado',
          explicit: true,
        },
      }),
    };
    workspaceChatModalState.sessionMeta = { selectedText: 'texto selecionado' };
    boundSendMock.mockResolvedValue({
      content: 'Explique',
      paramsOverride,
    });

    render(<WorkspaceChatModal />);
    await capturedChatPanelProps.onSend?.('Explique', undefined, {
      origin: workspaceChatModalState.boundSurface,
    });

    expect(boundSendMock).toHaveBeenCalledWith(
      'Explique',
      undefined,
      workspaceChatModalState.sessionMeta,
      { tabId: 'tab-editor', conversationId },
    );
    expect(sendChatSurfaceMessageMock).toHaveBeenCalledWith(
      conversationId,
      'Explique',
      undefined,
      paramsOverride,
      expect.objectContaining({ conversationId, tabId: 'tab-editor' }),
    );
  });

  it('nao rouba foco quando outro modal esta no topo', async () => {
    isWorkspaceModalTopmost.mockReturnValue(false);

    render(
      <>
        <WorkspaceChatModal />
        <button>Confirmar conflito</button>
      </>
    );

    const questionnaireControl = screen.getByRole('button', { name: 'Confirmar conflito' });
    questionnaireControl.focus();

    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

    expect(screen.getByRole('textbox', { name: 'Mensagem' })).not.toHaveFocus();
    expect(questionnaireControl).toHaveFocus();
  });

  it('F6 navega para landmarks do chat modal quando ele esta no topo', () => {
    render(<WorkspaceChatModal />);

    screen.getByRole('textbox', { name: 'Mensagem' }).focus();
    fireEvent.keyDown(window, { key: 'F6' });

    expect(screen.getByRole('button', { name: 'Ação da toolbar' })).toHaveFocus();
  });

  it('Shift+F6 navega para mensagens/contexto dentro do chat modal', () => {
    render(<WorkspaceChatModal />);

    screen.getByRole('textbox', { name: 'Mensagem' }).focus();
    fireEvent.keyDown(window, { key: 'F6', shiftKey: true });

    expect(screen.getByLabelText('Lista de mensagens')).toHaveFocus();
  });

  it('Escape a partir de outro landmark retorna ao composer do chat modal', () => {
    render(<WorkspaceChatModal />);

    const toolbarButton = screen.getByRole('button', { name: 'Ação da toolbar' });
    toolbarButton.focus();
    fireEvent.keyDown(toolbarButton, { key: 'Escape' });

    expect(screen.getByRole('textbox', { name: 'Mensagem' })).toHaveFocus();
    expect(workspaceChatModalState.close).not.toHaveBeenCalled();
  });

  it('Escape respeita handlers locais dentro das mensagens', () => {
    render(<WorkspaceChatModal />);

    const messageButton = screen.getByRole('button', { name: 'Mensagem com Escape local' });
    messageButton.focus();
    fireEvent.keyDown(messageButton, { key: 'Escape' });

    expect(messageButton).toHaveFocus();
    expect(screen.getByRole('textbox', { name: 'Mensagem' })).not.toHaveFocus();
  });

  it('F6 nao navega para landmarks do workspace atras do modal', () => {
    const focusBehind = vi.fn(() => true);

    render(
      <>
        <BackgroundLandmarks onFocus={focusBehind} />
        <WorkspaceChatModal />
      </>,
    );

    screen.getByRole('textbox', { name: 'Mensagem' }).focus();
    fireEvent.keyDown(window, { key: 'F6' });

    expect(focusBehind).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Ação da toolbar' })).toHaveFocus();
  });
});
