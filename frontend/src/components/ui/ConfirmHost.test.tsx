import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ConfirmHost } from './ConfirmHost';

const confirmSpy = vi.fn();
const cancelSpy = vi.fn();

vi.mock('../../store/confirmStore', () => ({
  useConfirmStore: (selector: (state: {
    active: { title: string; message: string; confirmText?: string; cancelText?: string } | null;
    confirm: () => void;
    cancel: () => void;
  }) => unknown) =>
    selector({
      active: { title: 'Apagar', message: 'Tem certeza', confirmText: 'Confirmar', cancelText: 'Cancelar' },
      confirm: confirmSpy,
      cancel: cancelSpy,
    }),
}));

describe('ConfirmHost', () => {
  it('renderiza dialogo e aciona callbacks', () => {
    render(<ConfirmHost />);

    expect(screen.getByText('Tem certeza')).toBeInTheDocument();

    const footerButtons = screen
      .getAllByRole('button')
      .filter((b) => b.textContent === 'Confirmar' || b.textContent === 'Cancelar');
    expect(footerButtons.map((b) => b.textContent)).toEqual(['Confirmar', 'Cancelar']);

    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }));
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar' }));

    expect(cancelSpy).toHaveBeenCalled();
    expect(confirmSpy).toHaveBeenCalled();
  });
});
