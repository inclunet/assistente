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

  it('nao fecha automaticamente quando duration <= 0 (persistente)', () => {
    vi.useFakeTimers();
    const onClose = vi.fn();

    render(<Toast message="Aviso persistente" duration={0} onClose={onClose} />);

    vi.advanceTimersByTime(10000);
    expect(onClose).not.toHaveBeenCalled();

    vi.useRealTimers();
  });

  it('fecha ao clicar no botao', () => {
    const onClose = vi.fn();

    render(<Toast message="Salvo" onClose={onClose} />);

    fireEvent.click(screen.getByRole('button', { name: 'ui.toast.close' }));
    expect(onClose).toHaveBeenCalled();
  });

  it('renderiza o botao de acao e dispara o callback ao clicar', () => {
    const onClose = vi.fn();
    const onClick = vi.fn();

    render(
      <Toast
        message="Runtime parcial"
        variant="warning"
        duration={0}
        action={{ label: 'Tentar novamente', onClick }}
        onClose={onClose}
      />,
    );

    const actionButton = screen.getByRole('button', { name: 'Tentar novamente' });
    expect(actionButton).toBeInTheDocument();

    fireEvent.click(actionButton);
    expect(onClick).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
  });
});
