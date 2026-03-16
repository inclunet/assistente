import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Textarea } from './Textarea';

describe('Textarea', () => {
  it('renderiza label, hint e erro', () => {
    render(
      <Textarea
        label="Descricao"
        hint="Explique"
        error="Obrigatorio"
      />
    );

    const textarea = screen.getByLabelText('Descricao');
    expect(textarea).toHaveAttribute('aria-invalid', 'true');

    const hint = screen.getByText('Explique');
    const error = screen.getByRole('alert', { name: '' });

    const describedBy = textarea.getAttribute('aria-describedby') || '';
    expect(describedBy).toContain(hint.getAttribute('id') || '');
    expect(describedBy).toContain(error.getAttribute('id') || '');
  });
});
