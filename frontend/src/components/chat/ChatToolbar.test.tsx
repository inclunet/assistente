import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ChatToolbar } from './ChatToolbar';

const clearConversationMock = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const loadConversationSessionMock = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const clearConversationMessagesMock = vi.hoisted(() => vi.fn());
const historyClickMock = vi.hoisted(() => vi.fn());
const profileClickMock = vi.hoisted(() => vi.fn());
const modalState = vi.hoisted(() => ({
  open: false,
  inside: false,
  topmost: true,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  ClearConversation: clearConversationMock,
  GetActiveProfileSlug: vi.fn().mockResolvedValue('padrao'),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(() => vi.fn()),
}));

vi.mock('../ui/Modal', () => ({
  isModalOpen: () => modalState.open,
  useIsInsideModal: () => modalState.inside,
  useModalIsTopmost: () => () => modalState.topmost,
}));

vi.mock('../pickers', async () => {
  const React = await import('react');
  return {
    HistoryPicker: React.forwardRef<HTMLButtonElement>(() => (
      <button className="picker-button" type="button" onClick={historyClickMock}>
        Historico
      </button>
    )),
  };
});

vi.mock('../pickers/ProfilePicker', async () => {
  const React = await import('react');
  return {
    ProfilePicker: React.forwardRef<HTMLButtonElement>(() => (
      <button className="picker-button" type="button" onClick={profileClickMock}>
        Perfil
      </button>
    )),
  };
});

vi.mock('./ChatSessionContext', () => ({
  useChatSession: () => ({
    conversationId: 'conversation-1',
    session: { queuedTurnCount: 0 },
    conversation: { id: 'conversation-1', title: 'Conversa' },
    isLoading: false,
    clearConversationMessages: clearConversationMessagesMock,
    loadConversationSession: loadConversationSessionMock,
  }),
}));

vi.mock('../workspace/WorkspacePanelContext', () => ({
  useWorkspacePanel: () => ({
    tab: { id: 'tab-chat', title: 'Chat', type: 'chat' },
  }),
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: (selector?: (state: unknown) => unknown) => {
    const state = {
      workspace: { profile: 'padrao', tabs: [{ id: 'tab-chat', title: 'Chat', type: 'chat' }] },
      updateTab: vi.fn().mockResolvedValue(undefined),
    };
    return typeof selector === 'function' ? selector(state) : state;
  },
}));

vi.mock('../../store/uiStore', () => ({
  useUIStore: (selector?: (state: unknown) => unknown) => {
    const state = { addToast: vi.fn() };
    return typeof selector === 'function' ? selector(state) : state;
  },
}));

vi.mock('../../store/navigationStore', () => ({
  useNavigationStore: {
    getState: () => ({ requestResourceEdit: vi.fn() }),
  },
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: vi.fn(),
    announceRequest: vi.fn(),
  }),
}));

vi.mock('../../hooks/useDefaultFocus', () => ({
  restoreDefaultFocus: vi.fn(),
}));

vi.mock('../../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({
    menu: { visible: false, x: 0, y: 0, items: [], ariaLabel: '' },
    openAtPoint: vi.fn(),
    closeMenu: vi.fn(),
    onSelectItem: vi.fn(),
  }),
}));

vi.mock('../menu', () => ({
  Menu: () => null,
}));

vi.mock('./TokenStatsButton', () => ({
  TokenStatsButton: () => <button type="button">Tokens</button>,
}));

vi.mock('./TokenStatsModal', () => ({
  TokenStatsModal: () => null,
}));

function renderToolbar() {
  return render(
    <MemoryRouter>
      <ChatToolbar />
    </MemoryRouter>,
  );
}

function dispatchCtrlKey(key: string) {
  const event = new KeyboardEvent('keydown', {
    key,
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
  });
  window.dispatchEvent(event);
  return event;
}

describe('ChatToolbar shortcuts', () => {
  beforeEach(() => {
    clearConversationMock.mockClear();
    loadConversationSessionMock.mockClear();
    clearConversationMessagesMock.mockClear();
    historyClickMock.mockClear();
    profileClickMock.mockClear();
    modalState.open = false;
    modalState.inside = false;
    modalState.topmost = true;
  });

  it('aciona atalhos do chat quando o toolbar esta dentro do modal topmost', async () => {
    modalState.open = true;
    modalState.inside = true;
    modalState.topmost = true;
    renderToolbar();

    expect(dispatchCtrlKey('h').defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('p').defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('l').defaultPrevented).toBe(true);

    expect(historyClickMock).toHaveBeenCalledTimes(1);
    expect(profileClickMock).toHaveBeenCalledTimes(1);
    await waitFor(() => {
      expect(clearConversationMock).toHaveBeenCalledWith('conversation-1');
      expect(loadConversationSessionMock).toHaveBeenCalledWith('conversation-1', { refreshSurfaceWindows: true });
    });
  });

  it('bloqueia atalhos do chat quando outro modal esta no topo', () => {
    modalState.open = true;
    modalState.inside = true;
    modalState.topmost = false;
    renderToolbar();

    expect(dispatchCtrlKey('h').defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('p').defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('l').defaultPrevented).toBe(true);

    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
    expect(clearConversationMock).not.toHaveBeenCalled();
  });

  it('nao deixa atalhos vazarem para toolbar atras de modal', () => {
    modalState.open = true;
    modalState.inside = false;
    modalState.topmost = true;
    renderToolbar();

    expect(dispatchCtrlKey('h').defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('p').defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('l').defaultPrevented).toBe(true);

    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
    expect(clearConversationMock).not.toHaveBeenCalled();
  });

  it('continua acionando atalhos quando nenhum modal esta aberto', () => {
    renderToolbar();

    dispatchCtrlKey('H');
    dispatchCtrlKey('P');

    expect(screen.getByRole('heading', { name: 'Conversa' })).toBeInTheDocument();
    expect(historyClickMock).toHaveBeenCalledTimes(1);
    expect(profileClickMock).toHaveBeenCalledTimes(1);
  });
});
