import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ChatToolbar } from './ChatToolbar';

const clearConversationMock = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const loadConversationSessionMock = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const clearConversationMessagesMock = vi.hoisted(() => vi.fn());
const historyClickMock = vi.hoisted(() => vi.fn());
const getAgentSessionOptionsMock = vi.hoisted(() => vi.fn().mockResolvedValue({
  conversationId: 'conversation-1',
  available: false,
  options: [],
}));
const requestResourceEditMock = vi.hoisted(() => vi.fn());
const mockPanelTabRef = vi.hoisted(() => ({ current: { id: 'tab-chat', title: 'Chat', type: 'chat' } as unknown as Record<string, unknown> }));
const openAtPointMock = vi.hoisted(() => vi.fn());
// A conversa deste teste não fala com agente de código: o diretório do agente
// não existe para ela, e o controle da barra some.
const getAgentWorkDirMock = vi.hoisted(() => vi.fn().mockRejectedValue(new Error('sem agente')));
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

vi.mock('@wailsjs/go/wailsapi/Conversations', () => ({
  ClearConversation: clearConversationMock,
}));

vi.mock('@wailsjs/go/wailsapi/ACPWorkDir', () => ({
  GetAgentConversationWorkDir: getAgentWorkDirMock,
  SetAgentConversationWorkDir: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/ACPOptions', () => ({
  GetAgentSessionOptions: getAgentSessionOptionsMock,
  SetAgentSessionOption: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({
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
    tab: mockPanelTabRef.current,
  }),
  useOptionalWorkspacePanel: () => ({
    tab: mockPanelTabRef.current,
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
    getState: () => ({ requestResourceEdit: requestResourceEditMock }),
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
    openAtPoint: openAtPointMock,
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

// O seletor de modelo do agente precisa aparecer na barra da conversa aberta, e
// pela conversa dela: é o caminho que a pessoa usa para trocar de modelo, e uma
// ligação errada aqui trocaria o modelo de outra conversa (AEP-0084 D6).
describe('ChatToolbar e o modelo do agente', () => {
  beforeEach(() => {
    getAgentSessionOptionsMock.mockClear();
  });

  it('mostra o modelo do agente da conversa aberta', async () => {
    getAgentSessionOptionsMock.mockResolvedValueOnce({
      conversationId: 'conversation-1',
      available: true,
      options: [{
        id: 'model',
        name: 'Modelo',
        category: 'model',
        currentValue: 'modelo-a',
        values: [{ value: 'modelo-a', name: 'Modelo A' }, { value: 'modelo-b', name: 'Modelo B' }],
      }],
    });

    renderToolbar();

    await waitFor(() => expect(getAgentSessionOptionsMock).toHaveBeenCalledWith('conversation-1'));
    expect(await screen.findByRole('button', { name: 'Modelo, Modelo A' })).toBeInTheDocument();
  });

  it('não mostra seletor quando a conversa não fala com agente', async () => {
    getAgentSessionOptionsMock.mockResolvedValueOnce({
      conversationId: 'conversation-1',
      available: false,
      options: [],
    });

    renderToolbar();

    await waitFor(() => expect(getAgentSessionOptionsMock).toHaveBeenCalledWith('conversation-1'));
    expect(screen.queryByRole('button', { name: /Modelo/ })).toBeNull();
  });
});

describe('ChatToolbar menu de perfil', () => {
  beforeEach(() => {
    requestResourceEditMock.mockClear();
    openAtPointMock.mockClear();
    mockPanelTabRef.current = { id: 'tab-chat', title: 'Chat', type: 'chat' } as unknown as Record<string, unknown>;
  });

  it('edita o perfil da aba quando há override', async () => {
    mockPanelTabRef.current = {
      id: 'tab-chat',
      title: 'Chat',
      type: 'chat',
      profileOverride: { slug: 'custom' },
    } as unknown as Record<string, unknown>;

    renderToolbar();
    const container = screen.getByTestId('profile-picker-container');
    fireEvent.contextMenu(container, { clientX: 10, clientY: 10 });

    await waitFor(() => expect(openAtPointMock).toHaveBeenCalled());
    const items = openAtPointMock.mock.calls[0][3] as Array<{ id: string; action: () => void }>;
    const editItem = items.find((i) => i.id === 'edit-active-profile');
    editItem?.action();
    expect(requestResourceEditMock).toHaveBeenCalledWith('profiles', 'custom', 'edit');
  });

  it('edita o perfil padrão quando não há override', async () => {
    mockPanelTabRef.current = { id: 'tab-chat', title: 'Chat', type: 'chat' } as unknown as Record<string, unknown>;

    renderToolbar();
    const container = screen.getByTestId('profile-picker-container');
    fireEvent.contextMenu(container, { clientX: 10, clientY: 10 });

    await waitFor(() => expect(openAtPointMock).toHaveBeenCalled());
    const items = openAtPointMock.mock.calls[0][3] as Array<{ id: string; action: () => void }>;
    const editItem = items.find((i) => i.id === 'edit-active-profile');
    editItem?.action();
    expect(requestResourceEditMock).toHaveBeenCalledWith('profiles', 'padrao', 'edit');
  });
});
