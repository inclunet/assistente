import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfileEditorTabs } from './ProfileEditorTabs';

/* ── Mocks ─────────────────────────────────────────────── */

const mockAnnounce = vi.fn();

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
  ProfileToolsSection: () => <button data-testid="tools-section">Tools</button>,
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

function renderTabs(overrides = {}) {
  const props = {
    editingProfile: { ...defaultProfile, ...overrides } as never,
    availableTools: [],
    availableSkills: [],
    availableContextProviders: [],
    availableAllowlists: [],
    updateField: vi.fn(),
    updateFields: vi.fn(),
  };
  return render(<ProfileEditorTabs {...props} />);
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
