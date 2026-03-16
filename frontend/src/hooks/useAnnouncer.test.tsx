import { describe, expect, it } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { useAnnouncer, useAnnouncerState, announce } from './useAnnouncer';

function AnnouncerStateFixture() {
  const { politeMessage, assertiveMessage } = useAnnouncerState();
  return (
    <div>
      <span data-testid="polite">{politeMessage}</span>
      <span data-testid="assertive">{assertiveMessage}</span>
    </div>
  );
}

describe('useAnnouncer', () => {
  it('anuncia mensagens', async () => {
    render(<AnnouncerStateFixture />);

    announce('Oi', 'polite');
    await waitFor(() => {
      expect(screen.getByTestId('polite')).toHaveTextContent('Oi');
    });

    announce('Alerta', 'assertive');
    await waitFor(() => {
      expect(screen.getByTestId('assertive')).toHaveTextContent('Alerta');
    });
  });

  it('hook retorna announce', async () => {
    const Test = () => {
      const { announce: hookAnnounce } = useAnnouncer();
      hookAnnounce('Teste');
      return null;
    };

    render(<AnnouncerStateFixture />);
    render(<Test />);
    await waitFor(() => {
      expect(screen.getByTestId('polite')).toHaveTextContent('Teste');
    });
  });
});
