import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ReactNode } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockDuplicateProfile = vi.fn();

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
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
  SetActiveProfile: vi.fn().mockResolvedValue(undefined),
  CreateProfile: vi.fn().mockResolvedValue('novo-perfil'),
  UpdateProfile: vi.fn().mockResolvedValue(undefined),
  DeleteProfile: vi.fn().mockResolvedValue(undefined),
  DuplicateProfile: (slug: string) => mockDuplicateProfile(slug),
  GetLLMProviders: vi.fn().mockResolvedValue([]),
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
  GetModels: vi.fn().mockResolvedValue([]),
  GetOpenAITTSVoices: vi.fn().mockResolvedValue([]),
  GetLLMProvidersWithStatus: vi.fn().mockResolvedValue([]),
  GetSpeechProviders: vi.fn().mockResolvedValue([]),
  GetSTTModels: vi.fn().mockResolvedValue([]),
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
import { GetProfile } from '@wailsjs/go/app/App';

describe('ProfilesPage', { timeout: 60_000 }, () => {
  beforeEach(() => {
    mockDuplicateProfile.mockReset();
    mockAddToast.mockReset();
    mockAnnounce.mockReset();
    mockDuplicateProfile.mockResolvedValue('perfil-padrao-copia');
  });

  it('abre editor ao criar novo perfil e renderiza abas do editor', async () => {
    const user = userEvent.setup();
    render(<ProfilesPage />);

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

  it('duplica um perfil via menu de acoes', async () => {
    const user = userEvent.setup();
    render(<ProfilesPage />);

    await waitFor(() => {
      expect(screen.getByText('Perfil Padrão')).toBeInTheDocument();
    });

    const duplicateButtons = screen.getAllByRole('button', { name: 'Duplicar' });
    await user.click(duplicateButtons[duplicateButtons.length - 1]);

    await waitFor(() => {
      expect(mockDuplicateProfile).toHaveBeenCalledWith('padrao');
    });

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith('Perfil duplicado!', 'success');
    });

    await waitFor(() => {
      expect(vi.mocked(GetProfile)).toHaveBeenCalledWith('perfil-padrao-copia');
    });
  });
});
