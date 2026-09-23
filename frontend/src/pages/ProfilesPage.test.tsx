import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ReactNode } from 'react';
import { useEffect } from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { PageMutationResult } from '../lib/commandPageMutation';

const mockReadProfileCommandTarget = vi.fn();
const mockRequestFormPageMutation = vi.fn();
const mockRequestRootPageMutation = vi.fn();
let formMutationOptions: any;
let rootMutationOptions: any;
const mockNavigate = vi.fn();
const mockSetActiveTab = vi.fn();
const mockRequestOpen = vi.fn();
let mockConversationId = 'conversation-1';

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useLocation: () => ({ pathname: '/profiles' }),
}));

vi.mock('../store/authStore', () => ({
  useAuthStore: Object.assign(
    (selector?: (state: Record<string, unknown>) => unknown) => {
      const state = { isAuthenticated: true, user: { userId: 'user-1', sessionId: 'session-1' } };
      return selector ? selector(state) : state;
    },
    {
      getState: () => ({ isAuthenticated: true, user: { userId: 'user-1', sessionId: 'session-1' } }),
      subscribe: () => () => {},
    },
  ),
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector?: (state: Record<string, unknown>) => unknown) => {
      const state = {
        workspace: { id: 'workspace-1', activeTabId: 'profiles-tab', tabs: [{ id: 'chat-tab', type: 'chat', conversationId: mockConversationId }] },
        setActiveTab: mockSetActiveTab,
      };
      return selector ? selector(state) : state;
    },
    {
      getState: () => ({
        workspace: { id: 'workspace-1', activeTabId: 'profiles-tab', tabs: [{ id: 'chat-tab', type: 'chat', conversationId: mockConversationId }] },
        setActiveTab: mockSetActiveTab,
      }),
      subscribe: () => () => {},
    },
  ),
}));

vi.mock('../store/workspaceChatModalStore', () => ({
  useWorkspaceChatModalStore: {
    getState: () => ({ requestOpen: mockRequestOpen }),
  },
}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({
  GetProfiles: vi.fn().mockResolvedValue([
    {
      slug: 'padrao',
      name: 'Perfil Padrão',
      description: '',
      icon: 'chatbox',
      source: 'home',
    },
  ]),
  GetActiveProfileSlug: vi.fn().mockResolvedValue('padrao'),
  GetProfileSearchPaths: vi.fn().mockResolvedValue([]),
  GetProfile: vi.fn().mockResolvedValue({
    name: 'Perfil Padrão',
    description: '',
    icon: 'chatbox',
    chat: {
      model: '',
      temperature: 0.7,
      max_tokens: 4096,
      top_p: 1.0,
      response_timeout: 180,
      reasoning_effort: '',
    },
    voice: {
      assistant: {
        enabled: false,
        provider: 'disabled',
        voice_id: '',
        rate: 1.0,
        pitch: 1.0,
        volume: 1.0,
      },
      user: {
        enabled: false,
        provider: 'disabled',
        rate: 1.0,
        pitch: 1.0,
        volume: 1.0,
      },
      system: {
        enabled: false,
        provider: 'disabled',
        rate: 1.0,
        pitch: 1.0,
        volume: 1.0,
      },
    },
    input: {
      enabled: true,
      stt_provider: 'webspeech',
      language: 'pt-BR',
      feedback_sounds: true,
      triggers: [],
    },
    channels: {
      response_mode: 'mirror',
    },
  }),
}));

vi.mock('../lib/commandPageMutationWails', () => ({
  readProfileCommandTarget: (...args: unknown[]) => mockReadProfileCommandTarget(...args),
}));

vi.mock('../lib/commandPageMutation', () => ({
  usePageMutationCommands: (options: any) => {
    if (options.allowedCommands.includes('profiles.create')) {
      formMutationOptions = options;
      return { request: mockRequestFormPageMutation };
    }
    rootMutationOptions = options;
    return { request: mockRequestRootPageMutation };
  },
}));

vi.mock('@wailsjs/go/wailsapi/LLMModels', () => ({
  GetModels: vi.fn().mockResolvedValue([]),
}));

vi.mock('@wailsjs/go/wailsapi/Speech', () => ({
  GetOpenAITTSVoices: vi.fn().mockResolvedValue([]),
  GetSpeechProviders: vi.fn().mockResolvedValue([]),
  GetSTTModels: vi.fn().mockResolvedValue([]),
}));

vi.mock('@wailsjs/go/wailsapi/LLMProviders', () => ({
  GetLLMProviders: vi.fn().mockResolvedValue([]),
  GetLLMProvidersWithStatus: vi.fn().mockResolvedValue([]),
}));

vi.mock('@wailsjs/go/wailsapi/Settings', () => ({
  GetNativeTTSProviders: vi.fn().mockResolvedValue(['webspeech', 'sapi5']),
}));

vi.mock('@wailsjs/go/models', () => ({
  profiles: {
    Profile: class {
      static createFrom(source: unknown = {}) {
        return source;
      }
    },
  },
}));

vi.mock('../hooks/useProfileDependencies', () => ({
  useProfileDependencies: () => ({
    tools: [],
    skills: [],
    allowlists: [],
    contextProviders: [],
    loading: false,
  }),
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    handleGridReady: vi.fn(),
  }),
}));

