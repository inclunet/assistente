import { describe, expect, it, vi } from 'vitest';
import { render } from '@testing-library/react';
import { DialogActions } from './DialogActions';
import { Button } from './Button';

describe('DialogActions', () => {
  it('renderiza primária antes de secundária no DOM (ordem de Tab)', () => {
    render(
      <DialogActions
        primary={<Button onClick={vi.fn()}>Confirmar</Button>}
        secondary={<Button variant="outline" onClick={vi.fn()}>Cancelar</Button>}
      />
    );

    const actions = document.querySelector('[data-dialog-actions]');
    expect(actions).not.toBeNull();
    const buttons = Array.from(actions!.querySelectorAll('button'));
    expect(buttons.map((b) => b.textContent)).toEqual(['Confirmar', 'Cancelar']);
  });

  it('permite omitir secundária', () => {
    render(<DialogActions primary={<Button onClick={vi.fn()}>Salvar</Button>} />);

    const actions = document.querySelector('[data-dialog-actions]');
    expect(actions).not.toBeNull();
    expect(Array.from(actions!.querySelectorAll('button')).map((b) => b.textContent)).toEqual(['Salvar']);
  });
});
