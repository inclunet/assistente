import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfileEditorTabs } from './ProfileEditorTabs';

/* ── Mocks ─────────────────────────────────────────────── */

const mockAnnounce = vi.fn();
const providersSpy = vi.fn();

vi.mock('@wailsjs/go/wailsapi/LLMProviders', () => ({
  GetLLMProvidersWithStatus: () => providersSpy(),
}));

beforeEach(() => {
  mockAnnounce.mockReset();
  providersSpy.mockReset();
  providersSpy.mockResolvedValue([
    { id: 'openai', api_format: 'openai', is_default: true },
    { id: 'cursor', api_format: 'acp' },
  ]);
});

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => mockAnnounce(...args),
  useAnnouncer: () => ({
    announce: mockAnnounce,
  }),
}));

vi.mock('@wailsjs/go/models', () => ({
  profiles: {
    Profile: class {
      static createFrom(source: unknown = {}) { return source; }
    },
  },
  main: {},
  allowlist: {},
  skills: {},
}));

vi.mock('./ProfileGeneralSection', () => ({
  ProfileGeneralSection: () => <button data-testid="general-section">General</button>,
}));

vi.mock('./ProfileChatSection', () => ({
  ProfileChatSection: () => <button data-testid="chat-section">Chat</button>,
}));

vi.mock('./ProfileSkillsSection', () => ({
  ProfileSkillsSection: () => <button data-testid="skills-section">Skills</button>,
}));

vi.mock('./ProfileContextProvidersSection', () => ({
  ProfileContextProvidersSection: () => <button data-testid="context-providers-section">Context Providers</button>,
}));

vi.mock('./ProfileToolsSection', () => ({
  ProfileToolsSection: ({ runtimeTools }: { runtimeTools?: string[] }) => (
    <button data-testid="tools-section" data-runtime-tools={runtimeTools?.join(',') ?? ''}>Tools</button>
  ),
}));

vi.mock('./ProfileAudioTab', () => ({
  ProfileAudioTab: () => <button data-testid="audio-tab">Audio</button>,
}));

/* ── Helper ────────────────────────────────────────────── */

const defaultProfile = {
  name: 'Test',
  description: '',
  icon: '',
  chat: {
    llm_provider: '',
    model: '',
    temperature: 0.7,
    max_tokens: 4096,
    max_tokens_mode: 'legacy',
    context_window: 0,
    max_context_messages: 0,
    min_context_messages: 0,
    top_p: 1.0,
    response_timeout: 180,
    reasoning_effort: '',
    enabled_skills: [],
    disable_on_demand_skills: false,
    disable_skills: false,
    enabled_tools: null,
    disable_tools: false,
    command_allowlist: '',
    max_agentic_iterations: 0,
  },
  voice: { provider: 'disabled', voice_id: '', rate: 1.0, volume: 1.0 },
  interaction: { stt_provider: 'webspeech', language: 'pt-BR', feedback_sounds: true },
};

function propsCom(overrides = {}) {
  return {
    editingProfile: { ...defaultProfile, ...overrides } as never,
    availableTools: [],
    availableSkills: [],
    availableContextProviders: [],
    availableAllowlists: [],
    updateField: vi.fn(),
    updateFields: vi.fn(),
  };
}

function renderTabs(overrides = {}) {
  return render(<ProfileEditorTabs {...propsCom(overrides)} />);
}

/** perfilCom monta o perfil apontando para um provedor. */
function perfilCom(provider: string) {
  return { chat: { ...defaultProfile.chat, llm_provider: provider } };
}

/* ── Testes ─────────────────────────────────────────────── */

