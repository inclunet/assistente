import type { ReactNode } from 'react';
import { CollapsibleSection } from '../ui/CollapsibleSection';
import { DataGrid, type DataGridColumn } from '../ui/DataGrid';
import { useToolbarKeyboardNav } from '../../hooks/useToolbarKeyboardNav';

export interface ResourceSelectionSectionProps<TRow> {
  title: string;
  isOpen: boolean;
  onToggle: () => void;
  disabled?: boolean;
  badge: 'on' | 'off';
  hasItems: boolean;
  hint: string;
  searchValue: string;
  onSearchChange: (value: string) => void;
  searchPlaceholder: string;
  searchLabel: string;
  searchTestId: string;
  toolbarLabel: string;
  toolbarTestId: string;
  filterNode?: ReactNode;
  showSelectAll: boolean;
  showDeselectAll: boolean;
  onSelectFiltered: () => void;
  onDeselectFiltered: () => void;
  selectAllLabel: string;
  deselectAllLabel: string;
  selectAllTestId: string;
  deselectAllTestId: string;
  extraToolbarActions?: ReactNode;
  rows: TRow[];
  columns: DataGridColumn<TRow>[];
  gridLabel: string;
  getItemId: (item: TRow) => string | number;
  selectedIds: Set<string | number>;
  onSelectionChange: (selectedIds: Set<string | number>) => void;
  onMoveItem?: (fromIndex: number, toIndex: number) => void;
  onFocusChange?: (item: TRow | null) => void;
  onItemToggle?: (item: TRow, rowIndex: number) => void;
  gridClassName?: string;
  noResultsMessage: string;
  emptyMessage: string;
  children?: ReactNode;
}

export function ResourceSelectionSection<TRow>({
  title,
  isOpen,
  onToggle,
  disabled = false,
  badge,
  hasItems,
  hint,
  searchValue,
  onSearchChange,
  searchPlaceholder,
  searchLabel,
  searchTestId,
  toolbarLabel,
  toolbarTestId,
  filterNode,
  showSelectAll,
  showDeselectAll,
  onSelectFiltered,
  onDeselectFiltered,
  selectAllLabel,
  deselectAllLabel,
  selectAllTestId,
  deselectAllTestId,
  extraToolbarActions,
  rows,
  columns,
  gridLabel,
  getItemId,
  selectedIds,
  onSelectionChange,
  onMoveItem,
  onFocusChange,
  onItemToggle,
  gridClassName,
  noResultsMessage,
  emptyMessage,
  children,
}: ResourceSelectionSectionProps<TRow>) {
  const toolbarRef = useToolbarKeyboardNav();

  return (
    <CollapsibleSection
      title={title}
      isOpen={isOpen}
      onToggle={onToggle}
      disabled={disabled}
      badge={badge}
    >
      {hasItems ? (
        <>
          <p className="profiles-field__hint">{hint}</p>
          <input
            type="text"
            className="profiles-field__filter-search"
            placeholder={searchPlaceholder}
            value={searchValue}
            onChange={(e) => onSearchChange(e.target.value)}
            aria-label={searchLabel}
            data-testid={searchTestId}
          />
          <div
            ref={toolbarRef}
            className="profiles-field__tools-actions"
            role="toolbar"
            aria-label={toolbarLabel}
            data-testid={toolbarTestId}
          >
            {filterNode}
            {showSelectAll && (
              <button
                type="button"
                className="profiles-field__tools-toggle"
                onClick={onSelectFiltered}
                disabled={disabled}
                data-testid={selectAllTestId}
              >
                {selectAllLabel}
              </button>
            )}
            {showDeselectAll && (
              <button
                type="button"
                className="profiles-field__tools-toggle"
                onClick={onDeselectFiltered}
                disabled={disabled}
                data-testid={deselectAllTestId}
              >
                {deselectAllLabel}
              </button>
            )}
            {extraToolbarActions}
          </div>
          {rows.length > 0 ? (
            <DataGrid<TRow>
              items={rows}
              columns={columns}
              label={gridLabel}
              getItemId={getItemId}
              selectedIds={selectedIds}
              selectionMode="checkbox"
              onSelectionChange={onSelectionChange}
              onMoveItem={onMoveItem}
              onFocusChange={onFocusChange}
              onItemToggle={onItemToggle}
              showHeader={true}
              autoFocusOnMount={false}
              className={gridClassName}
            />
          ) : (
            <p className="profiles-field__hint profiles-field__no-results">
              {noResultsMessage}
            </p>
          )}
          {children}
        </>
      ) : (
        <p className="profiles-field__hint profiles-field__empty">
          {emptyMessage}
        </p>
      )}
    </CollapsibleSection>
  );
}
