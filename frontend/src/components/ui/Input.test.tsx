import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Input } from './Input';

describe('Input', () => {
  it('associa label, hint e erro', () => {
    render(
      <Input
        label="Nome"
        hint="Digite seu nome"
        error="Obrigatorio"
        required
      />
    );

    const input = screen.getByLabelText(/Nome/);
    expect(input).toHaveAttribute('aria-invalid', 'true');

    const hint = screen.getByText('Digite seu nome');
    const error = screen.getByText('Obrigatorio');
    expect(error).toHaveTextContent('Obrigatorio');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    const describedBy = input.getAttribute('aria-describedby') || '';
    expect(describedBy).toContain(hint.getAttribute('id') || '');
    expect(describedBy).toContain(error.getAttribute('id') || '');
  });
});