describe('ProfileEditorTabs', () => {
  it('renderiza 6 abas', () => {
    renderTabs();
    const tabs = screen.getAllByRole('tab');
    expect(tabs).toHaveLength(6);
  });

  it('mostra aba Geral por padrão', () => {
    renderTabs();
    expect(screen.getByTestId('general-section')).toBeVisible();
    expect(screen.getByTestId('chat-section')).not.toBeVisible();
  });

  it('troca para aba Modelos ao clicar', async () => {
    const user = userEvent.setup();
    renderTabs();

    const modelsTab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'models');
    await user.click(modelsTab!);

    expect(screen.getByTestId('chat-section')).toBeVisible();
    expect(screen.getByTestId('general-section')).not.toBeVisible();
  });

  it('troca para aba Skills ao clicar', async () => {
    const user = userEvent.setup();
    renderTabs();

    const tab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'skills');
    await user.click(tab!);

    expect(screen.getByTestId('skills-section')).toBeInTheDocument();
  });

  it('troca para aba Tools ao clicar', async () => {
    const user = userEvent.setup();
    renderTabs();

    const tab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'tools');
    await user.click(tab!);

    expect(screen.getByTestId('tools-section')).toBeInTheDocument();
  });

  it('informa load_skill como runtime quando há skills sob demanda', async () => {
    const user = userEvent.setup();
    render(
      <ProfileEditorTabs
        {...propsCom({
          chat: {
            ...defaultProfile.chat,
            enabled_skills: ['coding', 'extra'],
          },
        })}
        availableSkills={[
          { slug: 'coding', name: 'Coding' },
          { slug: 'extra', name: 'Extra' },
        ] as never}
      />,
    );

    const tab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'tools');
    await user.click(tab!);

    expect(screen.getByTestId('tools-section')).toHaveAttribute('data-runtime-tools', 'load_skill');
  });

  it('não informa load_skill quando as skills sob demanda bloqueiam invocação pelo modelo', async () => {
    const user = userEvent.setup();
    render(
      <ProfileEditorTabs
        {...propsCom({
          chat: {
            ...defaultProfile.chat,
            enabled_skills: ['coding', 'manual'],
          },
        })}
        availableSkills={[
          { slug: 'coding', name: 'Coding' },
          { slug: 'manual', name: 'Manual', disableModelInvocation: true },
        ] as never}
      />,
    );

    const tab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'tools');
    await user.click(tab!);

    expect(screen.getByTestId('tools-section')).toHaveAttribute('data-runtime-tools', '');
  });

  it('não elege base uma auto_load que o modelo não pode invocar', async () => {
    const user = userEvent.setup();
    render(
      <ProfileEditorTabs
        {...propsCom({
          chat: {
            ...defaultProfile.chat,
            enabled_skills: null,
          },
        })}
        availableSkills={[
          { slug: 'base', name: 'Base', autoLoad: true, disableModelInvocation: true },
          { slug: 'extra', name: 'Extra', autoLoad: true },
        ] as never}
      />,
    );

    const tab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'tools');
    await user.click(tab!);

    // IsAutoLoad() é falso quando a invocação pelo modelo está desligada, então
    // a segunda skill vira base e não sobra nada sob demanda.
    expect(screen.getByTestId('tools-section')).toHaveAttribute('data-runtime-tools', '');
  });

  it('troca para aba Context Providers ao clicar', async () => {
    const user = userEvent.setup();
    renderTabs();

    const tab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'contextProviders');
    await user.click(tab!);

    expect(screen.getByTestId('context-providers-section')).toBeInTheDocument();
  });

  it('troca para aba Audio ao clicar', async () => {
    const user = userEvent.setup();
    renderTabs();

    const tab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'audio');
    await user.click(tab!);

    expect(screen.getByTestId('audio-tab')).toBeInTheDocument();
  });

  it('navega com Ctrl+PageDown para próxima aba', () => {
    renderTabs();

    const container = screen.getAllByRole('tab')[0].closest('[data-tab-scope]')!;
    fireEvent.keyDown(container, { key: 'PageDown', ctrlKey: true });

    expect(screen.getByTestId('chat-section')).toBeInTheDocument();
  });

  it.each([
    ['Ctrl+Tab', { key: 'Tab', ctrlKey: true }, 'chat-section'],
    ['Ctrl+Shift+Tab', { key: 'Tab', ctrlKey: true, shiftKey: true }, 'audio-tab'],
    ['Ctrl+PageDown', { key: 'PageDown', ctrlKey: true }, 'chat-section'],
    ['Ctrl+PageUp', { key: 'PageUp', ctrlKey: true }, 'audio-tab'],
  ])('%s restaura foco para o conteúdo da aba destino', async (_label, init, defaultTarget) => {
    renderTabs();

    const generalTab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'general')!;
    generalTab.focus();
    fireEvent.keyDown(generalTab, init);

    await waitFor(() => {
      expect(screen.getByTestId(defaultTarget)).toHaveFocus();
    });
  });

  it('anuncia a seção ao navegar com Ctrl+Tab', () => {
    mockAnnounce.mockClear();
    renderTabs();

    const generalTab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'general')!;
    generalTab.focus();
    fireEvent.keyDown(generalTab, { key: 'Tab', ctrlKey: true });

    expect(mockAnnounce).toHaveBeenCalledWith('profiles.editorTabs.models');
  });

  it('setas mantêm foco na tablist', () => {
    renderTabs();

    const generalTab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'general')!;
    generalTab.focus();
    fireEvent.keyDown(generalTab, { key: 'ArrowRight' });

    const modelsTab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'models')!;
    expect(modelsTab).toHaveFocus();
    expect(screen.getByTestId('general-section')).not.toHaveFocus();
  });

  it('Enter na guia leva foco para o conteúdo', async () => {
    renderTabs();

    const generalTab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'general')!;
    generalTab.focus();
    fireEvent.keyDown(generalTab, { key: 'Enter' });

    await waitFor(() => {
      expect(screen.getByTestId('general-section')).toHaveFocus();
    });
  });

  it('navega com Ctrl+PageUp para aba anterior (wrap)', () => {
    renderTabs();

    const container = screen.getAllByRole('tab')[0].closest('[data-tab-scope]')!;
    fireEvent.keyDown(container, { key: 'PageUp', ctrlKey: true });

    // Wrap: do general (0) vai para audio (5)
    expect(screen.getByTestId('audio-tab')).toBeInTheDocument();
  });

  it('tem aria-label na tablist', () => {
    renderTabs();
    const tablist = screen.getByRole('tablist');
    expect(tablist).toHaveAttribute('aria-label');
  });

  it('aba ativa tem aria-selected=true', () => {
    renderTabs();
    const activeTab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'general');
    expect(activeTab).toHaveAttribute('aria-selected', 'true');
  });
});

