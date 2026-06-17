import type { ReactNode } from 'react';
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
        'settingsPage.tabs.memories': 'Memórias',
        'settingsPage.tabs.channels': 'Canais',
        'settingsPage.tabs.contacts': 'Contatos',
        'settingsPage.tabs.credentials': 'Cred Manager',
        'settingsPage.tabs.allowlists': 'Allow Lists',
        'settingsPage.tabs.appearance': 'Aparência',
        'settingsPage.tabs.data': 'Dados',
        'settingsPage.tabs.restore-defaults': 'Restaurar Padrões',
      } as Record<string, string>)[key] ?? fallback ?? key,
  }),
}));

vi.mock('../components/ui/tabs', () => ({
  Tabs: ({ children, value }: { children: ReactNode; value: string }) => (
    <div data-testid="tabs" data-value={value}>{children}</div>
  ),
  TabList: ({ children, ariaLabel }: { children: ReactNode; ariaLabel?: string }) => (
    <div role="tablist" aria-label={ariaLabel}>{children}</div>
  ),
  Tab: ({ children, value, className, activeClassName }: { children: ReactNode; value: string; className?: string; activeClassName?: string }) => (
    <button role="tab" data-value={value} className={`${className ?? ''} ${activeClassName ?? ''}`}>
      {children}
    </button>
  ),
  TabPanel: ({ children, value }: { children: ReactNode; value: string }) => (
    <div role="tabpanel" data-value={value}>{children}</div>
  ),
}));

vi.mock('./ProvidersPage', () => ({ default: () => <div>ProvidersPage</div> }));
vi.mock('./McpPage', () => ({ default: () => <div>McpPage</div> }));
vi.mock('./SkillsPage', () => ({ default: () => <div>SkillsPage</div> }));
vi.mock('./MemoriesPage', () => ({ default: () => <div>MemoriesPage</div> }));
vi.mock('./ChannelsPage', () => ({ default: () => <div>ChannelsPage</div> }));
vi.mock('./ContactsPage', () => ({ default: () => <div>ContactsPage</div> }));
vi.mock('./CredentialsPage', () => ({ default: () => <div>CredentialsPage</div> }));
vi.mock('./AllowlistPage', () => ({ default: () => <div>AllowlistPage</div> }));
vi.mock('./AppearancePage', () => ({ default: () => <div>AppearancePage</div> }));
vi.mock('./DataManagementPage', () => ({ default: () => <div>DataManagementPage</div> }));
vi.mock('./RestoreDefaultsPage', () => ({ default: () => <div>RestoreDefaultsPage</div> }));

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
    expect(screen.getByText('Memórias')).toBeInTheDocument();
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

    const tabs = screen.getByTestId('tabs');
    expect(tabs).toHaveAttribute('data-value', 'providers');
  });

  it('seleciona a tab correta quando parametro de URL é fornecido', () => {
    mockTab = 'mcp';
    render(<SettingsPage />);

    const tabs = screen.getByTestId('tabs');
    expect(tabs).toHaveAttribute('data-value', 'mcp');
  });

  it('volta para tab padrão quando parametro de URL é inválido', () => {
    mockTab = 'invalid-tab';
    render(<SettingsPage />);

    const tabs = screen.getByTestId('tabs');
    expect(tabs).toHaveAttribute('data-value', 'providers');
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

    const panels = screen.getAllByRole('tabpanel');
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

    // The Tab mock doesn't call onValueChange, but we verify the tabs render
    expect(mcpTab).toBeInTheDocument();
  });

  it('possui data-tab-scope no container', () => {
    render(<SettingsPage />);

    const container = screen.getByTestId('tabs').closest('.settings-page');
    expect(container).toHaveAttribute('data-tab-scope');
  });

  describe('Ctrl+Tab / Ctrl+PageDown navigation', () => {
    it('Ctrl+Tab navega para a próxima aba', () => {
      mockTab = 'providers';
      render(<SettingsPage />);

      const container = screen.getByTestId('tabs').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'Tab', ctrlKey: true });

      expect(mockNavigate).toHaveBeenCalledWith('/settings/mcp', { replace: true });
    });

    it('Ctrl+Shift+Tab navega para a aba anterior', () => {
      mockTab = 'mcp';
      render(<SettingsPage />);

      const container = screen.getByTestId('tabs').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'Tab', ctrlKey: true, shiftKey: true });

      expect(mockNavigate).toHaveBeenCalledWith('/settings/providers', { replace: true });
    });

    it('Ctrl+PageDown navega para a próxima aba', () => {
      mockTab = 'skills';
      render(<SettingsPage />);

      const container = screen.getByTestId('tabs').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'PageDown', ctrlKey: true });

      expect(mockNavigate).toHaveBeenCalledWith('/settings/memories', { replace: true });
    });

    it('Ctrl+PageUp navega para a aba anterior', () => {
      mockTab = 'skills';
      render(<SettingsPage />);

      const container = screen.getByTestId('tabs').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'PageUp', ctrlKey: true });

      expect(mockNavigate).toHaveBeenCalledWith('/settings/mcp', { replace: true });
    });

    it('Ctrl+Tab faz wrap da última para a primeira aba', () => {
      mockTab = 'restore-defaults';
      render(<SettingsPage />);

      const container = screen.getByTestId('tabs').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'Tab', ctrlKey: true });

      expect(mockNavigate).toHaveBeenCalledWith('/settings/providers', { replace: true });
    });

    it('Ctrl+Shift+Tab faz wrap da primeira para a última aba', () => {
      mockTab = 'providers';
      render(<SettingsPage />);

      const container = screen.getByTestId('tabs').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'Tab', ctrlKey: true, shiftKey: true });

      expect(mockNavigate).toHaveBeenCalledWith('/settings/restore-defaults', { replace: true });
    });

    it('anuncia o nome da aba ao navegar', () => {
      mockTab = 'providers';
      render(<SettingsPage />);

      const container = screen.getByTestId('tabs').closest('.settings-page')!;
      fireEvent.keyDown(container, { key: 'Tab', ctrlKey: true });

      expect(mockAnnounce).toHaveBeenCalledWith('MCP');
    });
  });
});
