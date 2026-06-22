import { beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { TokenStatsModal } from './TokenStatsModal';

const getStatsSpy = vi.fn();
const tMock = (key: string) => (key === 'tokenStats.placeholder' ? '—' : key);

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: tMock, i18n: { language: 'pt-BR' } }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetConversationTokenStats: (id: string) => getStatsSpy(id),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: () => () => {},
}));

beforeEach(() => {
  cleanup();
  document.body.innerHTML = '';
  document.body.removeAttribute('style');
  getStatsSpy.mockReset();
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
    expect(screen.getByText('62.694 tokenStats.tokensPerCall')).toBeInTheDocument();
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
