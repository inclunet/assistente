import { useState } from 'react';
import { describe, it, expect, vi, beforeEach, beforeAll } from 'vitest';
import { act, render, screen, fireEvent } from '@testing-library/react';
import { DataGrid, DataGridColumn } from './DataGrid';

const announceMock = vi.hoisted(() => vi.fn());

vi.mock('../../services/audioFeedback', () => ({
  playBumpSound: vi.fn(),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceMock }),
}));

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

beforeEach(() => {
  announceMock.mockClear();
});

interface TestItem {
  id: string;
  name: string;
  desc: string;
}

const items: TestItem[] = [
  { id: 'a', name: 'Alpha', desc: 'First' },
  { id: 'b', name: 'Bravo', desc: 'Second' },
  { id: 'c', name: 'Charlie', desc: 'Third' },
  { id: 'd', name: 'Delta', desc: 'Fourth' },
];

const columns: DataGridColumn<TestItem>[] = [
  { key: 'name', label: 'Nome' },
  { key: 'desc', label: 'Descrição' },
];

function getGrid() {
  return screen.getByRole('grid');
}

function getCells() {
  return screen.getAllByRole('gridcell');
}

/** Ativa o foco lazy do DataGrid — simula o usuário tabbando para o grid. */
function focusGrid() {
  fireEvent.focus(getGrid());
}

// ─── Backward compatibility (list mode) ────────────────────────────