/* ── Perfil com agente de código (AEP-0084, Fase 8) ─────── */

const valoresDasAbas = () =>
  screen.getAllByRole('tab').map(tab => tab.getAttribute('data-tab-value'));

describe('ProfileEditorTabs com provedor de agente', () => {
  // Guia que não muda nada é pior do que guia ausente: quem a preenche fica
  // achando que ajustou o comportamento do turno.
  it('esconde as guias que o turno do agente ignora', async () => {
    renderTabs(perfilCom('cursor'));

    await waitFor(() => expect(valoresDasAbas()).toEqual(['general', 'models', 'audio']));
    expect(screen.queryByTestId('skills-section')).toBeNull();
    expect(screen.queryByTestId('tools-section')).toBeNull();
    expect(screen.queryByTestId('context-providers-section')).toBeNull();
  });

  it('provedor http mantém as guias todas', async () => {
    renderTabs(perfilCom('openai'));

    await waitFor(() => expect(providersSpy).toHaveBeenCalled());
    expect(valoresDasAbas()).toHaveLength(6);
    expect(screen.getByTestId('tools-section')).toBeInTheDocument();
  });

  // A contagem de guias muda debaixo de quem navega por teclado: a troca de
  // provedor precisa ser dita.
  it('trocar para agente anuncia o que passou a valer', async () => {
    const { rerender } = render(<ProfileEditorTabs {...propsCom(perfilCom('openai'))} />);
    await waitFor(() => expect(valoresDasAbas()).toHaveLength(6));

    rerender(<ProfileEditorTabs {...propsCom(perfilCom('cursor'))} />);

    await waitFor(() => expect(mockAnnounce).toHaveBeenCalledWith('profiles.agentProfile.tabsHidden'));
  });

  it('voltar para provedor http anuncia a volta das guias', async () => {
    const { rerender } = render(<ProfileEditorTabs {...propsCom(perfilCom('cursor'))} />);
    await waitFor(() => expect(valoresDasAbas()).toHaveLength(3));

    rerender(<ProfileEditorTabs {...propsCom(perfilCom('openai'))} />);

    await waitFor(() => expect(mockAnnounce).toHaveBeenCalledWith('profiles.agentProfile.tabsBack'));
    expect(valoresDasAbas()).toHaveLength(6);
  });

  // Abrir um perfil que já era de um agente não é uma troca: o editor está
  // mostrando o perfil como ele é, e ninguém precisa ouvir que guias sumiram.
  it('abrir um perfil que já é de agente não anuncia troca', async () => {
    renderTabs(perfilCom('cursor'));

    await waitFor(() => expect(valoresDasAbas()).toHaveLength(3));
    expect(mockAnnounce).not.toHaveBeenCalledWith('profiles.agentProfile.tabsHidden');
    expect(mockAnnounce).not.toHaveBeenCalledWith('profiles.agentProfile.tabsBack');
  });

  // Quem estava na guia que sumiu não pode ficar num painel que não existe
  // mais: o foco vai para lugar previsível.
  it('quem estava na guia que sumiu vai para Modelos', async () => {
    const user = userEvent.setup();
    const { rerender } = render(<ProfileEditorTabs {...propsCom(perfilCom('openai'))} />);
    await waitFor(() => expect(valoresDasAbas()).toHaveLength(6));

    const toolsTab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'tools')!;
    await user.click(toolsTab);
    expect(screen.getByTestId('tools-section')).toBeInTheDocument();

    rerender(<ProfileEditorTabs {...propsCom(perfilCom('cursor'))} />);

    await waitFor(() => {
      const modelsTab = screen.getAllByRole('tab').find(t => t.getAttribute('data-tab-value') === 'models');
      expect(modelsTab).toHaveAttribute('aria-selected', 'true');
    });
  });

  // Esconder guia não é apagar configuração: o perfil continua com o que
  // tinha, e voltar para um provedor HTTP traz tudo de volta.
  it('esconder guia não mexe no perfil', async () => {
    const props = propsCom(perfilCom('cursor'));
    render(<ProfileEditorTabs {...props} />);

    await waitFor(() => expect(valoresDasAbas()).toHaveLength(3));
    expect(props.updateField).not.toHaveBeenCalled();
    expect(props.updateFields).not.toHaveBeenCalled();
  });

  // Sem resposta da consulta, o editor segue completo: esconder guia por causa
  // de uma consulta que falhou tiraria da pessoa configuração que ela tem.
  it('consulta que falha deixa o editor como estava', async () => {
    providersSpy.mockRejectedValueOnce(new Error('sem resposta'));

    renderTabs(perfilCom('cursor'));

    await waitFor(() => expect(providersSpy).toHaveBeenCalled());
    expect(valoresDasAbas()).toHaveLength(6);
  });

  it('Ctrl+Tab não passa pelas guias escondidas', async () => {
    renderTabs(perfilCom('cursor'));
    await waitFor(() => expect(valoresDasAbas()).toHaveLength(3));

    const container = screen.getAllByRole('tab')[0].closest('[data-tab-scope]')!;
    // De Geral (0) para Modelos (1), e de Modelos direto para Áudio: as três
    // guias do meio não existem mais.
    fireEvent.keyDown(container, { key: 'Tab', ctrlKey: true });
    fireEvent.keyDown(container, { key: 'Tab', ctrlKey: true });

    expect(mockAnnounce).toHaveBeenCalledWith('profiles.editorTabs.audio');
  });
});
