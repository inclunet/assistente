import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { TokenStatsButton } from './TokenStatsButton';

const getStatsSpy = vi.fn();
let mockEventsOnCallback: ((data: Record<string, unknown>) => void) | null = null;

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetConversationTokenStats: (id: string) => getStatsSpy(id),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, callback: (data: Record<string, unknown>) => void) => {
    if (event === 'chat:token_stats_update') {
      mockEventsOnCallback = callback;
    }
    return () => {}; // unsubscribe function
  },
}));

beforeEach(() => {
  mockEventsOnCallback = null;
  getStatsSpy.mockClear();
});

describe('TokenStatsButton', () => {
  it('renderiza stats e abre modal', async () => {
    getStatsSpy.mockResolvedValueOnce({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 10,
      completionTokens: 20,
      totalTokens: 30,
      contextTokens: 30,
      messageCount: 1,
      mostUsedModel: 'x',
      contextUsage: 10,
      contextLimit: 100,
      isNearLimit: false,
      isCritical: false,
    });

    const onOpenModal = vi.fn();
    render(<TokenStatsButton conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"} onOpenModal={onOpenModal} />);

    const button = await waitFor(() => {
      const el = screen.getByRole('button');
      expect(el).not.toBeDisabled();
      return el;
    });

    fireEvent.click(button);
    expect(onOpenModal).toHaveBeenCalled();
  });

  it('nao renderiza sem conversationId', () => {
    const { container } = render(
      <TokenStatsButton onOpenModal={() => {}} />
    );

    expect(container.firstChild).toBeNull();
  });

  it('renderiza badge de contexto com porcentagem', async () => {
    getStatsSpy.mockResolvedValueOnce({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 10,
      completionTokens: 20,
      totalTokens: 30,
      contextTokens: 30,
      messageCount: 1,
      mostUsedModel: 'claude-3',
      contextUsage: 45.5,
      contextLimit: 200000,
      isNearLimit: false,
      isCritical: false,
    });

    const onOpenModal = vi.fn();
    render(<TokenStatsButton conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"} onOpenModal={onOpenModal} />);

    await waitFor(() => {
      expect(screen.getByText('45.5%')).toBeInTheDocument();
    });
  });

  it('atualiza badge em tempo real via evento', async () => {
    getStatsSpy.mockResolvedValueOnce({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 10,
      completionTokens: 20,
      totalTokens: 30,
      contextTokens: 30,
      messageCount: 1,
      mostUsedModel: 'claude-3',
      contextUsage: 30,
      contextLimit: 200000,
      isNearLimit: false,
      isCritical: false,
    });

    const onOpenModal = vi.fn();
    render(<TokenStatsButton conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"} onOpenModal={onOpenModal} />);

    await waitFor(() => {
      expect(screen.getByText('30.0%')).toBeInTheDocument();
    });

    // Simula evento de atualização de tokens
    if (mockEventsOnCallback) {
      mockEventsOnCallback({
        conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
        contextUsage: 75.3,
        contextLimit: 200000,
        isNearLimit: true,
        isCritical: false,
        totalTokens: 150600,
        contextTokens: 150600,
        promptTokens: 100400,
        completionTokens: 50200,
        messageCount: 5,
        mostUsedModel: 'claude-3',
      });
    }

    await waitFor(() => {
      expect(screen.getByText('75.3%')).toBeInTheDocument();
    });
  });

  it('aplica classe de aviso quando contextUsage >= 80%', async () => {
    getStatsSpy.mockResolvedValueOnce({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 10,
      completionTokens: 20,
      totalTokens: 30,
      contextTokens: 30,
      messageCount: 1,
      mostUsedModel: 'claude-3',
      contextUsage: 85,
      contextLimit: 200000,
      isNearLimit: true,
      isCritical: false,
    });

    const onOpenModal = vi.fn();
    const { container } = render(
      <TokenStatsButton conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"} onOpenModal={onOpenModal} />
    );

    await waitFor(() => {
      expect(screen.getByText('85.0%')).toBeInTheDocument();
    });

    const button = container.querySelector('button');
    expect(button).toHaveClass('token-stats-button--warning');
  });

  it('aplica classe crítica quando contextUsage >= 95%', async () => {
    getStatsSpy.mockResolvedValueOnce({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 10,
      completionTokens: 20,
      totalTokens: 30,
      contextTokens: 30,
      messageCount: 1,
      mostUsedModel: 'claude-3',
      contextUsage: 96.5,
      contextLimit: 200000,
      isNearLimit: true,
      isCritical: true,
    });

    const onOpenModal = vi.fn();
    const { container } = render(
      <TokenStatsButton conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"} onOpenModal={onOpenModal} />
    );

    await waitFor(() => {
      expect(screen.getByText('96.5%')).toBeInTheDocument();
    });

    const button = container.querySelector('button');
    expect(button).toHaveClass('token-stats-button--critical');
  });

  it('nao renderiza badge quando contextLimit é 0', async () => {
    getStatsSpy.mockResolvedValueOnce({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 10,
      completionTokens: 20,
      totalTokens: 30,
      contextTokens: 30,
      messageCount: 1,
      mostUsedModel: 'x',
      contextUsage: 0,
      contextLimit: 0,
      isNearLimit: false,
      isCritical: false,
    });

    const onOpenModal = vi.fn();
    render(<TokenStatsButton conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"} onOpenModal={onOpenModal} />);

    await waitFor(() => {
      expect(screen.getByRole('button')).toBeInTheDocument();
    });

    // Badge não deve estar presente
    expect(screen.queryByText(/\d+\.\d+%/)).not.toBeInTheDocument();
  });

  it('ignora eventos de outras conversas', async () => {
    getStatsSpy.mockResolvedValueOnce({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 10,
      completionTokens: 20,
      totalTokens: 30,
      contextTokens: 30,
      messageCount: 1,
      mostUsedModel: 'claude-3',
      contextUsage: 30,
      contextLimit: 200000,
      isNearLimit: false,
      isCritical: false,
    });

    const onOpenModal = vi.fn();
    render(<TokenStatsButton conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"} onOpenModal={onOpenModal} />);

    await waitFor(() => {
      expect(screen.getByText('30.0%')).toBeInTheDocument();
    });

    // Simula evento de conversa diferente (conversationId 999)
    if (mockEventsOnCallback) {
      mockEventsOnCallback({
        conversationId: "01926b90-7a5a-7c4e-8d3f-000000000999",
        contextUsage: 99,
        contextLimit: 200000,
        isNearLimit: true,
        isCritical: true,
        totalTokens: 198000,
        contextTokens: 198000,
        promptTokens: 100000,
        completionTokens: 98000,
        messageCount: 10,
        mostUsedModel: 'claude-3',
      });
    }

    // Deve manter o valor original (30%), não atualizar para 99%
    await waitFor(() => {
      expect(screen.getByText('30.0%')).toBeInTheDocument();
    });
  });
});