describe('DataGrid (list mode — backward compat)', () => {
  it('renderiza itens e colunas', () => {
    render(<DataGrid items={items} columns={columns} autoFocusOnMount={false} />);
    expect(screen.getByText('Alpha')).toBeInTheDocument();
    expect(screen.getByText('Second')).toBeInTheDocument();
    expect(screen.getAllByRole('row')).toHaveLength(items.length + 1); // +1 header
  });

  it('Space sem Ctrl NÃO faz toggle de seleção em list mode', () => {
    const onSel = vi.fn();
    render(
      <DataGrid items={items} columns={columns} multiSelect
        selectedIds={new Set()} onSelectionChange={onSel} autoFocusOnMount={false} />
    );
    const grid = getGrid();
    fireEvent.keyDown(grid, { key: ' ' });
    expect(onSel).not.toHaveBeenCalled();
  });

  it('Ctrl+Space faz toggle de seleção em list mode', () => {
    const onSel = vi.fn();
    render(
      <DataGrid items={items} columns={columns} multiSelect
        selectedIds={new Set()} onSelectionChange={onSel} autoFocusOnMount={false} />
    );
    focusGrid();
    const grid = getGrid();
    fireEvent.keyDown(grid, { key: ' ', ctrlKey: true });
    expect(onSel).toHaveBeenCalled();
  });

  it('anuncia ações da superfície ativa pelo announcer global sem live region local', () => {
    const onSel = vi.fn();
    render(
      <DataGrid
        items={items}
        columns={columns}
        selectionMode="checkbox"
        selectedIds={new Set()}
        onSelectionChange={onSel}
        autoFocusOnMount={false}
      />
    );

    const firstCell = getCells()[0];
    act(() => {
      firstCell.focus();
      fireEvent.click(firstCell);
    });

    expect(announceMock).toHaveBeenCalled();
    expect(document.body.querySelector('[aria-live]')).toBeNull();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('Arrow down em list mode (sem Ctrl) seleciona só o item atual', () => {
    const onSel = vi.fn();
    render(
      <DataGrid items={items} columns={columns} multiSelect
        selectedIds={new Set()} onSelectionChange={onSel} autoFocusOnMount={false} />
    );
    focusGrid();
    const grid = getGrid();
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    expect(onSel).toHaveBeenCalledWith(new Set(['b']));
  });

  it('showHeader=true (padrão) renderiza headers', () => {
    render(<DataGrid items={items} columns={columns} autoFocusOnMount={false} />);
    expect(screen.getByText('Nome')).toBeInTheDocument();
    expect(screen.getByText('Descrição')).toBeInTheDocument();
  });

  it('className é aplicado ao container', () => {
    render(<DataGrid items={items} columns={columns} className="my-custom" autoFocusOnMount={false} />);
    expect(getGrid().classList.contains('my-custom')).toBe(true);
  });
});

// ─── Checkbox mode ─────────────────────────────────────────────────

describe('DataGrid (checkbox mode)', () => {
  let onSelectionChange: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    onSelectionChange = vi.fn();
  });

  function renderCheckbox(selected: Set<string | number> = new Set()) {
    return render(
      <DataGrid
        items={items}
        columns={columns}
        selectionMode="checkbox"
        selectedIds={selected}
        onSelectionChange={onSelectionChange}
        autoFocusOnMount={false}
      />
    );
  }

  it('Space (sem Ctrl) faz toggle de seleção', () => {
    renderCheckbox();
    focusGrid();
    fireEvent.keyDown(getGrid(), { key: ' ' });
    expect(onSelectionChange).toHaveBeenCalled();
    const set = onSelectionChange.mock.calls[0][0] as Set<string>;
    expect(set.has('a')).toBe(true);
  });

  it('Space desmarca item já selecionado', () => {
    renderCheckbox(new Set(['a']));
    focusGrid();
    fireEvent.keyDown(getGrid(), { key: ' ' });
    expect(onSelectionChange).toHaveBeenCalled();
    const set = onSelectionChange.mock.calls[0][0] as Set<string>;
    expect(set.has('a')).toBe(false);
  });

  it('Arrow navigation NÃO altera seleção', () => {
    renderCheckbox(new Set(['a']));
    focusGrid();
    const grid = getGrid();
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    expect(onSelectionChange).not.toHaveBeenCalled();
  });

  it('Click em célula faz toggle de seleção', () => {
    renderCheckbox();
    const cells = getCells();
    fireEvent.click(cells[0]);
    expect(onSelectionChange).toHaveBeenCalled();
    const set = onSelectionChange.mock.calls[0][0] as Set<string>;
    expect(set.has('a')).toBe(true);
  });

  it('Click em item já selecionado desmarca', () => {
    renderCheckbox(new Set(['b']));
    const cells = getCells();
    // 'b' is the second row, cells[2] and [3]
    fireEvent.click(cells[2]);
    expect(onSelectionChange).toHaveBeenCalled();
    const set = onSelectionChange.mock.calls[0][0] as Set<string>;
    expect(set.has('b')).toBe(false);
  });

  it('Ctrl+A seleciona todos', () => {
    renderCheckbox();
    focusGrid();
    fireEvent.keyDown(getGrid(), { key: 'a', ctrlKey: true });
    expect(onSelectionChange).toHaveBeenCalled();
    const set = onSelectionChange.mock.calls[0][0] as Set<string>;
    expect(set.size).toBe(items.length);
  });

  it('Escape limpa seleção', () => {
    renderCheckbox(new Set(['a', 'b']));
    focusGrid();
    fireEvent.keyDown(getGrid(), { key: 'Escape' });
    expect(onSelectionChange).toHaveBeenCalled();
    const set = onSelectionChange.mock.calls[0][0] as Set<string>;
    expect(set.size).toBe(0);
  });

  it('showHeader=false esconde headers', () => {
    render(
      <DataGrid items={items} columns={columns} selectionMode="checkbox"
        showHeader={false} autoFocusOnMount={false} />
    );
    expect(screen.queryByText('Nome')).not.toBeInTheDocument();
  });

  it('container tem classe datagrid-container--checkbox', () => {
    renderCheckbox();
    expect(getGrid().classList.contains('datagrid-container--checkbox')).toBe(true);
  });

  it('ignora toggle customizado quando a linha focada sai da lista', () => {
    const onItemToggle = vi.fn();
    const { rerender } = render(
      <DataGrid
        items={items}
        columns={columns}
        selectionMode="checkbox"
        onItemToggle={onItemToggle}
        autoFocusOnMount={false}
      />
    );
    focusGrid();
    fireEvent.keyDown(getGrid(), { key: 'ArrowDown' });
    fireEvent.keyDown(getGrid(), { key: 'ArrowDown' });

    rerender(
      <DataGrid
        items={[items[0]]}
        columns={columns}
        selectionMode="checkbox"
        onItemToggle={onItemToggle}
        autoFocusOnMount={false}
      />
    );
    fireEvent.keyDown(getGrid(), { key: ' ' });

    expect(onItemToggle).not.toHaveBeenCalled();
  });
});

