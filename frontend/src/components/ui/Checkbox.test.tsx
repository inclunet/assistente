import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Checkbox } from './Checkbox';

describe('Checkbox', () => {
  it('renderiza input e label', () => {
    render(<Checkbox label="Ativo" />);

    const input = screen.getByRole('checkbox', { name: 'Ativo' });
    expect(input).toHaveAttribute('type', 'checkbox');
  });
});
