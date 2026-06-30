import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { BasePicker } from './BasePicker';

const announceMock = vi.hoisted(() => vi.fn());

vi.mock('./Combobox', () => ({
  Combobox: (props: { items: Array<{ value: string; label: string }>; selected: string; onSelect: (value: string) => void }) => (
    <div>
      <button onClick={() => props.onSelect(props.items[0]?.value || '')}>Selecionar</button>
      <div data-testid="combobox" data-items={props.items.length} data-selected={props.selected} />
    </div>
  ),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: announceMock,
  }),
}));

describe('BasePicker', () => {
  afterEach(() => {
    announceMock.mockClear();
  });

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

    expect(screen.getByText('Carregando...')).toBeInTheDocument();
  });

  it('anuncia estado de loading ao entrar em carregamento', () => {
    const { rerender } = render(
      <BasePicker
        items={[{ value: 'a', label: 'A' }]}
        selected="a"
        onSelect={() => {}}
        label="Label"
      />
    );

    expect(announceMock).not.toHaveBeenCalled();

    rerender(
      <BasePicker
        items={[]}
        selected=""
        onSelect={() => {}}
        label="Label"
        loading
        loadingLabel="Carregando opções"
      />
    );

    expect(announceMock).toHaveBeenCalledWith('Carregando opções');
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

    expect(screen.getByText('Nenhuma opção disponível')).toBeInTheDocument();
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
