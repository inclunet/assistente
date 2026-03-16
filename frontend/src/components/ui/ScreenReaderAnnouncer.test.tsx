import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ScreenReaderAnnouncer } from './ScreenReaderAnnouncer';

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncerState: () => ({
    politeMessage: 'ok',
    assertiveMessage: 'alerta',
  }),
}));

describe('ScreenReaderAnnouncer', () => {
  it('renderiza regioes de anuncio', () => {
    render(<ScreenReaderAnnouncer />);

    expect(screen.getByRole('status')).toHaveTextContent('ok');
    expect(screen.getByRole('alert')).toHaveTextContent('alerta');
  });
});
