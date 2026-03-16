import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { PageToolbar } from './PageToolbar';

describe('PageToolbar', () => {
  it('renderiza titulo e busca', () => {
    const onSearchChange = vi.fn();

    render(
      <PageToolbar
        title="Historico"
        searchValue=""
        onSearchChange={onSearchChange}
        searchPlaceholder="Buscar"
      />
    );

    expect(screen.getByRole('heading', { name: 'Historico' })).toBeInTheDocument();

    const input = screen.getByRole('textbox', { name: 'Buscar' });
    fireEvent.change(input, { target: { value: 'x' } });
    expect(onSearchChange).toHaveBeenCalledWith('x');
  });
});
