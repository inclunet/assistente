import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { BasePicker } from './BasePicker';

vi.mock('./Combobox', () => ({
  Combobox: (props: { items: Array<{ value: string; label: string }>; selected: string; onSelect: (value: string) => void }) => (
    <div>
      <button onClick={() => props.onSelect(props.items[0]?.value || '')}>Selecionar</button>
      <div data-testid="combobox" data-items={props.items.length} data-selected={props.selected} />
    </div>
  ),
}));

describe('BasePicker', () => {
  it('renderiza estado de loading', () => {
    render(
      <BasePicker
        items={[]}
        selected=""
        onSelect={() => {}}
        label="Label"
        loading
      />
    );

    expect(screen.getByRole('status')).toHaveTextContent('Carregando...');
  });

  it('renderiza estado de erro com retry', () => {
    const onRetry = vi.fn();

    render(
      <BasePicker
        items={[]}
        selected=""
        onSelect={() => {}}
        label="Label"
        error="Falhou"
        onRetry={onRetry}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'Tentar novamente' }));
    expect(onRetry).toHaveBeenCalled();
  });

  it('renderiza estado vazio', () => {
    render(
      <BasePicker
        items={[]}
        selected=""
        onSelect={() => {}}
        label="Label"
      />
    );

    expect(screen.getByRole('status')).toHaveTextContent('Nenhuma opção disponível');
  });

  it('renderiza combobox quando ha itens', () => {
    render(
      <BasePicker
        items={[{ value: 'a', label: 'A' }]}
        selected="a"
        onSelect={() => {}}
        label="Label"
      />
    );

    expect(screen.getByTestId('combobox')).toHaveAttribute('data-items', '1');
  });
});
