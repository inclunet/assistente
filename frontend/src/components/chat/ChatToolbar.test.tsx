import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ChatToolbar } from './ChatToolbar';

const clearConversationMock = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const loadConversationSessionMock = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const clearConversationMessagesMock = vi.hoisted(() => vi.fn());
const updateTabMock = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
const getProfileMock = vi.hoisted(() => vi.fn().mockResolvedValue({
  chat: { llm_provider: 'native-provider', model: 'modelo-perfil' },
}));
const getProvidersMock = vi.hoisted(() => vi.fn().mockResolvedValue([
  { id: 'native-provider', api_format: 'openai', is_default: true },
]));
const modelChangeRef = vi.hoisted(() => ({ current: null as null | ((model: string) => void) }));
const modelOpenMock = vi.hoisted(() => vi.fn());
const announceMock = vi.hoisted(() => vi.fn());
const profileChangeRef = vi.hoisted(() => ({ current: null as null | ((slug: string) => void) }));
const historyClickMock = vi.hoisted(() => vi.fn());
const getAgentSessionOptionsMock = vi.hoisted(() => vi.fn().mockResolvedValue({
  conversationId: 'conversation-1',
  available: false,
  options: [],
}));
const requestResourceEditMock = vi.hoisted(() => vi.fn());
const mockPanelTabRef = vi.hoisted(() => ({ current: { id: 'tab-chat', title: 'Chat', type: 'chat' } as unknown as Record<string, unknown> }));
const activeConversationRef = vi.hoisted(() => ({
  current: { id: 'conversation-1', title: 'Conversa' } as { id: string; title: string } | null,
}));
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
  GetPinnedMessages: vi.fn().mockResolvedValue([]),
  ToggleMessagePin: vi.fn(),
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
  GetProfile: getProfileMock,
}));

vi.mock('@wailsjs/go/wailsapi/LLMProviders', () => ({
  GetLLMProvidersWithStatus: getProvidersMock,
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(() => vi.fn()),
}));

vi.mock('../ui/Modal', () => ({
  Modal: ({ children, isOpen }: { children: ReactNode; isOpen: boolean }) => (
    isOpen ? <div>{children}</div> : null
  ),
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
    ProfilePicker: React.forwardRef<HTMLButtonElement, { onChange: (slug: string) => void }>(({ onChange }) => {
      profileChangeRef.current = onChange;
      return (
        <button className="picker-button" type="button" onClick={profileClickMock}>
          Perfil
        </button>
      );
    }),
  };
});

vi.mock('../pickers/ModelPicker', () => ({
  ModelPicker: ({ value, label, onChange, shortcut }: {
    value: string;
    label: string;
    onChange: (model: string) => void;
    shortcut?: string;
  }) => {
    modelChangeRef.current = onChange;
    return (
      <button
        className="picker-button"
        type="button"
        aria-label={`${label}, ${value}`}
        data-shortcut={shortcut}
        title={shortcut}
        onClick={modelOpenMock}
      >
        {label}
      </button>
    );
  },
}));

