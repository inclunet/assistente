import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfileEditorTabs } from './ProfileEditorTabs';

/* ── Mocks ─────────────────────────────────────────────── */

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: vi.fn(),
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
  ProfileGeneralSection: () => <div data-testid="general-section">General</div>,
}));

vi.mock('./ProfileChatSection', () => ({
  ProfileChatSection: () => <div data-testid="chat-section">Chat</div>,
}));

vi.mock('./ProfileSkillsSection', () => ({
  ProfileSkillsSection: () => <div data-testid="skills-section">Skills</div>,
}));

vi.mock('./ProfileToolsSection', () => ({
  ProfileToolsSection: () => <div data-testid="tools-section">Tools</div>,
}));

vi.mock('./ProfileAudioTab', () => ({
  ProfileAudioTab: () => <div data-testid="audio-tab">Audio</div>,
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

function renderTabs(overrides = {}) {
  const props = {
    editingProfile: { ...defaultProfile, ...overrides } as never,
    availableTools: [],
    availableSkills: [],
    availableAllowlists: [],
    updateField: vi.fn(),
    updateFields: vi.fn(),
  };
  return render(<ProfileEditorTabs {...props} />);
}

/* ── Testes ─────────────────────────────────────────────── */

describe('ProfileEditorTabs', () => {
  it('renderiza 5 abas', () => {
    renderTabs();
    const tabs = screen.getAllByRole('tab');
    expect(tabs).toHaveLength(5);
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

  it('navega com Ctrl+PageUp para aba anterior (wrap)', () => {
    renderTabs();

    const container = screen.getAllByRole('tab')[0].closest('[data-tab-scope]')!;
    fireEvent.keyDown(container, { key: 'PageUp', ctrlKey: true });

    // Wrap: do general (0) vai para audio (4)
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
