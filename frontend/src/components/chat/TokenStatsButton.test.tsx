import { describe, expect, it, vi, beforeEach } from 'vitest';
import { act, render, screen, fireEvent, waitFor } from '@testing-library/react';
import { TokenStatsButton } from './TokenStatsButton';

const getStatsSpy = vi.fn();
let mockEventsOnCallback: ((data: Record<string, unknown>) => void) | null = null;
const listeners = new Map<string, (data: Record<string, unknown>) => void>();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@wailsjs/go/wailsapi/Tokens', () => ({
  GetConversationTokenStats: (id: string) => getStatsSpy(id),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, callback: (data: Record<string, unknown>) => void) => {
    listeners.set(event, callback);
    if (event === 'chat:token_stats_update') {
      mockEventsOnCallback = callback;
    }
    return () => { if (listeners.get(event) === callback) listeners.delete(event); };
  },
}));

beforeEach(() => {
  mockEventsOnCallback = null;
  listeners.clear();
  getStatsSpy.mockReset();
});

function statsFor(conversationId: string, contextUsage: number) {
  return {
    conversationId, contextUsage, contextLimit: 100, contextTokens: contextUsage,
    promptTokens: contextUsage, completionTokens: 0, totalTokens: contextUsage,
    messageCount: 1, mostUsedModel: 'model', isNearLimit: false, isCritical: false,
  };
}

function pendingStats() {
  let resolve!: (value: ReturnType<typeof statsFor>) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<ReturnType<typeof statsFor>>((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

describe('TokenStatsButton', () => {
  it.each(['resolve', 'reject'] as const)('ignora %s de A depois de B carregada', async (result) => {
    const old = pendingStats();
    getStatsSpy.mockReturnValueOnce(old.promise).mockResolvedValueOnce(statsFor('B', 42));
    const view = render(<TokenStatsButton conversationId="A" onOpenModal={vi.fn()} />);
    view.rerender(<TokenStatsButton conversationId="B" onOpenModal={vi.fn()} />);
    await screen.findByText('42.0%');
    await act(async () => {
      if (result === 'resolve') old.resolve(statsFor('A', 11));
      else old.reject(new Error('old failure'));
    });
    expect(screen.getByText('42.0%')).toBeInTheDocument();
    expect(screen.getByRole('button')).not.toBeDisabled();
  });

  it('limpa estatísticas e bloqueia abertura enquanto a nova conversa carrega', async () => {
    const next = pendingStats();
    getStatsSpy.mockResolvedValueOnce(statsFor('A', 11)).mockReturnValueOnce(next.promise);
    const open = vi.fn();
    const view = render(<TokenStatsButton conversationId="A" onOpenModal={open} />);
    await screen.findByText('11.0%');
    view.rerender(<TokenStatsButton conversationId="B" onOpenModal={open} />);
    expect(screen.queryByText('11.0%')).not.toBeInTheDocument();
    expect(screen.getByRole('button')).toBeDisabled();
    fireEvent.click(screen.getByRole('button'));
    expect(open).not.toHaveBeenCalled();
    await act(async () => next.resolve(statsFor('B', 42)));
    expect(screen.getByText('42.0%')).toBeInTheDocument();
  });

  it.each(['chat:token_stats', 'chat:token_stats_update'])('evento %s invalida snapshot pendente e preserva realtime', async (event) => {
    const old = pendingStats();
    getStatsSpy.mockReturnValueOnce(old.promise);
    render(<TokenStatsButton conversationId="A" onOpenModal={vi.fn()} />);
    act(() => listeners.get(event)?.(statsFor('A', 63)));
    expect(screen.getByText('63.0%')).toBeInTheDocument();
    expect(screen.getByRole('button')).not.toBeDisabled();
    await act(async () => old.resolve(statsFor('A', 11)));
    expect(screen.getByText('63.0%')).toBeInTheDocument();
  });

  it.each(['resolve', 'reject'] as const)('reload mais recente vence %s antigo e finally não encerra loading atual', async (result) => {
    const first = pendingStats();
    const second = pendingStats();
    const third = pendingStats();
    getStatsSpy.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise).mockReturnValueOnce(third.promise);
    render(<TokenStatsButton conversationId="A" onOpenModal={vi.fn()} />);
    act(() => listeners.get('chat:messages_ready')?.({ conversationId: 'A' }));
    await act(async () => first.resolve(statsFor('A', 11)));
    expect(screen.getByRole('button')).toBeDisabled();
    act(() => listeners.get('chat:done')?.({ conversationId: 'A' }));
    await act(async () => third.resolve(statsFor('A', 73)));
    await act(async () => {
      if (result === 'resolve') second.resolve(statsFor('A', 22));
      else second.reject(new Error('stale reload'));
    });
    expect(screen.getByText('73.0%')).toBeInTheDocument();
    expect(screen.getByRole('button')).not.toBeDisabled();
  });

  it.each(['close', 'unmount'])('invalida callbacks e respostas após %s', async (mode) => {
    const old = pendingStats();
    getStatsSpy.mockReturnValueOnce(old.promise).mockResolvedValueOnce(statsFor('A', 42));
    const view = render(<TokenStatsButton conversationId="A" onOpenModal={vi.fn()} />);
    const queued = [...listeners.values()];
    if (mode === 'close') view.rerender(<TokenStatsButton onOpenModal={vi.fn()} />);
    else view.unmount();
    expect(listeners.size).toBe(0);
    await act(async () => {
      queued.forEach((callback) => callback(statsFor('A', 99)));
      old.resolve(statsFor('A', 11));
    });
    expect(getStatsSpy).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
    if (mode === 'close') view.rerender(<TokenStatsButton conversationId="A" onOpenModal={vi.fn()} />);
    else render(<TokenStatsButton conversationId="A" onOpenModal={vi.fn()} />);
    await screen.findByText('42.0%');
  });

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
    act(() => {
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
    });

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