vi.mock('./ChatSessionContext', () => ({
  useChatSession: () => ({
    conversationId: 'conversation-1',
    session: { queuedTurnCount: 0 },
    conversation: activeConversationRef.current,
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
      updateTab: updateTabMock,
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
    announce: announceMock,
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

function dispatchCtrlKey(key: string, target: EventTarget = window) {
  const event = new KeyboardEvent('keydown', {
    key,
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
  });
  target.dispatchEvent(event);
  return event;
}

function dispatchModelShortcut(
  target: EventTarget = window,
  init: Partial<KeyboardEventInit> = {},
) {
  const event = new KeyboardEvent('keydown', {
    key: 'm',
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
    ...init,
  });
  target.dispatchEvent(event);
  return event;
}

beforeEach(() => {
  updateTabMock.mockReset().mockResolvedValue(undefined);
  getProfileMock.mockReset().mockResolvedValue({
    chat: { llm_provider: 'native-provider', model: 'modelo-perfil' },
  });
  getProvidersMock.mockReset().mockResolvedValue([
    { id: 'native-provider', api_format: 'openai', is_default: true },
  ]);
  modelChangeRef.current = null;
  modelOpenMock.mockClear();
  announceMock.mockClear();
  profileChangeRef.current = null;
  activeConversationRef.current = { id: 'conversation-1', title: 'Conversa' };
  mockPanelTabRef.current = { id: 'tab-chat', title: 'Chat', type: 'chat' } as unknown as Record<string, unknown>;
});

describe('ChatToolbar mensagens fixadas', () => {
  it('abre a lista enquanto os detalhes da conversa ainda carregam', async () => {
    activeConversationRef.current = null;
    renderToolbar();

    fireEvent.click(screen.getByRole('button', { name: 'chat.pins.button' }));

    expect(await screen.findByText('chat.pins.description')).toBeInTheDocument();
  });
});

describe('ChatToolbar shortcuts', () => {
  beforeEach(() => {
    clearConversationMock.mockClear();
    loadConversationSessionMock.mockClear();
    clearConversationMessagesMock.mockClear();
    historyClickMock.mockClear();
    profileClickMock.mockClear();
    updateTabMock.mockClear();
    getProfileMock.mockReset().mockResolvedValue({
      chat: { llm_provider: 'native-provider', model: 'modelo-perfil' },
    });
    getProvidersMock.mockReset().mockResolvedValue([
      { id: 'native-provider', api_format: 'openai', is_default: true },
    ]);
    modelChangeRef.current = null;
    profileChangeRef.current = null;
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

  it('deixa a toolbar do modal tratar o evento prevenido pela superfície atrás dele', async () => {
    modalState.open = true;
    modalState.inside = false;
    renderToolbar();
    modalState.inside = true;
    renderToolbar();

    expect(dispatchCtrlKey('h').defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('p').defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('l').defaultPrevented).toBe(true);

    expect(historyClickMock).toHaveBeenCalledOnce();
    expect(profileClickMock).toHaveBeenCalledOnce();
    await waitFor(() => expect(clearConversationMock).toHaveBeenCalledOnce());
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

  it('Ctrl+M abre uma vez o seletor de modelos do chat ativo', async () => {
    renderToolbar();
    const trigger = await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const event = dispatchModelShortcut();

    expect(event.defaultPrevented).toBe(true);
    expect(modelOpenMock).toHaveBeenCalledOnce();
    expect(trigger).toHaveAttribute('title', 'Ctrl+M');
  });

  it('em surfaces mantidas montadas, somente a ativa responde aos atalhos', async () => {
    render(
      <MemoryRouter>
        <ChatToolbar enableShortcuts={false} />
        <ChatToolbar enableShortcuts />
      </MemoryRouter>,
    );
    await waitFor(() => {
      expect(screen.getAllByRole('button', {
        name: 'chat.modelOverride.label, $default',
      })).toHaveLength(2);
    });

    dispatchModelShortcut();
    dispatchCtrlKey('h');
    dispatchCtrlKey('p');
    dispatchCtrlKey('l');

    expect(modelOpenMock).toHaveBeenCalledOnce();
    expect(historyClickMock).toHaveBeenCalledOnce();
    expect(profileClickMock).toHaveBeenCalledOnce();
    await waitFor(() => expect(clearConversationMock).toHaveBeenCalledOnce());
  });

  it('continua acionando os atalhos após Escape quando a superfície interrompe a propagação', async () => {
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const surface = document.createElement('div');
    surface.setAttribute('role', 'document');
    const trigger = document.createElement('button');
    const menu = document.createElement('div');
    const menuItem = document.createElement('button');
    menu.setAttribute('role', 'menu');
    menu.appendChild(menuItem);
    surface.append(trigger, menu);
    document.body.appendChild(surface);
    surface.addEventListener('keydown', (event) => {
      if (event.key === 'Escape') {
        menu.hidden = true;
        trigger.focus();
      }
      event.stopPropagation();
    });

    menuItem.focus();
    fireEvent.keyDown(menuItem, { key: 'Escape' });
    expect(trigger).toHaveFocus();

    dispatchCtrlKey('m', trigger);
    dispatchCtrlKey('h', trigger);
    dispatchCtrlKey('p', trigger);
    dispatchCtrlKey('l', trigger);

    expect(modelOpenMock).toHaveBeenCalledOnce();
    expect(historyClickMock).toHaveBeenCalledOnce();
    expect(profileClickMock).toHaveBeenCalledOnce();
    await waitFor(() => expect(clearConversationMock).toHaveBeenCalledOnce());

    surface.remove();
  });

  it('não intercepta Ctrl+M em editores, terminal, modal, menu ou picker aberto', async () => {
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const contentEditable = document.createElement('div');
    contentEditable.setAttribute('contenteditable', 'true');
    const targets: HTMLElement[] = [
      document.createElement('textarea'),
      document.createElement('input'),
      contentEditable,
    ];
    const monaco = document.createElement('div');
    monaco.className = 'monaco-editor';
    const monacoTarget = document.createElement('span');
    monaco.appendChild(monacoTarget);
    targets.push(monacoTarget);
    const terminal = document.createElement('div');
    terminal.className = 'xterm';
    const terminalTarget = document.createElement('span');
    terminal.appendChild(terminalTarget);
    targets.push(terminalTarget);
    targets.slice(0, 3).forEach((target) => document.body.appendChild(target));
    document.body.append(monaco, terminal);

    targets.forEach((target) => {
      expect(dispatchModelShortcut(target).defaultPrevented).toBe(false);
    });

    modalState.open = true;
    expect(dispatchModelShortcut().defaultPrevented).toBe(false);
    modalState.open = false;

    const menu = document.createElement('div');
    menu.setAttribute('role', 'menu');
    document.body.appendChild(menu);
    expect(dispatchModelShortcut().defaultPrevented).toBe(false);
    menu.remove();

    const picker = document.createElement('div');
    picker.className = 'picker-dropdown';
    document.body.appendChild(picker);
    expect(dispatchModelShortcut().defaultPrevented).toBe(false);
    picker.remove();

    expect(modelOpenMock).not.toHaveBeenCalled();
    targets.forEach((target) => target.closest('body') && target.remove());
    monaco.remove();
    terminal.remove();
  });

  it('bloqueia listbox portalado visível, mas ignora o mesmo listbox oculto', async () => {
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const outsideFocus = document.createElement('button');
    const portalListbox = document.createElement('ul');
    portalListbox.setAttribute('role', 'listbox');
    document.body.append(outsideFocus, portalListbox);
    outsideFocus.focus();

    const blockedEvent = dispatchModelShortcut(outsideFocus);
    expect(blockedEvent.defaultPrevented).toBe(false);
    expect(dispatchCtrlKey('h', outsideFocus).defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('p', outsideFocus).defaultPrevented).toBe(true);
    expect(dispatchCtrlKey('l', outsideFocus).defaultPrevented).toBe(true);
    expect(modelOpenMock).not.toHaveBeenCalled();
    expect(historyClickMock).not.toHaveBeenCalled();
    expect(profileClickMock).not.toHaveBeenCalled();
    expect(clearConversationMock).not.toHaveBeenCalled();

    portalListbox.hidden = true;
    const normalEvent = dispatchModelShortcut(outsideFocus);
    expect(normalEvent.defaultPrevented).toBe(true);
    dispatchCtrlKey('h', outsideFocus);
    dispatchCtrlKey('p', outsideFocus);
    dispatchCtrlKey('l', outsideFocus);
    expect(modelOpenMock).toHaveBeenCalledOnce();
    expect(historyClickMock).toHaveBeenCalledOnce();
    expect(profileClickMock).toHaveBeenCalledOnce();
    await waitFor(() => expect(clearConversationMock).toHaveBeenCalledOnce());

    outsideFocus.remove();
    portalListbox.remove();
  });

  it('respeita evento tratado, IME, modificadores extras e repetição', async () => {
    renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });

    const prevented = new KeyboardEvent('keydown', {
      key: 'm',
      ctrlKey: true,
      bubbles: true,
      cancelable: true,
    });
    prevented.preventDefault();
    window.dispatchEvent(prevented);

    dispatchModelShortcut(window, { isComposing: true });
    dispatchModelShortcut(window, { shiftKey: true });
    dispatchModelShortcut(window, { altKey: true });
    dispatchModelShortcut(window, { metaKey: true });
    dispatchModelShortcut(window, { repeat: true });

    expect(modelOpenMock).not.toHaveBeenCalled();
  });

  it('remove o listener de Ctrl+M ao desmontar', async () => {
    const view = renderToolbar();
    await screen.findByRole('button', {
      name: 'chat.modelOverride.label, $default',
    });
    view.unmount();

    dispatchModelShortcut();

    expect(modelOpenMock).not.toHaveBeenCalled();
  });
});

// O seletor de modelo do agente precisa aparecer na barra da conversa aberta, e
// pela conversa dela: é o caminho que a pessoa usa para trocar de modelo, e uma
// ligação errada aqui trocaria o modelo de outra conversa (AEP-0084 D6).
describe('ChatToolbar e o modelo do agente', () => {
  beforeEach(() => {
    getAgentSessionOptionsMock.mockClear();
    getProfileMock.mockResolvedValue({
      chat: { llm_provider: 'agent-provider', model: 'modelo-a' },
    });
    getProvidersMock.mockResolvedValue([
      { id: 'agent-provider', api_format: 'acp', is_default: true },
    ]);
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

  it('mostra o seletor nativo quando a conversa não fala com agente', async () => {
    getProfileMock.mockResolvedValue({
      chat: { llm_provider: 'native-provider', model: 'modelo-perfil' },
    });
    getProvidersMock.mockResolvedValue([
      { id: 'native-provider', api_format: 'openai', is_default: true },
    ]);
    getAgentSessionOptionsMock.mockResolvedValueOnce({
      conversationId: 'conversation-1',
      available: false,
      options: [],
    });

    renderToolbar();

    await waitFor(() => expect(getAgentSessionOptionsMock).toHaveBeenCalledWith('conversation-1'));
    expect(await screen.findByRole('button', { name: 'chat.modelOverride.label, $default' })).toBeInTheDocument();
  });
});

describe('ChatToolbar e o modelo nativo da aba', () => {
  beforeEach(() => {
    getAgentSessionOptionsMock.mockResolvedValue({
      conversationId: 'conversation-1',
      available: false,
      options: [],
    });
  });

  it('persiste a escolha no ProfileOverride da aba', async () => {
    renderToolbar();
    await screen.findByRole('button', { name: 'chat.modelOverride.label, $default' });

    modelChangeRef.current?.('modelo-b');

    await waitFor(() => expect(updateTabMock).toHaveBeenCalledWith('tab-chat', {
      profile_override: { model: 'modelo-b' },
    }));
  });

  it('remove o override com nil ao voltar ao modelo do perfil', async () => {
    mockPanelTabRef.current = {
      id: 'tab-chat',
      title: 'Chat',
      type: 'chat',
      profileOverride: { slug: 'padrao', model: 'modelo-b' },
    } as unknown as Record<string, unknown>;
    renderToolbar();
    await screen.findByRole('button', { name: 'chat.modelOverride.label, modelo-b' });

    modelChangeRef.current?.('$default');

    await waitFor(() => expect(updateTabMock).toHaveBeenCalledWith('tab-chat', {
      profile_override: { model: null },
    }));
  });

  it('normaliza o modelo e trata entrada vazia como reset', async () => {
    renderToolbar();
    await screen.findByRole('button', { name: 'chat.modelOverride.label, $default' });

    modelChangeRef.current?.('  modelo-b  ');
    await waitFor(() => expect(updateTabMock).toHaveBeenNthCalledWith(1, 'tab-chat', {
      profile_override: { model: 'modelo-b' },
    }));

    modelChangeRef.current?.('   ');
    await waitFor(() => expect(updateTabMock).toHaveBeenNthCalledWith(2, 'tab-chat', {
      profile_override: { model: null },
    }));
  });

  it('limpa modelo incompatível ao trocar para perfil de outro provider', async () => {
    mockPanelTabRef.current = {
      id: 'tab-chat',
      title: 'Chat',
      type: 'chat',
      profileOverride: { slug: 'perfil-a', model: 'modelo-a' },
    } as unknown as Record<string, unknown>;
    getProfileMock.mockImplementation(async (slug: string) => ({
      chat: {
        llm_provider: slug === 'perfil-b' ? 'provider-b' : 'provider-a',
        model: 'modelo-perfil',
      },
    }));
    getProvidersMock.mockResolvedValue([
      { id: 'provider-a', api_format: 'openai', is_default: true },
      { id: 'provider-b', api_format: 'anthropic' },
    ]);
    renderToolbar();
    await waitFor(() => expect(profileChangeRef.current).not.toBeNull());

    profileChangeRef.current?.('perfil-b');

    await waitFor(() => expect(updateTabMock).toHaveBeenCalledWith('tab-chat', {
      profile_override: { slug: 'perfil-b', model: null },
    }));
  });

  it('preserva modelo compatível ao trocar perfil no mesmo provider', async () => {
    mockPanelTabRef.current = {
      id: 'tab-chat',
      title: 'Chat',
      type: 'chat',
      profileOverride: { slug: 'perfil-a', model: 'modelo-a' },
    } as unknown as Record<string, unknown>;
    getProfileMock.mockResolvedValue({
      chat: { llm_provider: 'provider-a', model: 'modelo-perfil' },
    });
    getProvidersMock.mockResolvedValue([
      { id: 'provider-a', api_format: 'openai', is_default: true },
    ]);
    renderToolbar();
    await waitFor(() => expect(profileChangeRef.current).not.toBeNull());

    profileChangeRef.current?.('perfil-b');

    await waitFor(() => expect(updateTabMock).toHaveBeenCalledWith('tab-chat', {
      profile_override: { slug: 'perfil-b' },
    }));
  });

  it('serializa seleções rápidas e aplica a última por último', async () => {
    let resolveFirst!: () => void;
    updateTabMock
      .mockImplementationOnce(() => new Promise<void>((resolve) => {
        resolveFirst = resolve;
      }))
      .mockResolvedValueOnce(undefined);
    renderToolbar();
    await screen.findByRole('button', { name: 'chat.modelOverride.label, $default' });

    act(() => {
      modelChangeRef.current?.('modelo-a');
      modelChangeRef.current?.('modelo-b');
    });

    await waitFor(() => expect(updateTabMock).toHaveBeenCalledTimes(1));
    expect(updateTabMock).toHaveBeenNthCalledWith(1, 'tab-chat', {
      profile_override: { model: 'modelo-a' },
    });

    await act(async () => {
      resolveFirst();
      await Promise.resolve();
    });
    await waitFor(() => expect(updateTabMock).toHaveBeenCalledTimes(2));
    expect(updateTabMock).toHaveBeenNthCalledWith(2, 'tab-chat', {
      profile_override: { model: 'modelo-b' },
    });
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