// ─── Move item (Alt+Up/Down) ───────────────────────────────────────

describe('DataGrid (onMoveItem)', () => {
  let onMoveItem: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    onMoveItem = vi.fn();
  });

  function renderMoveable() {
    return render(
      <DataGrid
        items={items}
        columns={columns}
        selectionMode="checkbox"
        selectedIds={new Set(['a', 'b'])}
        onMoveItem={onMoveItem}
        autoFocusOnMount={false}
      />
    );
  }

  it('Alt+Down move item para baixo', () => {
    renderMoveable();
    focusGrid();
    const grid = getGrid();
    fireEvent.keyDown(grid, { key: 'ArrowDown', altKey: true });
    expect(onMoveItem).toHaveBeenCalledWith(0, 1);
  });

  it('Alt+Up no primeiro item não chama onMoveItem', () => {
    renderMoveable();
    focusGrid();
    const grid = getGrid();
    fireEvent.keyDown(grid, { key: 'ArrowUp', altKey: true });
    expect(onMoveItem).not.toHaveBeenCalled();
  });

  it('Alt+Down no último item não chama onMoveItem', () => {
    renderMoveable();
    focusGrid();
    const grid = getGrid();
    // navigate to last item
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    fireEvent.keyDown(grid, { key: 'ArrowDown' });
    // now at last item
    fireEvent.keyDown(grid, { key: 'ArrowDown', altKey: true });
    expect(onMoveItem).not.toHaveBeenCalled();
  });

  it('Alt+Up move item para cima (a partir da segunda linha)', () => {
    renderMoveable();
    focusGrid();
    const grid = getGrid();
    fireEvent.keyDown(grid, { key: 'ArrowDown' }); // focus row 1
    fireEvent.keyDown(grid, { key: 'ArrowUp', altKey: true });
    expect(onMoveItem).toHaveBeenCalledWith(1, 0);
  });

  it('sem onMoveItem, Alt+Up/Down é navegação normal', () => {
    const onSel = vi.fn();
    render(
      <DataGrid items={items} columns={columns} multiSelect
        selectedIds={new Set()} onSelectionChange={onSel} autoFocusOnMount={false} />
    );
    focusGrid();
    const grid = getGrid();
    fireEvent.keyDown(grid, { key: 'ArrowDown', altKey: true });
    // without onMoveItem prop, behaves as normal arrow
    expect(onSel).toHaveBeenCalled();
  });
});

// ─── onFocusChange ─────────────────────────────────────────────────

describe('DataGrid (onFocusChange)', () => {
  it('chama onFocusChange ao receber foco pela primeira vez', () => {
    const onFocus = vi.fn();
    render(
      <DataGrid items={items} columns={columns}
        onFocusChange={onFocus} autoFocusOnMount={false} />
    );
    // Foco lazy: não dispara no mount
    expect(onFocus).not.toHaveBeenCalled();
    // Ao receber foco, ativa e notifica com o primeiro item
    focusGrid();
    expect(onFocus).toHaveBeenCalledWith(items[0], 0);
  });

  it('chama onFocusChange ao navegar para outra linha', () => {
    const onFocus = vi.fn();
    render(
      <DataGrid items={items} columns={columns}
        onFocusChange={onFocus} autoFocusOnMount={false} />
    );
    focusGrid();
    onFocus.mockClear();
    fireEvent.keyDown(getGrid(), { key: 'ArrowDown' });
    expect(onFocus).toHaveBeenCalledWith(items[1], 1);
  });
});

