import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Button } from './Button';

describe('Button', () => {
  it('renderiza conteudo e classes do variant', () => {
    render(
      <Button variant="danger" size="lg" fullWidth>
        Acao
      </Button>
    );

    const button = screen.getByRole('button', { name: 'Acao' });
    expect(button).toHaveClass('btn--danger');
    expect(button).toHaveClass('btn--lg');
    expect(button).toHaveClass('btn--full-width');
  });

  it('mostra spinner e desabilita quando loading', () => {
    render(<Button loading>Salvar</Button>);

    const button = screen.getByRole('button');
    expect(button).toBeDisabled();
    expect(button.querySelector('.btn__spinner')).toBeTruthy();
  });
});
