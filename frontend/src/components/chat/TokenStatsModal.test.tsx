import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { TokenStatsModal } from './TokenStatsModal';

const getStatsSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@wailsjs/go/main/App', () => ({
  GetConversationTokenStats: (id: number) => getStatsSpy(id),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: () => () => {},
}));

describe('TokenStatsModal', () => {
  it('renderiza stats quando aberto', async () => {
    getStatsSpy.mockResolvedValueOnce({
      conversationId: 1,
      promptTokens: 10,
      completionTokens: 20,
      totalTokens: 30,
      messageCount: 1,
      mostUsedModel: 'x',
      contextUsage: 10,
      contextLimit: 100,
      isNearLimit: false,
      isCritical: false,
    });

    render(
      <TokenStatsModal
        conversationId={1}
        isOpen={true}
        onClose={() => {}}
      />
    );

    await waitFor(() => {
      expect(screen.getByText('tokenStats.contextUsage')).toBeInTheDocument();
    });
  });
});
