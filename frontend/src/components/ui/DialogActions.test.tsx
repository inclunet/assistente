import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { DialogActions } from './DialogActions';
import { Button } from './Button';

describe('DialogActions', () => {
  it('renderiza primaria antes de secundaria no DOM (ordem de Tab)', () => {
    render(
      <DialogActions
        primary={<Button onClick={vi.fn()}>Confirmar</Button>}
        secondary={<Button variant="outline" onClick={vi.fn()}>Cancelar</Button>}
      />
    );

    const buttons = screen.getAllByRole('button');
    expect(buttons.map((b) => b.textContent)).toEqual(['Confirmar', 'Cancelar']);
  });

  it('permite omitir secundaria', () => {
    render(<DialogActions primary={<Button onClick={vi.fn()}>Salvar</Button>} />);

    expect(screen.getAllByRole('button').map((b) => b.textContent)).toEqual(['Salvar']);
  });
});
