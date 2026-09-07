import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { BasePicker } from './BasePicker';
import { WorkspacePanelProvider } from '../workspace/WorkspacePanelContext';

const announceRequestMock = vi.hoisted(() => vi.fn(() => true));

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
    announceRequest: announceRequestMock,
  }),
}));

describe('BasePicker', () => {
  afterEach(() => {
    announceRequestMock.mockClear();
    announceRequestMock.mockReturnValue(true);
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

    expect(announceRequestMock).not.toHaveBeenCalled();

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

    expect(announceRequestMock).toHaveBeenCalledWith({
      message: 'Carregando opções',
      origin: undefined,
      eventType: 'progress',
    });
  });

  it('inclui origem do painel nos anúncios de loading', () => {
    render(
      <WorkspacePanelProvider
        value={{
          tab: {
            id: 'tab-1',
            type: 'chat',
            conversationId: 'conversation-1',
            title: 'Conversa ativa',
            position: 0,
          },
          isActive: true,
        }}
      >
        <BasePicker
          items={[]}
          selected=""
          onSelect={() => {}}
          label="Label"
          loading
          loadingLabel="Carregando opções"
        />
      </WorkspacePanelProvider>
    );

    expect(announceRequestMock).toHaveBeenCalledWith({
      message: 'Carregando opções',
      origin: expect.objectContaining({
        tabId: 'tab-1',
        surfaceId: 'tab-1',
        conversationId: 'conversation-1',
        surfaceType: 'chat',
        title: 'Conversa ativa',
      }),
      eventType: 'progress',
    });
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

    expect(announceRequestMock).toHaveBeenCalledWith({
      message: 'Falhou',
      origin: undefined,
      eventType: 'error',
      announcePriority: 'assertive',
    });
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