vi.mock('../hooks/useGridPageLandmarks', () => ({
  useGridPageLandmarks: vi.fn(),
}));

const mockAnnounce = vi.fn();
vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: mockAnnounce,
    announceRequest: vi.fn(() => true),
  }),
}));

const mockAddToast = vi.fn();
vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { addToast: mockAddToast };
    return selector ? selector(s) : s;
  },
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, actions }: { left?: ReactNode; actions?: Array<{ key: string; label: string; onClick: () => void }> }) => (
    <div>
      {left}
      <div>
        {actions?.map((action) => (
          <button key={action.key} onClick={action.onClick}>
            {action.label}
          </button>
        ))}
      </div>
    </div>
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({
    items,
    getRowActions,
  }: {
    items?: Array<{ id: string; name: string }>;
    getRowActions?: (item: { id: string; name: string }) => Array<{ id: string; label: string; onClick: () => void }>;
  }) => (
    <div>
      {items?.map((item) => (
        <div key={item.id}>
          <span>{item.name}</span>
          {getRowActions?.(item)?.map((action) => (
            <button key={action.id} onClick={action.onClick}>
              {action.label}
            </button>
          ))}
        </div>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/Modal', () => ({
  Modal: ({ isOpen, children }: { isOpen: boolean; children: ReactNode }) => (isOpen ? <div>{children}</div> : null),
  isModalOpen: () => false,
}));

vi.mock('../components', () => ({
  Button: ({
    onClick,
    children,
    loading,
    ...rest
  }: {
    onClick?: () => void;
    children?: ReactNode;
    loading?: boolean;
  }) => (
    <button onClick={onClick} disabled={loading} {...rest}>
      {children}
    </button>
  ),
  PageLoading: ({ message }: { message?: string }) => <div role="status">{message}</div>,
}));

vi.mock('../components/ui/EditorPanel', () => ({
  EditorPanelFooter: ({ children, className }: { children: ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
}));

import ProfilesPage from './ProfilesPage';
import { GetProfile } from '@wailsjs/go/wailsapi/Profiles';
import { useNavigationStore } from '../store/navigationStore';
import { capturePagePresentationTarget, PAGE_PRESENTATION_COMMAND_EVENT } from '../lib/commandPagePresentation';
import { act } from 'react';

type ProfileLoadResult = Awaited<ReturnType<typeof GetProfile>>;

function mutationResult(commandId: string, request: { targetId?: string; profile?: { name?: string } }): PageMutationResult {
  if (commandId === 'profiles.duplicate') return { id: 'perfil-padrao-copia', title: 'Perfil Padrão (Cópia)' };
  if (commandId === 'profiles.create') return { id: 'novo-perfil', title: request.profile?.name || 'Novo Perfil' };
  return { id: request.targetId || 'padrao', title: request.profile?.name || 'Perfil Padrão' };
}

async function executeMockPageMutation(commandId: string, options: any) {
  const prepared = options.prepare(commandId);
  if (!prepared) return { status: 'cancelled' };
  const request = await prepared.readRequest();
  const result = mutationResult(commandId, request);
  await prepared.succeeded?.(result);
  return { status: 'succeeded', result };
}

function PresentationHarness({ pathname = '/profiles' }: { pathname?: string }) {
  useEffect(() => {
    const handle = (event: Event) => {
      const detail = (event as CustomEvent<{ commandID?: string; instanceId?: string }>).detail;
      if (!detail?.commandID || !detail.instanceId) return;
      const target = capturePagePresentationTarget(() => pathname, detail.commandID, detail.instanceId);
      if (!target) return;
      if (target.open(detail.commandID as never)) event.preventDefault();
    };
    window.addEventListener(PAGE_PRESENTATION_COMMAND_EVENT, handle);
    return () => window.removeEventListener(PAGE_PRESENTATION_COMMAND_EVENT, handle);
  }, [pathname]);
  return null;
}

function renderProfilesPage() {
  return render(<><PresentationHarness /><ProfilesPage /></>);
}

describe('ProfilesPage', { timeout: 60_000 }, () => {
  beforeEach(() => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    mockAddToast.mockReset();
    mockAnnounce.mockReset();
    mockNavigate.mockReset();
    mockSetActiveTab.mockReset();
    mockRequestOpen.mockReset();
    mockConversationId = 'conversation-1';
    useNavigationStore.getState().clearPendingEdit();
    mockReadProfileCommandTarget.mockReset();
    mockReadProfileCommandTarget.mockImplementation(async (slug: string) => ({
      profile: slug ? await GetProfile(slug) : undefined,
      fingerprint: slug ? `fp-${slug}` : 'fp-create',
    }));
    mockRequestFormPageMutation.mockReset();
    mockRequestRootPageMutation.mockReset();
    mockRequestFormPageMutation.mockImplementation((commandId: string) => executeMockPageMutation(commandId, formMutationOptions));
    mockRequestRootPageMutation.mockImplementation((commandId: string) => executeMockPageMutation(commandId, rootMutationOptions));
  });

  it('abre editor ao criar novo perfil e renderiza abas do editor', async () => {
    const user = userEvent.setup();
    renderProfilesPage();

    const newButton = await screen.findByRole('button', { name: 'Novo Perfil' });
    await user.click(newButton);


    // Aba "Geral" é a padrão — ProfileGeneralSection visível
    await waitFor(() => {
      expect(screen.getByTestId('profile-general-section')).toBeInTheDocument();
    });

    // Todas as abas do editor devem estar visíveis como botões
    const tabs = screen.getAllByRole('tab');
    expect(tabs.length).toBeGreaterThanOrEqual(5);

    // Clica na aba "Modelos" — ProfileChatSection deve aparecer
    const modelsTab = tabs.find((t) => t.getAttribute('data-tab-value') === 'models');
    expect(modelsTab).toBeTruthy();
    await user.click(modelsTab!);

    await waitFor(() => {
      expect(screen.getByTestId('profile-chat-section')).toBeInTheDocument();
    });

    // Clica na aba "Skills"
    const skillsTab = tabs.find((t) => t.getAttribute('data-tab-value') === 'skills');
    expect(skillsTab).toBeTruthy();
    await user.click(skillsTab!);

    await waitFor(() => {
      expect(screen.getByText('Nenhum skill encontrado.')).toBeInTheDocument();
    });

    // Clica na aba "Tools"
    const toolsTab = tabs.find((t) => t.getAttribute('data-tab-value') === 'tools');
    expect(toolsTab).toBeTruthy();
    await user.click(toolsTab!);

    await waitFor(() => {
      expect(screen.getByText('Nenhuma ferramenta encontrada.')).toBeInTheDocument();
    });

    // Clica na aba "Audio"
    const audioTab = tabs.find((t) => t.getAttribute('data-tab-value') === 'audio');
    expect(audioTab).toBeTruthy();
    await user.click(audioTab!);

    await waitFor(() => {
      expect(screen.getByText('Voz (TTS)')).toBeInTheDocument();
      expect(screen.getByText('Entrada de Voz (STT)')).toBeInTheDocument();
    });
  });

  it('não reabre criação por Ctrl+N legado fora do registry', async () => {
    renderProfilesPage();

    await waitFor(() => expect(screen.getByText('Perfil Padrão')).toBeInTheDocument());

    fireEvent.keyDown(window, { key: 'n', ctrlKey: true });

    expect(screen.queryByRole('button', { name: 'Salvar' })).not.toBeInTheDocument();
  });

  it('usa no create o fingerprint capturado ao abrir o formulário', async () => {
    const user = userEvent.setup();
    renderProfilesPage();

    await user.click(await screen.findByRole('button', { name: 'Novo Perfil' }));
    await screen.findByRole('button', { name: 'Salvar' });
    await waitFor(() => expect(mockReadProfileCommandTarget).toHaveBeenCalledWith(''));

    const readsAtOpen = mockReadProfileCommandTarget.mock.calls.filter(([slug]) => slug === '').length;
    await user.click(screen.getByRole('button', { name: 'Salvar' }));

    await waitFor(() => expect(mockRequestFormPageMutation).toHaveBeenCalledWith('profiles.create'));
    expect(mockReadProfileCommandTarget.mock.calls.filter(([slug]) => slug === '')).toHaveLength(readsAtOpen);
  });

  it('descarta carregamento de edição pendente quando Novo Perfil invalida o contexto', async () => {
    const user = userEvent.setup();
    let resolveProfile!: (profile: ProfileLoadResult) => void;
    vi.mocked(GetProfile).mockImplementationOnce(
      () => new Promise((resolve) => {
        resolveProfile = resolve;
      }),
    );

    renderProfilesPage();
    await waitFor(() => expect(screen.getByText('Perfil Padrão')).toBeInTheDocument());

    const editButtons = screen.getAllByRole('button', { name: 'Editar perfil' });
    await user.click(editButtons[editButtons.length - 1]);
    await user.click(await screen.findByRole('button', { name: 'Novo Perfil' }));

    expect(screen.getByDisplayValue('Novo Perfil')).toBeInTheDocument();

    const staleProfile = {
      name: 'Perfil Padrão',
      description: 'resposta atrasada',
      icon: 'chatbox',
      chat: { model: '', temperature: 0.7, max_tokens: 4096, top_p: 1.0, response_timeout: 180 },
      voice: {},
      input: { enabled: true, stt_provider: 'webspeech', language: 'pt-BR', feedback_sounds: true, triggers: [] },
      channels: { response_mode: 'mirror' },
    } as unknown as ProfileLoadResult;
    await act(async () => {
      resolveProfile(staleProfile);
      await Promise.resolve();
    });

    await waitFor(() => expect(screen.getByDisplayValue('Novo Perfil')).toBeInTheDocument());
    expect(screen.queryByDisplayValue('resposta atrasada')).not.toBeInTheDocument();
  });

  it.each(['blur', 'compositionstart'] as const)(
    'descarta GetProfile pendente após %s sem abrir editor',
    async (invalidation) => {
      const user = userEvent.setup();
      let resolveProfile!: (profile: ProfileLoadResult) => void;
      vi.mocked(GetProfile).mockImplementationOnce(
        () => new Promise((resolve) => {
          resolveProfile = resolve;
        }),
      );

      renderProfilesPage();
      await waitFor(() => expect(screen.getByText('Perfil Padrão')).toBeInTheDocument());
      const editButtons = screen.getAllByRole('button', { name: 'Editar perfil' });
      await user.click(editButtons[editButtons.length - 1]);

      if (invalidation === 'blur') {
        fireEvent.blur(window);
      } else {
        fireEvent.compositionStart(document);
      }

      const staleProfile = {
        name: 'Perfil Padrão',
        description: 'resposta após invalidação',
        icon: 'chatbox',
        chat: { model: '', temperature: 0.7, max_tokens: 4096, top_p: 1.0, response_timeout: 180 },
        voice: {},
        input: { enabled: true, stt_provider: 'webspeech', language: 'pt-BR', feedback_sounds: true, triggers: [] },
        channels: { response_mode: 'mirror' },
      } as unknown as ProfileLoadResult;
      await act(async () => {
        resolveProfile(staleProfile);
        await Promise.resolve();
      });

      await waitFor(() => {
        expect(screen.queryByRole('button', { name: 'Salvar' })).not.toBeInTheDocument();
      });
      expect(screen.queryByDisplayValue('resposta após invalidação')).not.toBeInTheDocument();
    },
  );

  it('duplica um perfil via menu de acoes', async () => {
    const user = userEvent.setup();
    renderProfilesPage();

    await waitFor(() => {
      expect(screen.getByText('Perfil Padrão')).toBeInTheDocument();
    });

    const duplicateButtons = screen.getAllByRole('button', { name: 'Duplicar' });
    await user.click(duplicateButtons[duplicateButtons.length - 1]);

    await waitFor(() => {
      expect(mockRequestRootPageMutation).toHaveBeenCalledWith('profiles.duplicate');
    });

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith('Perfil duplicado!', 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
    });

    await waitFor(() => {
      expect(vi.mocked(GetProfile)).toHaveBeenCalledWith('perfil-padrao-copia');
    });
  });

  it('abre voz por navegação e retorna à aba de origem ao cancelar', async () => {
    const user = userEvent.setup();
    useNavigationStore.getState().requestResourceEdit('profiles', 'padrao', 'edit', {
      tab: 'voice',
      caller: {
        kind: 'workspace',
        tabId: 'chat-tab',
        surfaceId: 'page:tab:chat-tab',
        surfaceType: 'page',
        conversationId: 'conversation-1',
      },
    });

    renderProfilesPage();

    await waitFor(() => {
      const voiceTab = screen.getAllByRole('tab').find(
        (tab) => tab.getAttribute('data-tab-value') === 'audio',
      );
      expect(voiceTab).toHaveAttribute('aria-selected', 'true');
    });

    await user.click(screen.getByRole('button', { name: 'Cancelar' }));

    expect(mockNavigate).toHaveBeenCalledWith('/');
    await waitFor(() => expect(mockSetActiveTab).toHaveBeenCalledWith('chat-tab'));
  });

  it('retorna à aba que contém a superfície embedded ao cancelar', async () => {
    const user = userEvent.setup();
    useNavigationStore.getState().requestResourceEdit('profiles', 'padrao', 'edit', {
      tab: 'voice',
      caller: {
        kind: 'workspace',
        tabId: 'chat-tab',
        surfaceId: 'embedded:editor:chat-tab',
        surfaceType: 'embedded',
        conversationId: 'conversation-1',
      },
    });

    renderProfilesPage();
    await screen.findByRole('button', { name: 'Cancelar' });
    await user.click(screen.getByRole('button', { name: 'Cancelar' }));

    expect(mockNavigate).toHaveBeenCalledWith('/');
    await waitFor(() => expect(mockSetActiveTab).toHaveBeenCalledWith('chat-tab'));
    expect(mockRequestOpen).not.toHaveBeenCalled();
  });

  it('retorna à aba de origem depois de salvar o perfil', async () => {
    const user = userEvent.setup();
    useNavigationStore.getState().requestResourceEdit('profiles', 'padrao', 'edit', {
      tab: 'voice',
      caller: {
        kind: 'workspace',
        tabId: 'chat-tab',
        surfaceId: 'page:tab:chat-tab',
        surfaceType: 'page',
        conversationId: 'conversation-1',
      },
    });

    renderProfilesPage();
    await screen.findByRole('button', { name: 'Salvar' });
    await user.click(screen.getByRole('button', { name: 'Salvar' }));

    await waitFor(() => expect(mockRequestFormPageMutation).toHaveBeenCalledWith('profiles.update'));
    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/'));
    await waitFor(() => expect(mockSetActiveTab).toHaveBeenCalledWith('chat-tab'));
  });

  it('impede cancelar ou fechar enquanto o perfil está sendo salvo', async () => {
    const user = userEvent.setup();
    let finishUpdate!: () => void;
    mockRequestFormPageMutation.mockImplementationOnce(
      () => new Promise<{ status: string; result?: PageMutationResult }>((resolve) => {
        finishUpdate = () => {
          void executeMockPageMutation('profiles.update', formMutationOptions).then(resolve);
        };
      }),
    );
    useNavigationStore.getState().requestResourceEdit('profiles', 'padrao', 'edit', {
      tab: 'voice',
      caller: {
        kind: 'workspace',
        tabId: 'chat-tab',
        surfaceId: 'page:tab:chat-tab',
        surfaceType: 'page',
        conversationId: 'conversation-1',
      },
    });

    renderProfilesPage();
    await user.click(await screen.findByRole('button', { name: 'Salvar' }));

    const cancelButton = screen.getByRole('button', { name: 'Cancelar' });
    await waitFor(() => expect(cancelButton).toBeDisabled());
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(mockNavigate).not.toHaveBeenCalled();
    expect(cancelButton).toBeInTheDocument();

    finishUpdate();
    await waitFor(() => expect(mockNavigate).toHaveBeenCalledWith('/'));
    await waitFor(() => expect(mockSetActiveTab).toHaveBeenCalledWith('chat-tab'));
  });

  it('permanece em perfis quando o contexto de origem ficou inválido', async () => {
    const user = userEvent.setup();
    useNavigationStore.getState().requestResourceEdit('profiles', 'padrao', 'edit', {
      tab: 'voice',
      caller: {
        kind: 'workspace',
        tabId: 'chat-tab',
        surfaceId: 'page:tab:chat-tab',
        surfaceType: 'page',
        conversationId: 'conversation-original',
      },
    });

    renderProfilesPage();
    await screen.findByRole('button', { name: 'Cancelar' });
    await user.click(screen.getByRole('button', { name: 'Cancelar' }));

    expect(mockNavigate).not.toHaveBeenCalled();
    expect(mockSetActiveTab).not.toHaveBeenCalled();
  });
});
