import { useState, useEffect, useRef, KeyboardEvent, useCallback } from 'react';
import { playBumpSound } from '../../services/audioFeedback';
import './DataGrid.css';

export interface DataGridColumn<T = any> {
  key: string;
  label: string;
  width?: string;
  truncate?: boolean;
  action?: boolean;
  actionIcon?: string;
  actionLabel?: string; // Texto acessível para leitores de tela (ex: "Abrir", "Editar", "Excluir")
  editable?: boolean;
  format?: (value: any, item: T) => string | React.ReactNode;
}

export interface DataGridProps<T = any> {
  items: T[];
  columns: DataGridColumn<T>[];
  label?: string;
  getItemId?: (item: T) => string | number;
  selectedIds?: Set<string | number>;
  multiSelect?: boolean;
  onSelectionChange?: (selectedIds: Set<string | number>) => void;
  onActivate?: (item: T, rowIndex: number) => void;
  onDelete?: (item: T, rowIndex: number) => void;
  onCellAction?: (item: T, column: DataGridColumn<T>, rowIndex: number, colIndex: number) => void;
  onCellEdit?: (item: T, column: DataGridColumn<T>, newValue: string, rowIndex: number, colIndex: number) => void;
  onGridReady?: (focusFirstCell: () => void) => void;
}

