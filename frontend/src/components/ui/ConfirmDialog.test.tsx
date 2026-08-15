import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ConfirmDialog } from './ConfirmDialog';

describe('ConfirmDialog', () => {
  it('aciona confirmar e cancelar', () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();

    render(
      <ConfirmDialog
        isOpen={true}
        title="Apagar"
        message="Tem certeza"
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    );

    expect(screen.getByText('Tem certeza')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Cancelar' }));
    fireEvent.click(screen.getByRole('button', { name: 'Confirmar' }));

    expect(onCancel).toHaveBeenCalled();
    expect(onConfirm).toHaveBeenCalled();
  });

  it('coloca Confirmar antes de Cancelar no DOM (AEP-0090)', () => {
    render(
      <ConfirmDialog
        isOpen={true}
        title="Apagar"
        message="Tem certeza"
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />
    );

    const footerButtons = screen
      .getAllByRole('button')
      .filter((b) => b.textContent === 'Confirmar' || b.textContent === 'Cancelar');
    expect(footerButtons.map((b) => b.textContent)).toEqual(['Confirmar', 'Cancelar']);
  });
});
