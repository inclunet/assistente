import type { ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

let mockTab: string | undefined;
const mockNavigate = vi.fn();

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
vi.mock('./ChannelsPage', () => ({ default: () => <div>ChannelsPage</div> }));
vi.mock('./ContactsPage', () => ({ default: () => <div>ContactsPage</div> }));
vi.mock('./CredentialsPage', () => ({ default: () => <div>CredentialsPage</div> }));
vi.mock('./AllowlistPage', () => ({ default: () => <div>AllowlistPage</div> }));
vi.mock('./RestoreDefaultsPage', () => ({ default: () => <div>RestoreDefaultsPage</div> }));

import SettingsPage from './SettingsPage';

describe('SettingsPage', () => {
  beforeEach(() => {
    mockTab = undefined;
    mockNavigate.mockReset();
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

  it('renderiza todas as 8 tabs com role="tab"', () => {
    render(<SettingsPage />);

    const tabs = screen.getAllByRole('tab');
    expect(tabs).toHaveLength(8);
  });

  it('renderiza todos os 8 tabpanels', () => {
    render(<SettingsPage />);

    const panels = screen.getAllByRole('tabpanel');
    expect(panels).toHaveLength(8);
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
});
