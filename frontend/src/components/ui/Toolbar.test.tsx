import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Toolbar } from './Toolbar';

describe('Toolbar', () => {
  it('renderiza busca e dispara onSearchChange', () => {
    const onSearchChange = vi.fn();

    render(
      <Toolbar
        searchValue=""
        onSearchChange={onSearchChange}
        searchPlaceholder="Buscar"
      />
    );

    const input = screen.getByRole('textbox', { name: 'Buscar' });
    fireEvent.change(input, { target: { value: 'abc' } });
    expect(onSearchChange).toHaveBeenCalledWith('abc');
  });

  it('renderiza acoes com atalho no aria-label', () => {
    render(
      <Toolbar
        actions={[
          { key: 'save', label: 'Salvar', shortcut: 'Ctrl+S' },
        ]}
      />
    );

    expect(screen.getByRole('button', { name: 'Salvar, Ctrl+S' })).toBeInTheDocument();
  });
});
