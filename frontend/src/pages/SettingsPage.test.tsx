import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

let mockTab: string | undefined;
const mockNavigate = vi.fn();
const mockAnnounce = vi.fn();

vi.mock('../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => mockAnnounce(...args),
}));

vi.mock('react-router-dom', () => ({
  useParams: () => ({ tab: mockTab }),
  useNavigate: () => mockNavigate,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) =>
      ({
        'settingsPage.tabListLabel': 'Configurações',
        'settingsPage.tabs.providers': 'Provedores LLM',
        'settingsPage.tabs.mcp': 'MCP',
        'settingsPage.tabs.skills': 'Skills',
        'settingsPage.tabs.channels': 'Canais',
        'settingsPage.tabs.contacts': 'Contatos',
        'settingsPage.tabs.credentials': 'Cred Manager',
        'settingsPage.tabs.allowlists': 'Allow Lists',
        'settingsPage.tabs.agent-permissions': 'Autorizações do Agente',
        'settingsPage.tabs.appearance': 'Aparência',
        'settingsPage.tabs.data': 'Dados',
        'settingsPage.tabs.restore-defaults': 'Restaurar Padrões',
      } as Record<string, string>)[key] ?? fallback ?? key,
  }),
}));

vi.mock('./ProvidersPage', () => ({ default: () => <button data-testid="providers-default">ProvidersPage</button> }));
vi.mock('./McpPage', () => ({ default: () => <button data-testid="mcp-default">McpPage</button> }));
vi.mock('./SkillsPage', () => ({ default: () => <button data-testid="skills-default">SkillsPage</button> }));
vi.mock('./ChannelsPage', () => ({ default: () => <button data-testid="channels-default">ChannelsPage</button> }));
vi.mock('./ContactsPage', () => ({ default: () => <button data-testid="contacts-default">ContactsPage</button> }));
vi.mock('./CredentialsPage', () => ({ default: () => <button data-testid="credentials-default">CredentialsPage</button> }));
vi.mock('./AllowlistPage', () => ({ default: () => <button data-testid="allowlists-default">AllowlistPage</button> }));
vi.mock('./AgentPermissionsPage', () => ({ default: () => <button data-testid="agent-permissions-default">AgentPermissionsPage</button> }));
vi.mock('./AppearancePage', () => ({ default: () => <button data-testid="appearance-default">AppearancePage</button> }));
vi.mock('./DataManagementPage', () => ({ default: () => <button data-testid="data-default">DataManagementPage</button> }));
vi.mock('./RestoreDefaultsPage', () => ({ default: () => <button data-testid="restore-defaults-default">RestoreDefaultsPage</button> }));

import SettingsPage from './SettingsPage';

