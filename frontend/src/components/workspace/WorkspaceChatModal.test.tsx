import { render, screen } from '@testing-library/react';
import type React from 'react';
import { describe, expect, it, vi } from 'vitest';

const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';

const mockSetBoundConversation = vi.fn();

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
  focusNonce: 1,
  adapterError: null,
  close: vi.fn(),
  setBoundConversation: mockSetBoundConversation,
};

const activeTab = { id: 'tab-editor', type: 'editor' as const, title: 'Editor', position: 0 };

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock('../ui/Modal', () => ({
  Modal: ({ isOpen, title, children }: { isOpen: boolean; title: string; children: React.ReactNode }) => (
    isOpen ? (
      <section aria-label={title}>
        <h1>{title}</h1>
        {children}
      </section>
    ) : null
  ),
}));

const capturedChatPanelProps: {
  onRequestConversationChange?: (id: string, conversation: { title?: string }) => void;
} = {};

vi.mock('../chat/ChatPanel', () => ({
  ChatPanel: ({ surface, onRequestConversationChange }: {
    surface: { sessionKey: string };
    onRequestConversationChange?: (id: string, conversation: { title?: string }) => void;
  }) => {
    capturedChatPanelProps.onRequestConversationChange = onRequestConversationChange;
    return <div data-session-key={surface.sessionKey}>chat-panel</div>;
  },
}));

vi.mock('../../store/workspaceChatModalStore', () => ({
  useWorkspaceChatModalStore: (selector?: (state: typeof workspaceChatModalState) => unknown) => (
    typeof selector === 'function' ? selector(workspaceChatModalState) : workspaceChatModalState
  ),
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector?: (state: { workspace: { tabs: Array<typeof activeTab> } }) => unknown) => {
      const state = { workspace: { tabs: [activeTab] } };
      return typeof selector === 'function' ? selector(state) : state;
    },
    {
      getState: () => ({ workspace: { tabs: [activeTab] } }),
    },
  ),
  useActiveTab: () => activeTab,
}));

vi.mock('../../store/chatStore', () => ({
  useChatStore: (selector?: (state: {
    sessionsByConversationId: Record<string, never>;
    timelinesByConversationId: Record<string, { id: string; title: string; threadedMessages: never[] }>;
  }) => unknown) => {
    const state = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        [conversationId]: {
          id: conversationId,
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

describe('WorkspaceChatModal', () => {
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
    mockSetBoundConversation.mockClear();
    render(<WorkspaceChatModal />);

    capturedChatPanelProps.onRequestConversationChange?.('nova-conversa', { title: 'Outra' });

    expect(mockSetBoundConversation).toHaveBeenCalledWith('nova-conversa');
  });
});
