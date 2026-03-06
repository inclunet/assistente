import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

vi.mock('@wailsjs/go/main/App', () => ({
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
      provider: 'disabled',
      voice_id: '',
      rate: 1.0,
      pitch: 1.0,
      volume: 1.0,
      enabled_for_agent: false,
      enabled_for_user: false,
    },
    interaction: {
      stt_provider: 'webspeech',
      language: 'pt-BR',
      feedback_sounds: true,
      triggers: [],
    },
  }),
  GetModels: vi.fn().mockResolvedValue([]),
  GetSAPI5Voices: vi.fn().mockResolvedValue([]),
  GetOpenAITTSVoices: vi.fn().mockResolvedValue([]),
}));

vi.mock('@wailsjs/go/models', () => ({
  profiles: {
    Profile: class {
      static createFrom(source: any = {}) {
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
    focusFirstCell: vi.fn(),
    handleGridReady: vi.fn(),
  }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: vi.fn(),
  }),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: () => ({
    addToast: vi.fn(),
  }),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, actions }: any) => (
    <div>
      {left}
      <div>
        {actions?.map((action: any) => (
          <button key={action.key} onClick={action.onClick}>
            {action.label}
          </button>
        ))}
      </div>
    </div>
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({ items }: any) => (
    <div>
      {items?.map((item: any) => (
        <div key={item.id}>{item.name}</div>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/Modal', () => ({
  Modal: ({ isOpen, children }: any) => (isOpen ? <div>{children}</div> : null),
}));

vi.mock('../components', () => ({
  Button: ({ onClick, children, loading, ...rest }: any) => (
    <button onClick={onClick} disabled={loading} {...rest}>
      {children}
    </button>
  ),
}));

vi.mock('../components/ui/EditorPanel', () => ({
  EditorPanelFooter: ({ children, className }: any) => (
    <div className={className}>{children}</div>
  ),
}));

describe('ProfilesPage', () => {
  it('abre editor ao criar novo perfil e renderiza seções principais', async () => {
    const user = userEvent.setup();
    const { default: ProfilesPage } = await import('./ProfilesPage');
    render(<ProfilesPage />);

    const newButton = await screen.findByRole('button', { name: 'Novo Perfil' });
    await user.click(newButton);

    await waitFor(() => {
      expect(screen.getByTestId('profile-general-section')).toBeInTheDocument();
    });

    expect(screen.getByTestId('profile-chat-section')).toBeInTheDocument();
    expect(screen.getByText('Skills')).toBeInTheDocument();
    expect(screen.getByText('Ferramentas (Tool Calling)')).toBeInTheDocument();
    expect(screen.getByText('Voz (TTS)')).toBeInTheDocument();
    expect(screen.getByText('Interação (STT)')).toBeInTheDocument();
    expect(screen.getByText('Nenhum skill encontrado.')).toBeInTheDocument();
    expect(screen.getByText('Nenhuma ferramenta encontrada.')).toBeInTheDocument();
  });
});