// ─── Regressão: loop infinito de re-renders ─────────────────────────

describe('DataGrid (regressão: loop infinito de re-renders)', () => {
  it('NÃO chama onFocusChange repetidamente quando items é recriado com mesmos dados', () => {
    const onFocus = vi.fn();

    const items1 = [
      { id: 'a', name: 'Alpha', desc: 'First' },
      { id: 'b', name: 'Bravo', desc: 'Second' },
    ];
    const items2 = [
      { id: 'a', name: 'Alpha', desc: 'First' },
      { id: 'b', name: 'Bravo', desc: 'Second' },
    ];

    const { rerender } = render(
      <DataGrid items={items1} columns={columns}
        onFocusChange={onFocus} autoFocusOnMount={false} />
    );

    // Ativa o foco lazy
    focusGrid();
    expect(onFocus).toHaveBeenCalledTimes(1);
    onFocus.mockClear();

    rerender(
      <DataGrid items={items2} columns={columns}
        onFocusChange={onFocus} autoFocusOnMount={false} />
    );

    expect(onFocus).not.toHaveBeenCalled();
  });

  it('NÃO chama onFocusChange quando items é recriado múltiplas vezes com mesmos IDs', () => {
    const onFocus = vi.fn();

    const makeItems = () => [
      { id: 'x', name: 'X', desc: 'X desc' },
      { id: 'y', name: 'Y', desc: 'Y desc' },
      { id: 'z', name: 'Z', desc: 'Z desc' },
    ];

    const { rerender } = render(
      <DataGrid items={makeItems()} columns={columns}
        onFocusChange={onFocus} autoFocusOnMount={false} />
    );

    // Ativa o foco lazy
    focusGrid();
    expect(onFocus).toHaveBeenCalledTimes(1);
    onFocus.mockClear();

    for (let i = 0; i < 10; i++) {
      rerender(
        <DataGrid items={makeItems()} columns={columns}
          onFocusChange={onFocus} autoFocusOnMount={false} />
      );
    }

    expect(onFocus).not.toHaveBeenCalled();
  });

  it('chama onFocusChange quando o item focado é substituído por outro ID', () => {
    const onFocus = vi.fn();

    const items1 = [
      { id: 'a', name: 'Alpha', desc: 'First' },
      { id: 'b', name: 'Bravo', desc: 'Second' },
    ];

    const { rerender } = render(
      <DataGrid items={items1} columns={columns}
        onFocusChange={onFocus} autoFocusOnMount={false} />
    );

    // Ativa o foco lazy
    focusGrid();
    expect(onFocus).toHaveBeenCalledTimes(1);
    onFocus.mockClear();

    const items2 = [
      { id: 'NEW', name: 'New Item', desc: 'Replaced' },
      { id: 'b', name: 'Bravo', desc: 'Second' },
    ];

    rerender(
      <DataGrid items={items2} columns={columns}
        onFocusChange={onFocus} autoFocusOnMount={false} />
    );

    expect(onFocus).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'NEW' }),
      0
    );
  });

  it('simula cenário MCP: onFocusChange faz setState → re-render com novo array', () => {
    let renderCount = 0;

    function ParentWithInlineCallbacks() {
      const [, setFocused] = useState<TestItem | null>(null);
      const [data] = useState(items);

      renderCount++;

      const inlineItems = data.map(d => ({ ...d }));

      return (
        <DataGrid
          items={inlineItems}
          columns={columns}
          autoFocusOnMount={false}
          onFocusChange={(item) => setFocused(item as TestItem | null)}
          getItemId={(item) => item.id}
        />
      );
    }

    render(<ParentWithInlineCallbacks />);

    expect(renderCount).toBeLessThanOrEqual(4);
  });
});
