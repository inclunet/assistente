import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Toast } from './Toast';

describe('Toast', () => {
  it('fecha automaticamente apos duracao', () => {
    vi.useFakeTimers();
    const onClose = vi.fn();

    render(<Toast message="Salvo" duration={500} onClose={onClose} />);

    vi.advanceTimersByTime(500);
    expect(onClose).toHaveBeenCalled();

    vi.useRealTimers();
  });

  it('fecha ao clicar no botao', () => {
    const onClose = vi.fn();

    render(<Toast message="Salvo" onClose={onClose} />);

    fireEvent.click(screen.getByRole('button', { name: 'ui.toast.close' }));
    expect(onClose).toHaveBeenCalled();
  });
});
