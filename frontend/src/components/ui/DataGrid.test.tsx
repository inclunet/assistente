import { describe, it, expect, vi, beforeEach, beforeAll } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { DataGrid, DataGridColumn } from './DataGrid';

vi.mock('../../services/audioFeedback', () => ({
  playBumpSound: vi.fn(),
}));

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
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
    const grid = getGrid();
    fireEvent.keyDown(grid, { key: ' ', ctrlKey: true });
    expect(onSel).toHaveBeenCalled();
  });

  it('Arrow down em list mode (sem Ctrl) seleciona só o item atual', () => {
    const onSel = vi.fn();
    render(
      <DataGrid items={items} columns={columns} multiSelect
        selectedIds={new Set()} onSelectionChange={onSel} autoFocusOnMount={false} />
    );
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
    fireEvent.keyDown(getGrid(), { key: ' ' });
    expect(onSelectionChange).toHaveBeenCalled();
    const set = onSelectionChange.mock.calls[0][0] as Set<string>;
    expect(set.has('a')).toBe(true);
  });

  it('Space desmarca item já selecionado', () => {
    renderCheckbox(new Set(['a']));
    fireEvent.keyDown(getGrid(), { key: ' ' });
    expect(onSelectionChange).toHaveBeenCalled();
    const set = onSelectionChange.mock.calls[0][0] as Set<string>;
    expect(set.has('a')).toBe(false);
  });

  it('Arrow navigation NÃO altera seleção', () => {
    renderCheckbox(new Set(['a']));
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
    fireEvent.keyDown(getGrid(), { key: 'a', ctrlKey: true });
    expect(onSelectionChange).toHaveBeenCalled();
    const set = onSelectionChange.mock.calls[0][0] as Set<string>;
    expect(set.size).toBe(items.length);
  });

  it('Escape limpa seleção', () => {
    renderCheckbox(new Set(['a', 'b']));
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
    const grid = getGrid();
    fireEvent.keyDown(grid, { key: 'ArrowDown', altKey: true });
    expect(onMoveItem).toHaveBeenCalledWith(0, 1);
  });

  it('Alt+Up no primeiro item não chama onMoveItem', () => {
    renderMoveable();
    const grid = getGrid();
    fireEvent.keyDown(grid, { key: 'ArrowUp', altKey: true });
    expect(onMoveItem).not.toHaveBeenCalled();
  });

  it('Alt+Down no último item não chama onMoveItem', () => {
    renderMoveable();
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
    const grid = getGrid();
    fireEvent.keyDown(grid, { key: 'ArrowDown', altKey: true });
    // without onMoveItem prop, behaves as normal arrow
    expect(onSel).toHaveBeenCalled();
  });
});

// ─── onFocusChange ─────────────────────────────────────────────────

describe('DataGrid (onFocusChange)', () => {
  it('chama onFocusChange no mount com o primeiro item', () => {
    const onFocus = vi.fn();
    render(
      <DataGrid items={items} columns={columns}
        onFocusChange={onFocus} autoFocusOnMount={false} />
    );
    expect(onFocus).toHaveBeenCalledWith(items[0], 0);
  });

  it('chama onFocusChange ao navegar para outra linha', () => {
    const onFocus = vi.fn();
    render(
      <DataGrid items={items} columns={columns}
        onFocusChange={onFocus} autoFocusOnMount={false} />
    );
    onFocus.mockClear();
    fireEvent.keyDown(getGrid(), { key: 'ArrowDown' });
    expect(onFocus).toHaveBeenCalledWith(items[1], 1);
  });
});
