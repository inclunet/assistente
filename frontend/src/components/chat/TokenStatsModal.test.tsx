import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { TokenStatsModal } from './TokenStatsModal';

const getStatsSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetConversationTokenStats: (id: string) => getStatsSpy(id),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: () => () => {},
}));

describe('TokenStatsModal', () => {
  it('renderiza stats quando aberto', async () => {
    getStatsSpy.mockResolvedValueOnce({
      conversationId: "01926b90-7a5a-7c4e-8d3f-000000000001",
      promptTokens: 10,
      completionTokens: 20,
      totalTokens: 30,
      messageCount: 1,
      mostUsedModel: 'x',
      contextUsage: 10,
      contextLimit: 100,
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
  });
});