export function DataGrid<T = any>({
  items,
  columns,
  label = 'Grid de dados',
  getItemId = (item: any) => item.id,
  selectedIds,
  multiSelect = false,
  onSelectionChange,
  onActivate,
  onDelete,
  onCellAction,
  onCellEdit,
  onGridReady,
}: DataGridProps<T>) {
  const [focusedRow, setFocusedRow] = useState(0);
  const [focusedCol, setFocusedCol] = useState(0);
  const [editingRow, setEditingRow] = useState(-1);
  const [editingCol, setEditingCol] = useState(-1);
  const [editValue, setEditValue] = useState('');
  const [localSelectedIds, setLocalSelectedIds] = useState<Set<string | number>>(new Set(selectedIds || []));
  const [announcement, setAnnouncement] = useState('');
  
  const gridRef = useRef<HTMLDivElement>(null);
  const editInputRef = useRef<HTMLInputElement>(null);
  const cellRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const announceTimerRef = useRef<NodeJS.Timeout>();
  const hasInitializedRef = useRef(false); // Para garantir foco inicial apenas uma vez

  const rowCount = items.length;
  const columnCount = columns.length;

  // Cria função estável para focar a primeira célula
  const focusFirstCell = useCallback(() => {
    console.log('focusFirstCell chamada');
    // Sempre pega as células disponíveis no momento da chamada
    const cellKey = '0-0';
    const cellElement = cellRefs.current.get(cellKey);
    console.log('Célula encontrada:', {
      cellKey,
      cellElement: !!cellElement,
      totalCells: cellRefs.current.size
    });
    if (cellElement) {
      cellElement.focus();
      console.log('Foco definido na célula');
    }
  }, []); // cellRefs é uma ref e não muda

  // Fornece a função ao parent - apenas uma vez no mount
  useEffect(() => {
    if (onGridReady) {
      onGridReady(focusFirstCell);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // Apenas no mount para evitar re-registro

  const announce = (message: string) => {
    // Limpa o timer anterior se existir
    if (announceTimerRef.current) {
      clearTimeout(announceTimerRef.current);
    }
    
    setAnnouncement(message);
    
    // Tempo suficiente para leitores de tela processarem
    announceTimerRef.current = setTimeout(() => {
      setAnnouncement('');
    }, 3000);
  };

  // Foca no grid quando montado (se houver items) - apenas uma vez
  useEffect(() => {
    // Aguarda os dados carregarem
    const checkTimer = setInterval(() => {
      if (items.length > 0 && !hasInitializedRef.current) {
        hasInitializedRef.current = true;
        clearInterval(checkTimer);
        
        // Pequeno delay para garantir que o grid foi renderizado
        setTimeout(() => {
          // Só foca se nenhum elemento do grid já está focado
          const activeElement = document.activeElement;
          const isSearchFocused = activeElement?.classList.contains('toolbar__search');
          const isGridCellFocused = activeElement?.classList.contains('grid-cell');
          
          // Foca no grid, exceto se o search field já estiver focado ou uma célula já estiver focada
          if (!isSearchFocused && !isGridCellFocused) {
            focusFirstCell();
          }
        }, 100);
      }
    }, 100);
    
    return () => {
      clearInterval(checkTimer);
    };
  }, []); // Array vazio - roda apenas no mount

  // Sincroniza selectedIds externo com estado local
  useEffect(() => {
    if (selectedIds) {
      setLocalSelectedIds(new Set(selectedIds));
    }
  }, [selectedIds ? Array.from(selectedIds).join(',') : '']);

  // Foca no input ao editar
  useEffect(() => {
    if (editingRow >= 0 && editInputRef.current) {
      editInputRef.current.focus();
      editInputRef.current.select();
    }
  }, [editingRow, editingCol]);

  // Função auxiliar para focar célula - chamada explicitamente quando necessário
  const focusCell = useCallback((row: number, col: number) => {
    if (rowCount === 0 || columnCount === 0) return;
    
    const cellKey = `${row}-${col}`;
    const cellElement = cellRefs.current.get(cellKey);
    if (cellElement && document.activeElement !== cellElement) {
      cellElement.scrollIntoView({ block: 'nearest', inline: 'nearest' });
      cellElement.focus();
    }
  }, [rowCount, columnCount]);

  const toggleSelection = (rowIndex: number) => {
    const itemId = getItemId(items[rowIndex]);
    const newSelected = new Set(localSelectedIds);
    
    if (newSelected.has(itemId)) {
      newSelected.delete(itemId);
      announce(`Desmarcado. ${newSelected.size} selecionados`);
    } else {
      newSelected.add(itemId);
      announce(`Marcado. ${newSelected.size} selecionados`);
    }
    
    setLocalSelectedIds(newSelected);
    onSelectionChange?.(newSelected);
  };

  const selectAll = () => {
    const allIds = new Set(items.map(item => getItemId(item)));
    setLocalSelectedIds(allIds);
    onSelectionChange?.(allIds);
    announce(`${allIds.size} itens selecionados`);
  };

  const clearSelection = () => {
    setLocalSelectedIds(new Set());
    onSelectionChange?.(new Set());
    announce('Seleção limpa');
  };

  const startEditing = (rowIndex: number, colIndex: number) => {
    const col = columns[colIndex];
    if (col.action || !col.editable) return;
    
    const item = items[rowIndex];
    const value = item[col.key as keyof T];
    setEditingRow(rowIndex);
    setEditingCol(colIndex);
    setEditValue(String(value || ''));
    announce('Editando');
  };

  const saveEdit = () => {
    if (editingRow >= 0 && editingCol >= 0) {
      const item = items[editingRow];
      const column = columns[editingCol];
      onCellEdit?.(item, column, editValue, editingRow, editingCol);
      announce('Salvo');
    }
    cancelEdit();
  };

  const cancelEdit = () => {
    if (editingRow >= 0) {
      announce('Cancelado');
    }
    setEditingRow(-1);
    setEditingCol(-1);
    setEditValue('');
    gridRef.current?.focus();
  };

  const handleKeyDown = (event: KeyboardEvent) => {
    if (rowCount === 0 || columnCount === 0) return;

    // Se estamos editando
    if (editingRow >= 0) {
      if (event.key === 'Enter') {
        event.preventDefault();
        saveEdit();
      } else if (event.key === 'Escape') {
        event.preventDefault();
        cancelEdit();
      }
      return;
    }

    let handled = false;
    let newRow = focusedRow;
    let newCol = focusedCol;

    switch (event.key) {
      case ' ':
        event.preventDefault();
        if (event.ctrlKey && multiSelect) {
          toggleSelection(focusedRow);
        } else {
          const col = columns[focusedCol];
          if (col.action) {
            onCellAction?.(items[focusedRow], col, focusedRow, focusedCol);
          }
        }
        return;

      case 'Enter':
        event.preventDefault();
        const colEnter = columns[focusedCol];
        if (colEnter.action) {
          // Se for célula de ação, executa a ação
          onCellAction?.(items[focusedRow], colEnter, focusedRow, focusedCol);
        } else {
          // Se não for ação, ativa o item (abre/edita)
          onActivate?.(items[focusedRow], focusedRow);
        }
        return;

      case 'Delete':
        event.preventDefault();
        onDelete?.(items[focusedRow], focusedRow);
        return;

      case 'F2':
        event.preventDefault();
        startEditing(focusedRow, focusedCol);
        return;

      case 'Escape':
        event.preventDefault();
        if (multiSelect) {
          clearSelection();
        }
        return;

      case 'a':
        if (event.ctrlKey && multiSelect) {
          event.preventDefault();
          selectAll();
          return;
        }
        break;

      case 'ArrowUp':
        event.preventDefault();
        if (focusedRow === 0) {
          playBumpSound();
          return;
        }
        newRow = Math.max(0, focusedRow - 1);
        handled = true;
        break;

      case 'ArrowDown':
        event.preventDefault();
        if (focusedRow === rowCount - 1) {
          playBumpSound();
          return;
        }
        newRow = Math.min(rowCount - 1, focusedRow + 1);
        handled = true;
        break;

      case 'ArrowLeft':
        event.preventDefault();
        if (focusedCol === 0) {
          playBumpSound();
          return;
        }
        newCol = Math.max(0, focusedCol - 1);
        handled = true;
        break;

      case 'ArrowRight':
        event.preventDefault();
        if (focusedCol === columnCount - 1) {
          playBumpSound();
          return;
        }
        newCol = Math.min(columnCount - 1, focusedCol + 1);
        handled = true;
        break;

      case 'Home':
        event.preventDefault();
        if (event.ctrlKey) {
          if (focusedRow === 0 && focusedCol === 0) {
            playBumpSound();
            return;
          }
          newRow = 0;
          newCol = 0;
        } else {
          if (focusedCol === 0) {
            playBumpSound();
            return;
          }
          newCol = 0;
        }
        handled = true;
        break;

      case 'End':
        event.preventDefault();
        if (event.ctrlKey) {
          if (focusedRow === rowCount - 1 && focusedCol === columnCount - 1) {
            playBumpSound();
            return;
          }
          newRow = rowCount - 1;
          newCol = columnCount - 1;
        } else {
          if (focusedCol === columnCount - 1) {
            playBumpSound();
            return;
          }
          newCol = columnCount - 1;
        }
        handled = true;
        break;

      case 'PageUp':
        event.preventDefault();
        if (focusedRow === 0) {
          playBumpSound();
          return;
        }
        newRow = Math.max(0, focusedRow - 10);
        handled = true;
        break;

      case 'PageDown':
        event.preventDefault();
        if (focusedRow === rowCount - 1) {
          playBumpSound();
          return;
        }
        newRow = Math.min(rowCount - 1, focusedRow + 10);
        handled = true;
        break;
    }

    if (handled && (newRow !== focusedRow || newCol !== focusedCol)) {
      const oldRow = focusedRow;
      setFocusedRow(newRow);
      setFocusedCol(newCol);
      
      // Foca a nova célula após atualizar o estado
      setTimeout(() => focusCell(newRow, newCol), 0);

      // Seleção durante navegação
      if (multiSelect && newRow !== oldRow) {
        if (event.shiftKey) {
          // Shift: seleciona intervalo
          const start = Math.min(oldRow, newRow);
          const end = Math.max(oldRow, newRow);
          const newSelected = new Set(localSelectedIds);
          for (let i = start; i <= end; i++) {
            newSelected.add(getItemId(items[i]));
          }
          setLocalSelectedIds(newSelected);
          onSelectionChange?.(newSelected);
        } else if (!event.ctrlKey) {
          // Sem modificadores: seleciona só o atual
          const itemId = getItemId(items[newRow]);
          const newSelected = new Set([itemId]);
          setLocalSelectedIds(newSelected);
          onSelectionChange?.(newSelected);
        }
      }
    }
  };

  const handleCellClick = (rowIndex: number, colIndex: number, event: React.MouseEvent) => {
    const col = columns[colIndex];
    console.log('DataGrid handleCellClick:', { rowIndex, colIndex, columnKey: col.key, isAction: col.action });
    
    setFocusedRow(rowIndex);
    setFocusedCol(colIndex);
    
    // Se for célula de ação, executa a ação e para a propagação
    if (col.action) {
      console.log('Executando ação para coluna:', col.key);
      event.preventDefault();
      event.stopPropagation();
      onCellAction?.(items[rowIndex], col, rowIndex, colIndex);
      // Foca a célula clicada após a ação
      setTimeout(() => focusCell(rowIndex, colIndex), 0);
      return;
    }
    
    // Foca a célula clicada
    setTimeout(() => focusCell(rowIndex, colIndex), 0);

    if (event.detail === 2) {
      // Double click para editar
      startEditing(rowIndex, colIndex);
    } else if (multiSelect && event.ctrlKey) {
      toggleSelection(rowIndex);
    }
  };

  const renderCellContent = (item: T, column: DataGridColumn<T>, rowIndex: number, colIndex: number) => {
    if (editingRow === rowIndex && editingCol === colIndex) {
      return (
        <input
          ref={editInputRef}
          type="text"
          value={editValue}
          onChange={(e) => setEditValue(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              saveEdit();
            } else if (e.key === 'Escape') {
              e.preventDefault();
              cancelEdit();
            }
            e.stopPropagation();
          }}
          onBlur={saveEdit}
          className="cell-edit-input"
        />
      );
    }

    const value = item[column.key as keyof T];
    
    if (column.action) {
      return (
        <button
          type="button"
          className="action-button"
          aria-label={column.actionLabel || column.label}
          onClick={(e) => {
            console.log('Action button clicked:', column.key);
            e.stopPropagation();
            e.preventDefault();
            onCellAction?.(item, column, rowIndex, colIndex);
          }}
        >
          <span className="action-icon">{column.actionIcon || '⋮'}</span>
        </button>
      );
    }
    
    if (column.format) {
      return column.format(value, item);
    }
    
    return String(value || '');
  };

  if (rowCount === 0) {
    return (
      <div className="datagrid-empty" role="status">
        Nenhum item para exibir
      </div>
    );
  }

  return (
    <div
      ref={gridRef}
      className="datagrid-container"
      role="grid"
      aria-label={label}
      aria-rowcount={rowCount}
      aria-colcount={columnCount}
      aria-describedby="datagrid-instructions"
      onKeyDown={handleKeyDown}
      onClick={() => {
        // Foca a primeira célula quando o grid é clicado
        if (items.length > 0 && columns.length > 0) {
          focusCell(focusedRow, focusedCol);
        }
      }}
    >
      {/* Screen reader announcements - Usa dois elementos alternados para garantir que sempre haja mudança no DOM */}
      <div
        role="status"
        aria-live="assertive"
        aria-atomic="true"
        className="sr-only"
        key="announce-1"
      >
        {announcement}
      </div>
      <div
        role="status"
        aria-live="assertive"
        aria-atomic="true"
        className="sr-only"
        style={{ position: 'absolute' }}
        key="announce-2"
      >
        {/* Reserva para anúncios urgentes */}
      </div>

      {/* Keyboard instructions */}
      <div id="datagrid-instructions" className="sr-only">
        Grade de dados com {rowCount} linhas e {columnCount} colunas. 
        Use as setas verticais para navegar entre linhas. 
        Use as setas horizontais para navegar entre colunas. 
        Pressione Enter para ativar um item. 
        Pressione Espaço para marcar ou desmarcar. 
        {multiSelect && 'Pressione Ctrl+A para selecionar todos. '} 
        Pressione Delete para remover. 
        Pressione F2 para editar. 
        Pressione Escape para limpar a seleção.
      </div>

      <div className="datagrid-header" role="row">
        {columns.map((column, colIndex) => (
          <div
            key={column.key}
            className="datagrid-header-cell"
            role="columnheader"
            style={{ width: column.width }}
            aria-colindex={colIndex + 1}
          >
            {column.label}
          </div>
        ))}
      </div>
      
      <div className="datagrid-body">
        {items.map((item, rowIndex) => {
          const itemId = getItemId(item);
          const isSelected = localSelectedIds.has(itemId);
          const isFocused = rowIndex === focusedRow;

          return (
            <div
              key={itemId}
              className={`datagrid-row ${isSelected ? 'selected' : ''} ${isFocused ? 'focused' : ''}`}
              role="row"
              aria-rowindex={rowIndex + 1}
              aria-selected={isSelected}
            >
              {columns.map((column, colIndex) => {
                const isCellFocused = rowIndex === focusedRow && colIndex === focusedCol;
                const cellKey = `${rowIndex}-${colIndex}`;

                return (
                  <div
                    key={column.key}
                    ref={(el) => {
                      if (el) cellRefs.current.set(cellKey, el);
                      else cellRefs.current.delete(cellKey);
                    }}
                    className={`datagrid-cell ${isCellFocused ? 'focused' : ''} ${column.action ? 'action-cell' : ''} ${column.truncate ? 'truncate' : ''}`}
                    role="gridcell"
                    style={{ width: column.width }}
                    aria-colindex={colIndex + 1}
                    onClick={(e) => handleCellClick(rowIndex, colIndex, e)}
                    tabIndex={isCellFocused ? 0 : -1}
                  >
                    {renderCellContent(item, column, rowIndex, colIndex)}
                  </div>
                );
              })}
            </div>
          );
        })}
      </div>
    </div>
  );
}
