import { describe, it, expect, vi, beforeAll } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ResourceSelectionSection, type ResourceSelectionSectionProps } from './ResourceSelectionSection';
import type { DataGridColumn } from '../ui/DataGrid';

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, defaultValue: string) => defaultValue,
  }),
}));

vi.mock('../../services/audioFeedback', () => ({
  playBumpSound: vi.fn(),
}));

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

interface TestRow {
  id: string;
  name: string;
}

const columns: DataGridColumn<TestRow>[] = [
  { key: 'name', label: 'Nome' },
];

function renderSection(overrides: Partial<ResourceSelectionSectionProps<TestRow>> = {}) {
  const props: ResourceSelectionSectionProps<TestRow> = {
    title: 'Recursos',
    isOpen: true,
    onToggle: vi.fn(),
    badge: 'on',
    hasItems: true,
    hint: 'Selecione recursos.',
    searchValue: '',
    onSearchChange: vi.fn(),
    searchPlaceholder: 'Buscar recurso',
    searchLabel: 'Filtrar recursos por nome',
    searchTestId: 'resources-search',
    toolbarLabel: 'Ações de seleção',
    toolbarTestId: 'resources-toolbar',
    showSelectAll: true,
    showDeselectAll: true,
    onSelectFiltered: vi.fn(),
    onDeselectFiltered: vi.fn(),
    selectAllLabel: 'Selecionar todas',
    deselectAllLabel: 'Desmarcar todas',
    selectAllTestId: 'resources-select-all',
    deselectAllTestId: 'resources-deselect-all',
    rows: [{ id: 'one', name: 'Recurso 1' }],
    columns,
    gridLabel: 'Lista de recursos',
    getItemId: (item) => item.id,
    selectedIds: new Set<string | number>(),
    onSelectionChange: vi.fn(),
    noResultsMessage: 'Nenhum resultado.',
    emptyMessage: 'Nenhum recurso.',
    ...overrides,
  };

  render(<ResourceSelectionSection<TestRow> {...props} />);
  return props;
}

describe('ResourceSelectionSection', () => {
  it('renderiza busca, toolbar, grid e conteúdo extra', () => {
    renderSection({ children: <div>Configuração extra</div> });

    expect(screen.getByText('Selecione recursos.')).toBeInTheDocument();
    expect(screen.getByTestId('resources-search')).toBeInTheDocument();
    expect(screen.getByRole('toolbar', { name: 'Ações de seleção' })).toBeInTheDocument();
    expect(screen.getByRole('grid', { name: 'Lista de recursos' })).toBeInTheDocument();
    expect(screen.getByText('Configuração extra')).toBeInTheDocument();
  });

  it('encaminha eventos de busca e seleção filtrada', () => {
    const props = renderSection();

    fireEvent.change(screen.getByTestId('resources-search'), { target: { value: 'abc' } });
    fireEvent.click(screen.getByTestId('resources-select-all'));
    fireEvent.click(screen.getByTestId('resources-deselect-all'));

    expect(props.onSearchChange).toHaveBeenCalledWith('abc');
    expect(props.onSelectFiltered).toHaveBeenCalled();
    expect(props.onDeselectFiltered).toHaveBeenCalled();
  });

  it('mostra mensagem vazia quando não há itens disponíveis', () => {
    renderSection({ hasItems: false, rows: [] });

    expect(screen.getByText('Nenhum recurso.')).toBeInTheDocument();
    expect(screen.queryByRole('grid')).not.toBeInTheDocument();
  });

  it('mostra mensagem de filtro sem resultados', () => {
    renderSection({ rows: [] });

    expect(screen.getByText('Nenhum resultado.')).toBeInTheDocument();
  });
});
