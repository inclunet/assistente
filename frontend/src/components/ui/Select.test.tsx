import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Select } from './Select';

describe('Select', () => {
  it('renderiza opcoes e estados de acessibilidade', () => {
    render(
      <Select
        label="Canal"
        hint="Escolha um canal"
        error="Obrigatorio"
        options={[
          { value: 'a', label: 'A' },
          { value: 'b', label: 'B', disabled: true },
        ]}
      />
    );

    const select = screen.getByLabelText('Canal');
    expect(select).toHaveAttribute('aria-invalid', 'true');

    const optionB = screen.getByRole('option', { name: 'B' });
    expect(optionB).toBeDisabled();

    const describedBy = select.getAttribute('aria-describedby') || '';
    const hint = screen.getByText('Escolha um canal');
    const error = screen.getByText('Obrigatorio');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(describedBy).toContain(hint.getAttribute('id') || '');
    expect(describedBy).toContain(error.getAttribute('id') || '');
  });
});
