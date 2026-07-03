import { useState, useEffect, useRef, useCallback, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { ContextMenu, MenuItem } from './menu';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { playBumpSound } from '../../services/audioFeedback';
import './DataGrid.css';

export interface DataGridColumn<T = unknown> {
  key: string;
  label: string;
  width?: string;
  truncate?: boolean;
  action?: boolean;
  actionIcon?: string;
  actionLabel?: string; // Texto acessível para leitores de tela (ex: "Abrir", "Editar", "Excluir")
  editable?: boolean;
  selectionToggle?: boolean;
  format?: (value: unknown, item: T) => string | React.ReactNode;
}

export interface DataGridProps<T = unknown> {
  items: T[];
  columns: DataGridColumn<T>[];
  label?: string;
  autoFocusOnMount?: boolean;
  getItemId?: (item: T) => string | number;
  selectedIds?: Set<string | number>;
  multiSelect?: boolean;
  selectionMode?: 'list' | 'checkbox';
  onSelectionChange?: (selectedIds: Set<string | number>) => void;
  onActivate?: (item: T, rowIndex: number) => void;
  onDelete?: (item: T, rowIndex: number) => void;
  onCellAction?: (item: T, column: DataGridColumn<T>, rowIndex: number, colIndex: number) => void;
  onCellEdit?: (item: T, column: DataGridColumn<T>, newValue: string, rowIndex: number, colIndex: number) => void;
  onGridReady?: (focusFirstCell: () => void) => void;
  onMoveItem?: (fromIndex: number, toIndex: number) => void;
  onFocusChange?: (item: T | null, rowIndex: number) => void;
  onNearEnd?: () => void;
  nearEndThreshold?: number;
  onItemToggle?: (item: T, rowIndex: number) => void;
  className?: string;
  showHeader?: boolean;
  /**
   * Função que retorna as ações contextuais para um item (linha) do grid.
   * Usada para o menu de contexto e coluna de ações.
   */
  getRowActions?: (item: T) => MenuItem[];
}

export function DataGrid<T = unknown>({
  items,
  columns,
  label = 'Grid de dados',
  autoFocusOnMount = true,
  getItemId = (item: T) => (item as { id?: string | number }).id ?? '',
  selectedIds,
  multiSelect = false,
  selectionMode = 'list',
  onSelectionChange,
  onActivate,
  onDelete,
  onCellAction,
  onCellEdit,
  onGridReady,
  onMoveItem,
  onFocusChange,
  onNearEnd,
  nearEndThreshold = 8,
  onItemToggle,
  className,
  showHeader = true,
  getRowActions,
}: DataGridProps<T>) {
  const { t } = useTranslation();
  const { announce: announceGlobally } = useAnnouncer();
  // Foco lazy: começa em -1 (nenhuma linha focada).
  // Só inicializa quando o grid recebe foco real do usuário.
  // Isso evita que leitores de tela leiam o conteúdo ao montar/remontar.
  const [focusedRow, setFocusedRow] = useState(-1);
  const [focusedCol, setFocusedCol] = useState(0);
  const [editingRow, setEditingRow] = useState(-1);
  const [editingCol, setEditingCol] = useState(-1);
  const [editValue, setEditValue] = useState('');
  const [localSelectedIds, setLocalSelectedIds] = useState<Set<string | number>>(new Set(selectedIds || []));
  
  const gridRef = useRef<HTMLDivElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const editInputRef = useRef<HTMLInputElement>(null);
  const instructionsId = useId().replace(/[^a-zA-Z0-9_-]/g, '') + '-instructions';
  const cellRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const focusTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const hasInitializedRef = useRef(false);
  // Indica que o grid já recebeu foco pelo menos uma vez
  const hasReceivedFocusRef = useRef(false);
  const focusedItemIdRef = useRef<string | number | null>(null);
  const onFocusChangeRef = useRef(onFocusChange);
  onFocusChangeRef.current = onFocusChange;
  const onNearEndRef = useRef(onNearEnd);
  onNearEndRef.current = onNearEnd;
  const focusedRowRef = useRef(focusedRow);
  const focusedColRef = useRef(focusedCol);
  const nearEndSignalRef = useRef<number | null>(null);
  const scrollNearEndSignalRef = useRef<number | null>(null);
  const scrollNearEndTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const hasNearEndInteractionRef = useRef(false);

  const isCheckboxMode = selectionMode === 'checkbox';
  const isMultiSelect = multiSelect || isCheckboxMode;

  const rowCount = items.length;
  const columnCount = columns.length;

  // Cria função estável para focar a primeira célula
  const focusFirstCell = useCallback(() => {
    // Sempre pega as células disponíveis no momento da chamada
    const cellKey = '0-0';
    const cellElement = cellRefs.current.get(cellKey);
    if (cellElement) {
      cellElement.focus();
    }
  }, []); // cellRefs é uma ref e não muda

  // Fornece a função ao parent - apenas uma vez no mount
  useEffect(() => {
    if (onGridReady) {
      onGridReady(focusFirstCell);
    }
  }, [focusFirstCell, onGridReady]);

  useEffect(() => {
    return () => {
      if (focusTimerRef.current) {
        clearTimeout(focusTimerRef.current);
      }
      if (scrollNearEndTimerRef.current) {
        clearTimeout(scrollNearEndTimerRef.current);
      }
    };
  }, []);

  const markScrollNearEndSignaled = useCallback((itemCount: number) => {
    scrollNearEndSignalRef.current = itemCount;
    if (scrollNearEndTimerRef.current) {
      clearTimeout(scrollNearEndTimerRef.current);
    }
    scrollNearEndTimerRef.current = setTimeout(() => {
      if (scrollNearEndSignalRef.current === itemCount) {
        scrollNearEndSignalRef.current = null;
      }
      if (nearEndSignalRef.current === itemCount) {
        nearEndSignalRef.current = null;
      }
      scrollNearEndTimerRef.current = null;
    }, 1000);
  }, []);

  const announce = (message: string) => {
    // Só anuncia quando o grid (ou algo dentro dele) tem foco.
    // Evita anúncios indesejados quando o componente monta em
    // background (ex: troca de abas com lazy loading).
    if (!gridRef.current?.contains(document.activeElement)) return;

    announceGlobally(message);
  };

  // Foca no grid quando montado (se houver items) - apenas uma vez
  useEffect(() => {
    if (!autoFocusOnMount) {
      return;
    }
    // Aguarda os dados carregarem
    const checkTimer = setInterval(() => {
      if (items.length > 0 && !hasInitializedRef.current) {
        hasInitializedRef.current = true;
        clearInterval(checkTimer);
        
        // Pequeno delay para garantir que o grid foi renderizado
        setTimeout(() => {
          // Só foca se nenhum elemento interativo já está focado.
          // Evita roubar foco de tab buttons e outros controles
          // quando o grid monta em background (ex: troca de abas).
          const activeElement = document.activeElement;
          const isFocusIdle = !activeElement || activeElement === document.body;
          const isInsideGrid = gridRef.current?.contains(activeElement);
          
          if (isFocusIdle || isInsideGrid) {
            // Ativa o foco lazy e foca a primeira célula
            activateFocus(0, 0);
          }
        }, 100);
      }
    }, 100);
    
    return () => {
      clearInterval(checkTimer);
    };
  }, [autoFocusOnMount, items.length, focusFirstCell]);

  // Sincroniza selectedIds externo com estado local
  useEffect(() => {
    if (selectedIds) {
      setLocalSelectedIds(new Set(selectedIds));
    }
  }, [selectedIds ? Array.from(selectedIds).join(',') : '']);

  // Mantém refs de focusedRow/Col atualizadas
  useEffect(() => {
    focusedRowRef.current = focusedRow;
    focusedColRef.current = focusedCol;
  }, [focusedRow, focusedCol]);

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

  const clearScheduledCellFocus = useCallback(() => {
    if (focusTimerRef.current) {
      clearTimeout(focusTimerRef.current);
      focusTimerRef.current = null;
    }
  }, []);

  const scheduleFocusCell = useCallback((row: number, col: number) => {
    clearScheduledCellFocus();
    focusTimerRef.current = setTimeout(() => {
      focusTimerRef.current = null;
      focusCell(row, col);
    }, 0);
  }, [clearScheduledCellFocus, focusCell]);

  const focusCurrentCell = useCallback(() => {
    focusCell(focusedRowRef.current, focusedColRef.current);
  }, [focusCell]);

  // Ativa o foco lazy: marca o grid como "já recebeu foco" e
  // posiciona focusedRow/Col. Chamada no primeiro foco real do usuário.
  const activateFocus = useCallback((row: number, col: number) => {
    hasReceivedFocusRef.current = true;
    hasNearEndInteractionRef.current = true;
    setFocusedRow(row);
    setFocusedCol(col);
    focusedRowRef.current = row;
    focusedColRef.current = col;
    scheduleFocusCell(row, col);
  }, [scheduleFocusCell]);

  // Rastreia qual item está focado por ID e notifica o pai.
  // Só notifica quando o ID do item focado realmente mudou, para evitar
  // loops infinitos quando `items` é recriado com mesmos dados.
  // Não notifica enquanto o grid não recebeu foco (focusedRow === -1).
  useEffect(() => {
    if (focusedRow < 0) return; // Foco lazy: ainda não ativado
    if (items.length > 0 && focusedRow < items.length) {
      const newId = getItemId(items[focusedRow]);
      if (newId !== focusedItemIdRef.current) {
        focusedItemIdRef.current = newId;
        onFocusChangeRef.current?.(items[focusedRow], focusedRow);
      }
      if (onNearEndRef.current && items.length - focusedRow <= nearEndThreshold) {
        if (nearEndSignalRef.current !== items.length) {
          nearEndSignalRef.current = items.length;
          markScrollNearEndSignaled(items.length);
          onNearEndRef.current?.();
        }
      } else {
        nearEndSignalRef.current = null;
      }
    } else if (items.length === 0 && focusedItemIdRef.current !== null) {
      focusedItemIdRef.current = null;
      onFocusChangeRef.current?.(null, -1);
    }
  }, [focusedRow, items, getItemId, nearEndThreshold, markScrollNearEndSignaled]);

  const handleBodyScroll = useCallback((event: React.UIEvent<HTMLDivElement>) => {
    if (!onNearEndRef.current) return;
    hasNearEndInteractionRef.current = true;
    const target = event.currentTarget;
    const remaining = target.scrollHeight - target.scrollTop - target.clientHeight;
    if (remaining <= 160) {
      if (scrollNearEndSignalRef.current === items.length || nearEndSignalRef.current === items.length) return;
      markScrollNearEndSignaled(items.length);
      nearEndSignalRef.current = items.length;
      onNearEndRef.current();
    } else {
      scrollNearEndSignalRef.current = null;
    }
  }, [items.length, markScrollNearEndSignaled]);

  useEffect(() => {
    if (!onNearEndRef.current || !bodyRef.current || items.length === 0 || !hasNearEndInteractionRef.current) return;
    const target = bodyRef.current;
    const remaining = target.scrollHeight - target.scrollTop - target.clientHeight;
    if (remaining <= 160) {
      if (scrollNearEndSignalRef.current === items.length || nearEndSignalRef.current === items.length) return;
      markScrollNearEndSignaled(items.length);
      nearEndSignalRef.current = items.length;
      onNearEndRef.current();
    } else {
      scrollNearEndSignalRef.current = null;
    }
  }, [items.length, markScrollNearEndSignaled]);

  // Segue o item quando a lista é reordenada
  useEffect(() => {
    if (focusedRow < 0) return; // Foco lazy: não rastrear antes de ativar
    if (focusedItemIdRef.current === null || items.length === 0) return;

    const row = focusedRowRef.current;
    const currentItem = row >= 0 && row < items.length ? items[row] : null;
    if (currentItem && getItemId(currentItem) === focusedItemIdRef.current) return;

    const newIndex = items.findIndex(item => getItemId(item) === focusedItemIdRef.current);
    if (newIndex >= 0 && newIndex !== row) {
      setFocusedRow(newIndex);
      scheduleFocusCell(newIndex, focusedColRef.current);
    }
  }, [items, getItemId, scheduleFocusCell]);

  // Foca no input ao editar
  useEffect(() => {
    if (editingRow >= 0 && editInputRef.current) {
      editInputRef.current.focus();
      editInputRef.current.select();
    }
  }, [editingRow, editingCol]);

  const {
    menu: contextMenu,
    openForTrigger: openContextMenuForTrigger,
    openAtPoint: openContextMenuAtPoint,
    closeMenu: closeContextMenu,
    onSelectItem: onContextMenuSelectItem,
  } = useAnchoredContextMenu({
    onAfterSelect: focusCurrentCell,
    onAfterDismiss: focusCurrentCell,
    restoreTriggerFocusOnDismiss: false,
    restoreTriggerFocusOnSelect: false,
  });

  // Handler de clique direito na linha
  const normalizeRowActions = useCallback((actions: MenuItem[]): MenuItem[] => {
    return actions.map((action) => ({
      ...action,
      action: action.action ?? (action as MenuItem & { onClick?: () => void }).onClick,
      submenu: action.submenu ? normalizeRowActions(action.submenu) : undefined,
    }));
  }, []);

  const handleRowContextMenu = (item: T, rowIndex: number) => (event: React.MouseEvent) => {
    if (!getRowActions) return;
    event.preventDefault();
    event.stopPropagation();
    const rawActions = getRowActions(item);
    const actions = rawActions ? normalizeRowActions(rawActions) : rawActions;
    if (!actions || actions.length === 0) return;
    const cellKey = `${rowIndex}-${focusedColRef.current}`;
    const cellElement = cellRefs.current.get(cellKey);
    openContextMenuAtPoint(
      event.clientX,
      event.clientY,
      label,
      actions,
      cellElement ?? (event.currentTarget as HTMLElement)
    );
  };

  const toggleSelection = (rowIndex: number) => {
    const item = items[rowIndex];
    if (!item) return;

    if (onItemToggle) {
      onItemToggle(item, rowIndex);
      return;
    }
    const itemId = getItemId(item);
    const newSelected = new Set(localSelectedIds);
    
    if (newSelected.has(itemId)) {
      newSelected.delete(itemId);
      announce(t('a11y.announce.gridItemDeselected', { count: newSelected.size }));
    } else {
      newSelected.add(itemId);
      announce(t('a11y.announce.gridItemSelected', { count: newSelected.size }));
    }
    
    setLocalSelectedIds(newSelected);
    onSelectionChange?.(newSelected);
  };

  const selectAll = () => {
    const allIds = new Set(items.map(item => getItemId(item)));
    setLocalSelectedIds(allIds);
    onSelectionChange?.(allIds);
    announce(t('a11y.announce.gridAllSelected', { count: allIds.size }));
  };

  const clearSelection = () => {
    setLocalSelectedIds(new Set());
    onSelectionChange?.(new Set());
    announce(t('a11y.announce.gridSelectionCleared'));
  };

  const startEditing = (rowIndex: number, colIndex: number) => {
    const col = columns[colIndex];
    if (col.action || !col.editable) return;

    clearScheduledCellFocus();
    const item = items[rowIndex];
    const value = item[col.key as keyof T];
    setEditingRow(rowIndex);
    setEditingCol(colIndex);
    setEditValue(String(value || ''));
    announce(t('a11y.announce.gridEditing'));
  };

  const saveEdit = () => {
    if (editingRow >= 0 && editingCol >= 0) {
      const item = items[editingRow];
      const column = columns[editingCol];
      onCellEdit?.(item, column, editValue, editingRow, editingCol);
      announce(t('a11y.announce.gridSaved'));
    }
    cancelEdit();
  };

  const cancelEdit = () => {
    if (editingRow >= 0) {
      announce(t('a11y.announce.gridEditCancelled'));
    }
    setEditingRow(-1);
    setEditingCol(-1);
    setEditValue('');
    gridRef.current?.focus();
  };

  // Handler de foco no container do grid.
  // Quando o grid recebe foco diretamente (tabIndex=0 no container),
  // ativa o foco lazy e move para a primeira célula.
  const handleGridFocus = useCallback((event: React.FocusEvent<HTMLDivElement>) => {
    // Se o foco veio de dentro do grid (célula → célula), ignora
    if (gridRef.current?.contains(event.relatedTarget as Node)) return;
    // Se já tem uma célula ativa, apenas garantir que o foco vai pra ela
    if (hasReceivedFocusRef.current && focusedRowRef.current >= 0) {
      const cellKey = `${focusedRowRef.current}-${focusedColRef.current}`;
      const cellElement = cellRefs.current.get(cellKey);
      if (cellElement && event.target === gridRef.current) {
        cellElement.focus();
      }
      return;
    }
    // Primeiro foco: ativa o foco lazy
    if (items.length > 0 && columns.length > 0) {
      activateFocus(0, 0);
    }
  }, [items.length, columns.length, activateFocus]);

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (rowCount === 0 || columnCount === 0) return;
    // Se o foco lazy não foi ativado, ativa ao pressionar qualquer tecla
    if (focusedRow < 0) {
      activateFocus(0, 0);
      return;
    }

    const keyTarget = event.target as HTMLElement | null;
    if (keyTarget?.closest('.context-menu')) {
      return;
    }

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
        if (isCheckboxMode && columns[focusedCol]?.selectionToggle !== false) {
          toggleSelection(focusedRow);
        } else if (event.ctrlKey && isMultiSelect) {
          toggleSelection(focusedRow);
        } else {
          const col = columns[focusedCol];
          if (col.action) {
            onCellAction?.(items[focusedRow], col, focusedRow, focusedCol);
          }
        }
        return;

      case 'Enter': {
        event.preventDefault();
        const colEnter = columns[focusedCol];
        if (colEnter.key === 'actions' && getRowActions) {
          const rawActions = getRowActions(items[focusedRow]);
          const actions = rawActions ? normalizeRowActions(rawActions) : rawActions;
          if (actions && actions.length > 0) {
            const cellKey = `${focusedRow}-${focusedCol}`;
            const cellElement = cellRefs.current.get(cellKey);
            if (cellElement) {
              openContextMenuForTrigger(cellElement, colEnter.actionLabel || label, actions);
            }
          }
          return;
        }
        if (colEnter.action) {
          // Se for célula de ação, executa a ação
          onCellAction?.(items[focusedRow], colEnter, focusedRow, focusedCol);
        } else {
          // Se não for ação, ativa o item (abre/edita)
          onActivate?.(items[focusedRow], focusedRow);
        }
        return;
      }

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
        if (isMultiSelect) {
          clearSelection();
        }
        return;

      case 'a':
        if (event.ctrlKey && isMultiSelect) {
          event.preventDefault();
          selectAll();
          return;
        }
        break;

      case 'c':
        if (event.ctrlKey && !event.altKey) {
          event.preventDefault();
          // Ctrl+C: copiar célula atual
          // Ctrl+Shift+C: copiar todas as células selecionadas
          if (event.shiftKey && isMultiSelect && localSelectedIds.size > 0) {
            // Copiar todas as linhas selecionadas
            const selectedItems = items.filter(item => localSelectedIds.has(getItemId(item)));
            const textToCopy = selectedItems.map(item => {
              return columns.map(col => {
                const value = item[col.key as keyof T];
                return col.format ? String(col.format(value, item)) : String(value || '');
              }).join('\t');
            }).join('\n');
            navigator.clipboard.writeText(textToCopy);
            announce(t('a11y.announce.gridRowsCopied', { count: selectedItems.length }));
          } else {
            // Copiar apenas a célula focada
            const item = items[focusedRow];
            const col = columns[focusedCol];
            const value = item[col.key as keyof T];
            const textToCopy = col.format ? String(col.format(value, item)) : String(value || '');
            navigator.clipboard.writeText(textToCopy);
            announce(t('a11y.announce.gridCellCopied'));
          }
          return;
        }
        break;

      case 'ArrowUp':
        event.preventDefault();
        if (event.altKey && onMoveItem) {
          if (focusedRow > 0) {
            onMoveItem(focusedRow, focusedRow - 1);
            const movedRow = focusedRow - 1;
            setFocusedRow(movedRow);
            scheduleFocusCell(movedRow, focusedCol);
            announce(t('a11y.announce.gridMovedUp'));
          } else {
            playBumpSound();
          }
          return;
        }
        if (focusedRow === 0) {
          playBumpSound();
          return;
        }
        newRow = Math.max(0, focusedRow - 1);
        handled = true;
        break;

      case 'ArrowDown':
        event.preventDefault();
        if (event.altKey && onMoveItem) {
          if (focusedRow < rowCount - 1) {
            onMoveItem(focusedRow, focusedRow + 1);
            const movedRow = focusedRow + 1;
            setFocusedRow(movedRow);
            scheduleFocusCell(movedRow, focusedCol);
            announce(t('a11y.announce.gridMovedDown'));
          } else {
            playBumpSound();
          }
          return;
        }
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

      case 'ContextMenu':
        if (getRowActions) {
          event.preventDefault();
          const rawActions = getRowActions(items[focusedRow]);
          const actions = rawActions ? normalizeRowActions(rawActions) : rawActions;
          if (actions && actions.length > 0) {
            const cellKey = `${focusedRow}-${focusedCol}`;
            const cellElement = cellRefs.current.get(cellKey);
            if (cellElement) {
              openContextMenuForTrigger(cellElement, label, actions);
            }
          }
        }
        return;

      case 'F10':
        if (event.shiftKey && getRowActions) {
          event.preventDefault();
          const rawActions = getRowActions(items[focusedRow]);
          const actions = rawActions ? normalizeRowActions(rawActions) : rawActions;
          if (actions && actions.length > 0) {
            const cellKey = `${focusedRow}-${focusedCol}`;
            const cellElement = cellRefs.current.get(cellKey);
            if (cellElement) {
              openContextMenuForTrigger(cellElement, label, actions);
            }
          }
        }
        return;
    }

    if (handled && (newRow !== focusedRow || newCol !== focusedCol)) {
      const oldRow = focusedRow;
      setFocusedRow(newRow);
      setFocusedCol(newCol);

      // Foca a nova célula após atualizar o estado
      scheduleFocusCell(newRow, newCol);

      // Seleção durante navegação — apenas no modo list
      if (!isCheckboxMode && isMultiSelect && newRow !== oldRow) {
        if (event.shiftKey) {
          const start = Math.min(oldRow, newRow);
          const end = Math.max(oldRow, newRow);
          const newSelected = new Set(localSelectedIds);
          for (let i = start; i <= end; i++) {
            newSelected.add(getItemId(items[i]));
          }
          setLocalSelectedIds(newSelected);
          onSelectionChange?.(newSelected);
        } else if (!event.ctrlKey) {
          const itemId = getItemId(items[newRow]);
          const newSelected = new Set([itemId]);
          setLocalSelectedIds(newSelected);
          onSelectionChange?.(newSelected);
        }
      }
    }
  };

  const handleCellClick = (rowIndex: number, colIndex: number, event: React.MouseEvent) => {
    const target = event.target as HTMLElement | null;
    if (target?.closest('.menu-wrapper') || target?.closest('.menu-toggle')) {
      hasReceivedFocusRef.current = true;
      setFocusedRow(rowIndex);
      setFocusedCol(colIndex);
      scheduleFocusCell(rowIndex, colIndex);
      return;
    }
    const col = columns[colIndex];
    
    // Ativa o foco lazy ao clicar (primeiro contato do usuário)
    hasReceivedFocusRef.current = true;
    setFocusedRow(rowIndex);
    setFocusedCol(colIndex);
    
    // Se for célula de ação, executa a ação e para a propagação
    if (col.action) {
      event.preventDefault();
      event.stopPropagation();
      onCellAction?.(items[rowIndex], col, rowIndex, colIndex);
      // Foca a célula clicada após a ação
      scheduleFocusCell(rowIndex, colIndex);
      return;
    }

    // Foca a célula clicada
    scheduleFocusCell(rowIndex, colIndex);

    if (isCheckboxMode && col.selectionToggle !== false) {
      toggleSelection(rowIndex);
    } else if (event.detail === 2) {
      startEditing(rowIndex, colIndex);
    } else if (isMultiSelect && event.ctrlKey) {
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
          tabIndex={-1}  // Não deve ser tabstop - a célula já é focável e recebe Enter/Space
          onClick={(e) => {
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
      <div className="datagrid-empty">
        {t('common.emptyState', 'Nenhum item para exibir')}
      </div>
    );
  }

  const gridAriaRowCount = showHeader ? rowCount + 1 : rowCount;

  return (
    <>
      <div
        ref={gridRef}
        className={`datagrid-container${isCheckboxMode ? ' datagrid-container--checkbox' : ''}${className ? ` ${className}` : ''}`}
        role="grid"
        aria-label={label}
        aria-rowcount={gridAriaRowCount}
        aria-colcount={columnCount}
        aria-describedby={instructionsId}
        tabIndex={focusedRow < 0 ? 0 : -1}
        onFocus={handleGridFocus}
        onKeyDown={handleKeyDown}
        onClick={() => {
          if (items.length > 0 && columns.length > 0) {
            if (focusedRow < 0) {
              activateFocus(0, 0);
            } else {
              focusCell(focusedRow, focusedCol);
            }
          }
        }}
      >
      <div id={instructionsId} className="sr-only">
        Grade de dados com {rowCount} linhas e {columnCount} colunas.
        Use as setas verticais para navegar entre linhas.
        Use as setas horizontais para navegar entre colunas.
        Pressione Enter para ativar um item.
        {isCheckboxMode
          ? 'Pressione Espaço para marcar ou desmarcar. '
          : 'Pressione Ctrl+Espaço para marcar ou desmarcar. '}
        {isMultiSelect && 'Pressione Ctrl+A para selecionar todos. '}
        {onMoveItem && 'Pressione Alt+Seta para mover o item. '}
        Pressione Delete para remover.
        Pressione F2 para editar.
        Pressione Escape para limpar a seleção.
      </div>
      {showHeader && (
        <div className="datagrid-header" role="row" aria-rowindex={1}>
          {columns.map((column, colIndex) => (
            <div
              key={column.key}
              className="datagrid-header-cell"
              role="columnheader"
              style={{ width: column.width }}
              aria-colindex={colIndex + 1}
              tabIndex={-1}
            >
              {column.label}
            </div>
          ))}
        </div>
      )}
      
      <div ref={bodyRef} className="datagrid-body" onScroll={handleBodyScroll}>
        {items.map((item, rowIndex) => {
          const itemId = getItemId(item);
          const isSelected = localSelectedIds.has(itemId);
          const isFocused = focusedRow >= 0 && rowIndex === focusedRow;

          return (
            <div
              key={itemId}
              className={`datagrid-row ${isSelected ? 'selected' : ''} ${isFocused ? 'focused' : ''}`}
              role="row"
              aria-rowindex={showHeader ? rowIndex + 2 : rowIndex + 1}
              aria-selected={isSelected}
              onContextMenu={getRowActions ? handleRowContextMenu(item, rowIndex) : undefined}
            >
              {columns.map((column, colIndex) => {
                const isCellFocused = focusedRow >= 0 && rowIndex === focusedRow && colIndex === focusedCol;
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
                    aria-label={column.actionLabel || undefined}
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
          {/* Renderiza o ContextMenu global do grid */}
          <ContextMenu
            items={contextMenu.items}
            x={contextMenu.x}
            y={contextMenu.y}
            visible={contextMenu.visible}
            ariaLabel={contextMenu.ariaLabel}
            onClose={closeContextMenu}
            onSelect={onContextMenuSelectItem}
          />
      </div>
    </div>
    </>
  );
}
