import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetSubAgentConversations = vi.fn();
const mockNavigate = vi.fn();
const mockExecuteDeepLink = vi.fn().mockResolvedValue(undefined);
const mockAnnounce = vi.fn();

const stableT = (key: string, fallback?: string) => {
  if (typeof fallback === 'string') return fallback;
  return key;
};

type SubAgentItem = {
  conversationId: string;
  title: string;
  latestStatus: string;
  runCount: number;
  background: boolean;
  messageCount: number;
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  createdAt: string;
  updatedAt: string;
};

vi.mock('react-router-dom', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-router-dom')>();
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({ t: stableT }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetSubAgentConversations: () => mockGetSubAgentConversations(),
}));

vi.mock('@wailsjs/go/models', () => ({ subagent: {} }));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({ handleGridReady: vi.fn() }),
}));

vi.mock('../hooks/useGridPageLandmarks', () => ({
  useGridPageLandmarks: vi.fn(),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: mockAnnounce }),
}));

vi.mock('../lib/deepLinks', () => ({
  executeDeepLink: (...args: unknown[]) => mockExecuteDeepLink(...args),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, actions }: { left?: ReactNode; actions?: Array<{ key: string; label: string; onClick: () => void; disabled?: boolean }> }) => {
    return (
      <div>
        {left}
        {actions?.map((action) => (
          <button key={action.key} onClick={action.onClick} disabled={action.disabled}>
            {action.label}
          </button>
        ))}
      </div>
    );
  },
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({
    items,
    onFocusChange,
    onActivate,
  }: {
    items?: SubAgentItem[];
    onFocusChange?: (item: SubAgentItem | null) => void;
    onActivate?: (item: SubAgentItem) => void;
  }) => (
    <div>
      <button type="button" onClick={() => onFocusChange?.(items?.[0] ?? null)}>focus-first</button>
      {items?.map((item) => (
        <div key={item.conversationId}>
          <button type="button" onClick={() => onActivate?.(item)}>{item.title}</button>
        </div>
      ))}
    </div>
  ),
}));

const subAgents: SubAgentItem[] = [
  {
    conversationId: '01926b90-7a5a-7c4e-8d3f-0000000000a1',
    title: 'Pesquisa de mercado',
    latestStatus: 'succeeded',
    runCount: 2,
    background: true,
    messageCount: 6,
    promptTokens: 100,
    completionTokens: 50,
    totalTokens: 150,
    createdAt: '2025-01-01T00:00:00Z',
    updatedAt: '2025-01-02T00:00:00Z',
  },
  {
    conversationId: '01926b90-7a5a-7c4e-8d3f-0000000000a2',
    title: 'Resumo diário',
    latestStatus: 'running',
    runCount: 1,
    background: false,
    messageCount: 2,
    promptTokens: 10,
    completionTokens: 5,
    totalTokens: 15,
    createdAt: '2025-01-03T00:00:00Z',
    updatedAt: '2025-01-03T00:00:00Z',
  },
];

import SubAgentsPage from './SubAgentsPage';

describe('SubAgentsPage', { timeout: 60_000 }, () => {
  beforeEach(() => {
    mockGetSubAgentConversations.mockReset();
    mockNavigate.mockReset();
    mockExecuteDeepLink.mockClear();
    mockAnnounce.mockReset();
  });

  it('lista as sub-conversas retornadas pelo backend', async () => {
    mockGetSubAgentConversations.mockResolvedValue(subAgents);
    render(<SubAgentsPage />);

    await screen.findByText('Pesquisa de mercado');
    expect(screen.getByText('Resumo diário')).toBeInTheDocument();
  });

  it('abre a sub-conversa via pipeline oficial de deep link', async () => {
    const user = userEvent.setup();
    mockGetSubAgentConversations.mockResolvedValue(subAgents);
    render(<SubAgentsPage />);

    await user.click(await screen.findByText('Pesquisa de mercado'));

    await waitFor(() => {
      expect(mockExecuteDeepLink).toHaveBeenCalledWith(
        { type: 'conversation:open', conversationId: '01926b90-7a5a-7c4e-8d3f-0000000000a1', title: 'Pesquisa de mercado' },
        expect.objectContaining({ navigate: expect.any(Function) }),
      );
    });
  });

  it('exibe estado vazio quando não há sub-agentes', async () => {
    mockGetSubAgentConversations.mockResolvedValue([]);
    render(<SubAgentsPage />);

    expect(await screen.findByText('subagents.empty')).toBeInTheDocument();
  });

  it('anuncia erro quando a carga falha', async () => {
    mockGetSubAgentConversations.mockRejectedValue(new Error('boom'));
    render(<SubAgentsPage />);

    await waitFor(() => {
      expect(mockAnnounce).toHaveBeenCalledWith('subagents.errorLoading', 'assertive');
    });
  });

  it('renderiza estado de erro visível (distinto do vazio) quando a carga falha', async () => {
    mockGetSubAgentConversations.mockRejectedValue(new Error('boom'));
    render(<SubAgentsPage />);

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('subagents.errorLoading');
    // Estado de erro é distinto do estado vazio.
    expect(screen.queryByText('subagents.empty')).not.toBeInTheDocument();
  });

  it('permite tentar novamente após falha de carga', async () => {
    const user = userEvent.setup();
    mockGetSubAgentConversations.mockRejectedValueOnce(new Error('boom'));
    mockGetSubAgentConversations.mockResolvedValueOnce(subAgents);
    render(<SubAgentsPage />);

    await user.click(await screen.findByText('subagents.retry'));

    await screen.findByText('Pesquisa de mercado');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('não quebra e anuncia erro quando o deep link falha ao abrir', async () => {
    const user = userEvent.setup();
    mockGetSubAgentConversations.mockResolvedValue(subAgents);
    mockExecuteDeepLink.mockRejectedValueOnce(new Error('deep link boom'));
    render(<SubAgentsPage />);

    await user.click(await screen.findByText('Pesquisa de mercado'));

    await waitFor(() => {
      expect(mockAnnounce).toHaveBeenCalledWith('subagents.openError', 'assertive');
    });
  });
});
