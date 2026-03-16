import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { FormField } from './FormField';

describe('FormField', () => {
  it('associa label ao campo e mostra descricao', () => {
    render(
      <FormField label="Nome" description="Ajuda" required>
        <input />
      </FormField>
    );

    const input = screen.getByLabelText(/Nome/);
    expect(input).toBeInTheDocument();
    expect(screen.getByText('Ajuda')).toBeInTheDocument();
  });

  it('prioriza erro sobre descricao', () => {
    render(
      <FormField label="Nome" description="Ajuda" error="Obrigatorio">
        <input />
      </FormField>
    );

    expect(screen.getByText('Obrigatorio')).toBeInTheDocument();
    expect(screen.queryByText('Ajuda')).toBeNull();
  });
});