describe('SettingsPage', () => {
  beforeEach(() => {
    mockTab = undefined;
    mockNavigate.mockReset();
    mockAnnounce.mockReset();
  });

  it('renderiza todas as tabs de configuração', async () => {
    render(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText('Provedores LLM')).toBeInTheDocument();
    });

    expect(screen.getByText('MCP')).toBeInTheDocument();
    expect(screen.getByText('Skills')).toBeInTheDocument();
    expect(screen.getByText('Canais')).toBeInTheDocument();
    expect(screen.getByText('Contatos')).toBeInTheDocument();
    expect(screen.getByText('Cred Manager')).toBeInTheDocument();
    expect(screen.getByText('Allow Lists')).toBeInTheDocument();
    expect(screen.getByText('Aparência')).toBeInTheDocument();
    expect(screen.getByText('Dados')).toBeInTheDocument();
    expect(screen.getByText('Restaurar Padrões')).toBeInTheDocument();
  });

  it('usa providers como tab padrão quando nenhuma tab é especificada', () => {
    render(<SettingsPage />);

    expect(screen.getByRole('tab', { name: 'Provedores LLM' })).toHaveAttribute('aria-selected', 'true');
  });

  it('seleciona a tab correta quando parametro de URL é fornecido', () => {
    mockTab = 'mcp';
    render(<SettingsPage />);

    expect(screen.getByRole('tab', { name: 'MCP' })).toHaveAttribute('aria-selected', 'true');
  });

  it('volta para tab padrão quando parametro de URL é inválido', () => {
    mockTab = 'invalid-tab';
    render(<SettingsPage />);

    expect(screen.getByRole('tab', { name: 'Provedores LLM' })).toHaveAttribute('aria-selected', 'true');
  });

  it('possui tablist acessível com aria-label', () => {
    render(<SettingsPage />);

    const tablist = screen.getByRole('tablist');
    expect(tablist).toHaveAttribute('aria-label', 'Configurações');
  });

  it('renderiza todas as 11 tabs com role="tab"', () => {
    render(<SettingsPage />);

    const tabs = screen.getAllByRole('tab');
    expect(tabs).toHaveLength(11);
  });

  it('renderiza todos os 11 tabpanels', () => {
    render(<SettingsPage />);

    const panels = screen.getAllByRole('tabpanel', { hidden: true });
    expect(panels).toHaveLength(11);
  });

  it('renderiza o conteúdo do ProvidersPage no panel correspondente', async () => {
    render(<SettingsPage />);

    await waitFor(() => {
      expect(screen.getByText('ProvidersPage')).toBeInTheDocument();
    });
  });

  it('navega ao clicar em uma tab', async () => {
    const user = userEvent.setup();
    render(<SettingsPage />);

    const mcpTab = screen.getByText('MCP');
    await user.click(mcpTab);

    expect(mockNavigate).toHaveBeenCalledWith('/settings/mcp', { replace: true });
  });

  it('possui data-tab-scope no container', () => {
    render(<SettingsPage />);

    const container = screen.getByRole('tablist').closest('.settings-page');
    expect(container).toHaveAttribute('data-tab-scope');
  });

  describe('Ctrl+Tab / Ctrl+PageDown navigation', () => {
    it('Ctrl+Tab navega para a próxima aba', () => {
      mockTab = 'providers';
      render(<SettingsPage />);

      const container = screen.getByRole('tablist').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'Tab', ctrlKey: true });

      expect(mockNavigate).toHaveBeenCalledWith('/settings/mcp', { replace: true });
    });

    it('Ctrl+Shift+Tab navega para a aba anterior', () => {
      mockTab = 'mcp';
      render(<SettingsPage />);

      const container = screen.getByRole('tablist').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'Tab', ctrlKey: true, shiftKey: true });

      expect(mockNavigate).toHaveBeenCalledWith('/settings/providers', { replace: true });
    });

    it('Ctrl+PageDown navega para a próxima aba', () => {
      mockTab = 'skills';
      render(<SettingsPage />);

      const container = screen.getByRole('tablist').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'PageDown', ctrlKey: true });

      expect(mockNavigate).toHaveBeenCalledWith('/settings/channels', { replace: true });
    });

    it('Ctrl+PageUp navega para a aba anterior', () => {
      mockTab = 'skills';
      render(<SettingsPage />);

      const container = screen.getByRole('tablist').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'PageUp', ctrlKey: true });

      expect(mockNavigate).toHaveBeenCalledWith('/settings/mcp', { replace: true });
    });

    it('Ctrl+Tab faz wrap da última para a primeira aba', () => {
      mockTab = 'restore-defaults';
      render(<SettingsPage />);

      const container = screen.getByRole('tablist').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'Tab', ctrlKey: true });

      expect(mockNavigate).toHaveBeenCalledWith('/settings/providers', { replace: true });
    });

    it('Ctrl+Shift+Tab faz wrap da primeira para a última aba', () => {
      mockTab = 'providers';
      render(<SettingsPage />);

      const container = screen.getByRole('tablist').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'Tab', ctrlKey: true, shiftKey: true });

      expect(mockNavigate).toHaveBeenCalledWith('/settings/restore-defaults', { replace: true });
    });

    it('anuncia o nome da aba ao navegar', () => {
      mockTab = 'providers';
      render(<SettingsPage />);

      const container = screen.getByRole('tablist').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'Tab', ctrlKey: true });

      expect(mockAnnounce).toHaveBeenCalledWith('MCP');
    });

    it.each([
      ['Ctrl+Tab', { key: 'Tab', ctrlKey: true }, 'mcp', 'mcp-default'],
      ['Ctrl+Shift+Tab', { key: 'Tab', ctrlKey: true, shiftKey: true }, 'restore-defaults', 'restore-defaults-default'],
      ['Ctrl+PageDown', { key: 'PageDown', ctrlKey: true }, 'mcp', 'mcp-default'],
      ['Ctrl+PageUp', { key: 'PageUp', ctrlKey: true }, 'restore-defaults', 'restore-defaults-default'],
    ])('%s restaura foco para o conteúdo', async (_label, init, nextTab, defaultTarget) => {
      mockTab = 'providers';
      const { rerender } = render(<SettingsPage />);

      const tab = screen.getByRole('tab', { name: 'Provedores LLM' });
      tab.focus();
      fireEvent.keyDown(tab, init);
      mockTab = nextTab;
      rerender(<SettingsPage />);

      await waitFor(() => {
        expect(screen.getByTestId(defaultTarget)).toHaveFocus();
      });
    });

    it('setas mantêm foco na lista de abas', () => {
      render(<SettingsPage />);

      const providersTab = screen.getByRole('tab', { name: 'Provedores LLM' });
      providersTab.focus();
      fireEvent.keyDown(providersTab, { key: 'ArrowRight' });

      expect(screen.getByRole('tab', { name: 'MCP' })).toHaveFocus();
      expect(screen.getByTestId('providers-default')).not.toHaveFocus();
    });

    it('Enter na guia leva foco para o conteúdo', async () => {
      render(<SettingsPage />);

      const providersTab = screen.getByRole('tab', { name: 'Provedores LLM' });
      providersTab.focus();
      fireEvent.keyDown(providersTab, { key: 'Enter' });

      await waitFor(() => {
        expect(screen.getByTestId('providers-default')).toHaveFocus();
      });
    });

  });
});
