import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { TokenStatsButton } from './TokenStatsButton';

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

describe('TokenStatsButton', () => {
  it('renderiza stats e abre modal', async () => {
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

    const onOpenModal = vi.fn();
    render(<TokenStatsButton conversationId={1} onOpenModal={onOpenModal} />);

    await waitFor(() => {
      expect(screen.getByRole('button')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button'));
    expect(onOpenModal).toHaveBeenCalled();
  });

  it('nao renderiza sem conversationId', () => {
    const { container } = render(
      <TokenStatsButton onOpenModal={() => {}} />
    );

    expect(container.firstChild).toBeNull();
  });
});
