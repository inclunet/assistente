import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { TokenStatsModal } from './TokenStatsModal';

const getStatsSpy = vi.fn();
const tMock = (key: string) => (key === 'tokenStats.placeholder' ? '—' : key);
const eventCallbacks: Record<string, Array<(data: Record<string, unknown>) => void>> = {};

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: tMock, i18n: { language: 'pt-BR' } }),
}));

vi.mock('@wailsjs/go/wailsapi/Tokens', () => ({
  GetConversationTokenStats: (id: string) => getStatsSpy(id),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, callback: (data: Record<string, unknown>) => void) => {
    eventCallbacks[event] = [...(eventCallbacks[event] ?? []), callback];
    return () => {
      eventCallbacks[event] = (eventCallbacks[event] ?? []).filter((registered) => registered !== callback);
    };
  },
}));

beforeEach(() => {
  cleanup();
  document.body.innerHTML = '';
  document.body.removeAttribute('style');
  getStatsSpy.mockReset();
  Object.keys(eventCallbacks).forEach((event) => {
    eventCallbacks[event] = [];
  });
});

describe('TokenStatsModal', () => {
  it('renderiza stats quando aberto', async () => {
    getStatsSpy.mockResolvedValue({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 185170,
      completionTokens: 2912,
      totalTokens: 188082,
      contextTokens: 63942,
      messageCount: 6,
      modelCallCount: 3,
      mostUsedModel: 'x',
      contextUsage: 31.971,
      contextLimit: 200000,
      isNearLimit: false,
      isCritical: false,
      systemPromptEstimatedTokens: 5,
      summaryTokens: 3,
      messagesInContextTokens: 15,
      messagesOutOfContextTokens: 7,
      messagesInContextCount: 1,
      messagesOutOfContextCount: 0,
      toolsUsedCount: 0,
      toolBreakdown: [],
      agenticLoopCount: 0,
      agenticLoopTotalTokens: 0,
      agenticLoopBreakdown: [],
    });

    render(
      <TokenStatsModal
        conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"}
        isOpen={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('tokenStats.contextUsage')).toBeInTheDocument();
    });
    const progress = screen.getByRole('progressbar', { name: 'tokenStats.contextUsage' });
    expect(progress).toHaveAttribute('aria-valuenow', '32');
    expect(progress).toHaveAttribute('aria-valuetext', '32.0%');
    expect(screen.getByText('tokenStats.modelCalls')).toBeInTheDocument();
    expect(screen.queryByText('62.694 tokenStats.tokensPerCall')).not.toBeInTheDocument();
    expect(screen.queryByText('31.347 tokenStats.tokensPerMsg')).not.toBeInTheDocument();
  });

  it('renderiza métricas e aviso de prompt cache', async () => {
    getStatsSpy.mockResolvedValue({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 1000,
      completionTokens: 200,
      totalTokens: 1200,
      cacheReadTokens: 300,
      cacheWriteTokens: 100,
      cacheMissTokens: 600,
      cacheHitRate: 30,
      cacheTokensReported: true,
      promptCacheEnabled: true,
      contextTokens: 900,
      messageCount: 2,
      mostUsedModel: 'claude',
      contextUsage: 10,
      contextLimit: 10000,
      isNearLimit: false,
      isCritical: false,
      systemPromptEstimatedTokens: 5,
      summaryTokens: 3,
      messagesInContextTokens: 15,
      messagesOutOfContextTokens: 7,
      messagesInContextCount: 1,
      messagesOutOfContextCount: 0,
      toolsUsedCount: 0,
      toolBreakdown: [],
    });

    render(
      <TokenStatsModal
        conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"}
        isOpen={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('tokenStats.contextUsage')).toBeInTheDocument();
    });
    const cacheTab = screen.getByRole('tab', { name: 'tokenStats.tabPromptCache' });
    fireEvent.click(cacheTab);

    expect(screen.getByText('tokenStats.cacheReadTokens')).toBeInTheDocument();
    expect(screen.getByText('300')).toBeInTheDocument();
    expect(screen.getByText('30.0%')).toBeInTheDocument();
    expect(screen.getByText('tokenStats.cacheReportedNote')).toBeInTheDocument();
    expect(screen.getByText('tokenStats.costDisclaimerWithCache')).toBeInTheDocument();
  });

  it('não mostra abatimento de custo quando cache reportado não tem leitura', async () => {
    getStatsSpy.mockResolvedValue({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 1000,
      completionTokens: 200,
      totalTokens: 1200,
      cacheReadTokens: 0,
      cacheWriteTokens: 100,
      cacheMissTokens: 600,
      cacheHitRate: 0,
      cacheTokensReported: true,
      promptCacheEnabled: true,
      contextTokens: 900,
      messageCount: 2,
      mostUsedModel: 'claude',
      contextUsage: 10,
      contextLimit: 10000,
      isNearLimit: false,
      isCritical: false,
      systemPromptEstimatedTokens: 5,
      summaryTokens: 3,
      messagesInContextTokens: 15,
      messagesOutOfContextTokens: 7,
      messagesInContextCount: 1,
      messagesOutOfContextCount: 0,
      toolsUsedCount: 0,
      toolBreakdown: [],
    });

    render(
      <TokenStatsModal
        conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"}
        isOpen={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('tokenStats.contextUsage')).toBeInTheDocument();
    });

    expect(screen.getByText('tokenStats.costDisclaimer')).toBeInTheDocument();
    expect(screen.queryByText('tokenStats.costDisclaimerWithCache')).not.toBeInTheDocument();
  });

  it('atualiza stats ao receber evento em tempo real', async () => {
    getStatsSpy.mockResolvedValue({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 1000,
      completionTokens: 200,
      totalTokens: 1200,
      cacheReadTokens: 0,
      cacheWriteTokens: 0,
      cacheMissTokens: 0,
      cacheHitRate: 0,
      cacheTokensReported: false,
      promptCacheEnabled: true,
      contextTokens: 900,
      messageCount: 2,
      mostUsedModel: 'claude',
      contextUsage: 10,
      contextLimit: 10000,
      isNearLimit: false,
      isCritical: false,
      systemPromptEstimatedTokens: 5,
      summaryTokens: 3,
      messagesInContextTokens: 15,
      messagesOutOfContextTokens: 7,
      messagesInContextCount: 1,
      messagesOutOfContextCount: 0,
      toolsUsedCount: 0,
      toolBreakdown: [],
    });

    render(
      <TokenStatsModal
        conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"}
        isOpen={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('tokenStats.contextUsage')).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('tab', { name: 'tokenStats.tabPromptCache' }));
    expect(screen.getByText('tokenStats.cacheEnabledNotReportedNote')).toBeInTheDocument();

    act(() => {
      eventCallbacks['chat:token_stats_update']?.forEach((callback) => callback({
        conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
        cacheReadTokens: 300,
        cacheWriteTokens: 100,
        cacheMissTokens: 600,
        cacheHitRate: 30,
        cacheTokensReported: true,
        promptCacheEnabled: true,
      }));
    });

    await waitFor(() => {
      expect(screen.getByText('tokenStats.cacheReportedNote')).toBeInTheDocument();
    });
    expect(screen.queryByText('tokenStats.cacheEnabledNotReportedNote')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('tab', { name: 'tokenStats.tabOverview' }));
    expect(screen.getByText('claude')).toBeInTheDocument();
  });

  it('limpa contadores de cache omitidos em evento em tempo real', async () => {
    getStatsSpy.mockResolvedValue({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 1000,
      completionTokens: 200,
      totalTokens: 1200,
      cacheReadTokens: 300,
      cacheWriteTokens: 100,
      cacheMissTokens: 600,
      cacheHitRate: 30,
      cacheTokensReported: true,
      promptCacheEnabled: true,
      contextTokens: 900,
      messageCount: 2,
      mostUsedModel: 'claude',
      contextUsage: 10,
      contextLimit: 10000,
      isNearLimit: false,
      isCritical: false,
      systemPromptEstimatedTokens: 5,
      summaryTokens: 3,
      messagesInContextTokens: 15,
      messagesOutOfContextTokens: 7,
      messagesInContextCount: 1,
      messagesOutOfContextCount: 0,
      toolsUsedCount: 0,
      toolBreakdown: [],
    });

    render(
      <TokenStatsModal
        conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"}
        isOpen={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('tokenStats.costDisclaimerWithCache')).toBeInTheDocument();
    });

    act(() => {
      eventCallbacks['chat:token_stats_update']?.forEach((callback) => callback({
        conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
        cacheTokensReported: false,
        promptCacheEnabled: true,
      }));
    });

    await waitFor(() => {
      expect(screen.getByText('tokenStats.costDisclaimer')).toBeInTheDocument();
    });
    expect(screen.queryByText('tokenStats.costDisclaimerWithCache')).not.toBeInTheDocument();
  });

  it('preserva campos detalhados ao receber evento final de tokens', async () => {
    getStatsSpy.mockResolvedValue({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 1000,
      completionTokens: 200,
      totalTokens: 1200,
      contextTokens: 900,
      messageCount: 2,
      mostUsedModel: 'claude',
      contextUsage: 10,
      contextLimit: 10000,
      isNearLimit: false,
      isCritical: false,
      systemPromptEstimatedTokens: 5,
      summaryTokens: 3,
      messagesInContextTokens: 15,
      messagesOutOfContextTokens: 7,
      messagesInContextCount: 1,
      messagesOutOfContextCount: 0,
      toolsUsedCount: 1,
      toolBreakdown: [{
        toolName: 'search',
        callCount: 1,
        totalPromptTokens: 10,
        totalCompletionTokens: 5,
        totalTokens: 15,
      }],
    });

    render(
      <TokenStatsModal
        conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"}
        isOpen={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('claude')).toBeInTheDocument();
    });

    act(() => {
      eventCallbacks['chat:token_stats']?.forEach((callback) => callback({
        conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
        promptTokens: 1300,
        completionTokens: 300,
        totalTokens: 1600,
        contextTokens: 1200,
        contextUsage: 12,
        contextLimit: 10000,
        isNearLimit: false,
        isCritical: false,
        messageCount: 3,
        modelCallCount: 2,
        cacheTokensReported: false,
      }));
    });

    await waitFor(() => {
      expect(screen.getAllByText('1.600').length).toBeGreaterThan(0);
    });
    expect(screen.getByText('claude')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', { name: 'tokenStats.tabToolCalling' }));
    expect(screen.getByText('search')).toBeInTheDocument();
  });

  it('aplica evento final recebido durante carregamento inicial', async () => {
    let resolveStats!: (value: Record<string, unknown>) => void;
    getStatsSpy.mockReturnValue(new Promise((resolve) => {
      resolveStats = resolve;
    }));

    render(
      <TokenStatsModal
        conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"}
        isOpen={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(eventCallbacks['chat:token_stats']?.length).toBeGreaterThan(0);
    });

    act(() => {
      eventCallbacks['chat:token_stats']?.forEach((callback) => callback({
        conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
        promptTokens: 1300,
        completionTokens: 300,
        totalTokens: 1600,
        contextTokens: 1200,
        contextUsage: 12,
        contextLimit: 10000,
        isNearLimit: false,
        isCritical: false,
        messageCount: 3,
        modelCallCount: 2,
        cacheTokensReported: false,
      }));
    });

    await act(async () => {
      resolveStats({
        conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
        promptTokens: 1000,
        completionTokens: 200,
        totalTokens: 1200,
        contextTokens: 900,
        messageCount: 2,
        mostUsedModel: 'claude',
        contextUsage: 10,
        contextLimit: 10000,
        isNearLimit: false,
        isCritical: false,
        systemPromptEstimatedTokens: 5,
        summaryTokens: 3,
        messagesInContextTokens: 15,
        messagesOutOfContextTokens: 7,
        messagesInContextCount: 1,
        messagesOutOfContextCount: 0,
        toolsUsedCount: 0,
        toolBreakdown: [],
      });
    });

    await waitFor(() => {
      expect(screen.getAllByText('1.600').length).toBeGreaterThan(0);
    });
    expect(screen.getByText('claude')).toBeInTheDocument();
  });

  it('ignora evento parcial antes do snapshot inicial', async () => {
    let resolveStats!: (value: Record<string, unknown>) => void;
    getStatsSpy.mockReturnValue(new Promise((resolve) => {
      resolveStats = resolve;
    }));

    render(
      <TokenStatsModal
        conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"}
        isOpen={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(eventCallbacks['chat:token_stats_update']?.length).toBeGreaterThan(0);
    });

    act(() => {
      eventCallbacks['chat:token_stats_update']?.forEach((callback) => callback({
        conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
        cacheReadTokens: 300,
        cacheTokensReported: true,
      }));
    });

    expect(screen.getByText('tokenStats.loading')).toBeInTheDocument();
    expect(screen.queryByText('tokenStats.contextUsage')).not.toBeInTheDocument();

    await act(async () => {
      resolveStats({
        conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
        promptTokens: 1000,
        completionTokens: 200,
        totalTokens: 1200,
        cacheHitRate: 0,
        cacheTokensReported: false,
        promptCacheEnabled: true,
        contextTokens: 900,
        messageCount: 2,
        mostUsedModel: 'claude',
        contextUsage: 10,
        contextLimit: 10000,
        isNearLimit: false,
        isCritical: false,
        systemPromptEstimatedTokens: 5,
        summaryTokens: 3,
        messagesInContextTokens: 15,
        messagesOutOfContextTokens: 7,
        messagesInContextCount: 1,
        messagesOutOfContextCount: 0,
        toolsUsedCount: 0,
        toolBreakdown: [],
      });
    });

    await waitFor(() => {
      expect(screen.getByText('tokenStats.contextUsage')).toBeInTheDocument();
    });
  });

  it('mostra fallback sem inferir warning quando métricas de cache estão ausentes', async () => {
    getStatsSpy.mockResolvedValue({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 1000,
      completionTokens: 200,
      totalTokens: 1200,
      cacheHitRate: 0,
      cacheTokensReported: false,
      promptCacheEnabled: true,
      contextTokens: 900,
      messageCount: 2,
      mostUsedModel: 'claude',
      contextUsage: 10,
      contextLimit: 10000,
      isNearLimit: false,
      isCritical: false,
      systemPromptEstimatedTokens: 5,
      summaryTokens: 3,
      messagesInContextTokens: 15,
      messagesOutOfContextTokens: 7,
      messagesInContextCount: 1,
      messagesOutOfContextCount: 0,
      toolsUsedCount: 0,
      toolBreakdown: [],
    });

    render(
      <TokenStatsModal
        conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"}
        isOpen={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('tokenStats.contextUsage')).toBeInTheDocument();
    });
    const cacheTab = screen.getByRole('tab', { name: 'tokenStats.tabPromptCache' });
    fireEvent.click(cacheTab);

    expect(screen.getByText('tokenStats.cacheEnabledNotReportedNote')).toBeInTheDocument();
    expect(screen.queryByText('tokenStats.cacheUnavailableNote')).not.toBeInTheDocument();
    expect(screen.queryByText('0.0%')).not.toBeInTheDocument();
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(7);
  });

  it('mostra fallback genérico quando estado do perfil é desconhecido', async () => {
    getStatsSpy.mockResolvedValue({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 1000,
      completionTokens: 200,
      totalTokens: 1200,
      cacheHitRate: 0,
      cacheTokensReported: false,
      contextTokens: 900,
      messageCount: 2,
      mostUsedModel: 'claude',
      contextUsage: 10,
      contextLimit: 10000,
      isNearLimit: false,
      isCritical: false,
      systemPromptEstimatedTokens: 5,
      summaryTokens: 3,
      messagesInContextTokens: 15,
      messagesOutOfContextTokens: 7,
      messagesInContextCount: 1,
      messagesOutOfContextCount: 0,
      toolsUsedCount: 0,
      toolBreakdown: [],
    });

    render(
      <TokenStatsModal
        conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"}
        isOpen={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('tokenStats.contextUsage')).toBeInTheDocument();
    });
    const cacheTab = screen.getByRole('tab', { name: 'tokenStats.tabPromptCache' });
    fireEvent.click(cacheTab);

    expect(screen.getByText('tokenStats.cacheUnavailableNote')).toBeInTheDocument();
    expect(screen.queryByText('tokenStats.cacheEnabledNotReportedNote')).not.toBeInTheDocument();
  });

  it('explica quando controles explícitos de prompt cache não estão habilitados', async () => {
    getStatsSpy.mockResolvedValue({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 1000,
      completionTokens: 200,
      totalTokens: 1200,
      cacheHitRate: 0,
      cacheTokensReported: false,
      promptCacheEnabled: false,
      contextTokens: 900,
      messageCount: 2,
      mostUsedModel: 'claude',
      contextUsage: 10,
      contextLimit: 10000,
      isNearLimit: false,
      isCritical: false,
      systemPromptEstimatedTokens: 5,
      summaryTokens: 3,
      messagesInContextTokens: 15,
      messagesOutOfContextTokens: 7,
      messagesInContextCount: 1,
      messagesOutOfContextCount: 0,
      toolsUsedCount: 0,
      toolBreakdown: [],
    });

    render(
      <TokenStatsModal
        conversationId={"01926b90-7a5a-7c4e-8d3f-000000000001"}
        isOpen={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('tokenStats.contextUsage')).toBeInTheDocument();
    });
    const cacheTab = screen.getByRole('tab', { name: 'tokenStats.tabPromptCache' });
    fireEvent.click(cacheTab);

    expect(screen.getByText('tokenStats.cacheProfileControlsDisabledNote')).toBeInTheDocument();
    expect(screen.queryByText('tokenStats.cacheUnavailableNote')).not.toBeInTheDocument();
  });
});
