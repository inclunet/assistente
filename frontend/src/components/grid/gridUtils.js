/**
 * Grid Utilities - Funções compartilhadas para DataGrid e CardGrid
 * 
 * Este módulo contém a lógica comum de seleção e navegação
 * usada por ambos os componentes de grid.
 */

// ============================================
// SELEÇÃO
// ============================================

/**
 * Cria um gerenciador de seleção para grids
 * 
 * @param {Object} options
 * @param {Function} options.getItemId - Função para obter ID do item
 * @param {Function} options.getItems - Função para obter lista de items
 * @param {Function} options.onSelectionChange - Callback quando seleção muda
 * @returns {Object} Objeto com funções de seleção
 */
export function createSelectionManager({ getItemId, getItems, onSelectionChange }) {
  let selectedIds = new Set();
  let anchorIndex = -1;

  return {
    /**
     * Retorna os IDs selecionados
     */
    getSelectedIds() {
      return selectedIds;
    },

    /**
     * Define os IDs selecionados
     */
    setSelectedIds(ids) {
      selectedIds = ids instanceof Set ? ids : new Set(ids);
      onSelectionChange?.(selectedIds);
    },

    /**
     * Retorna o índice âncora (para seleção com Shift)
     */
    getAnchorIndex() {
      return anchorIndex;
    },

    /**
     * Define o índice âncora
     */
    setAnchorIndex(index) {
      anchorIndex = index;
    },

    /**
     * Seleciona um único item
     */
    selectSingle(index) {
      const items = getItems();
      if (index < 0 || index >= items.length) return;
      
      selectedIds = new Set([getItemId(items[index])]);
      anchorIndex = index;
      onSelectionChange?.(selectedIds);
    },

    /**
     * Seleciona um range de items
     */
    selectRange(fromIndex, toIndex) {
      const items = getItems();
      const start = Math.min(fromIndex, toIndex);
      const end = Math.max(fromIndex, toIndex);
      
      selectedIds = new Set();
      for (let i = start; i <= end; i++) {
        if (i >= 0 && i < items.length) {
          selectedIds.add(getItemId(items[i]));
        }
      }
      onSelectionChange?.(selectedIds);
    },

    /**
     * Alterna a seleção de um item
     */
    toggleSelection(index) {
      const items = getItems();
      if (index < 0 || index >= items.length) return;
      
      const id = getItemId(items[index]);
      if (selectedIds.has(id)) {
        selectedIds.delete(id);
      } else {
        selectedIds.add(id);
      }
      selectedIds = new Set(selectedIds); // Força reatividade
      onSelectionChange?.(selectedIds);
    },

    /**
     * Seleciona todos os items
     */
    selectAll() {
      const items = getItems();
      selectedIds = new Set(items.map(getItemId));
      onSelectionChange?.(selectedIds);
    },

    /**
     * Limpa a seleção
     */
    clearSelection() {
      selectedIds = new Set();
      anchorIndex = -1;
      onSelectionChange?.(selectedIds);
    },

    /**
     * Verifica se um item está selecionado
     */
    isSelected(index) {
      const items = getItems();
      if (index < 0 || index >= items.length) return false;
      return selectedIds.has(getItemId(items[index]));
    },

    /**
     * Processa clique com modificadores (Ctrl/Shift)
     */
    handleClickWithModifiers(index, { ctrlKey, shiftKey }, multiSelect) {
      if (!multiSelect) return;

      if (shiftKey && !ctrlKey) {
        if (anchorIndex === -1) anchorIndex = index;
        this.selectRange(anchorIndex, index);
      } else if (ctrlKey && !shiftKey) {
        this.toggleSelection(index);
      } else {
        this.selectSingle(index);
      }
    },

    /**
     * Processa navegação com modificadores (para seleção durante navegação)
     */
    handleNavigationWithModifiers(newIndex, oldIndex, { ctrlKey, shiftKey }, multiSelect) {
      if (!multiSelect) return;

      if (shiftKey && !ctrlKey) {
        if (anchorIndex === -1) anchorIndex = oldIndex;
        this.selectRange(anchorIndex, newIndex);
      } else if (!ctrlKey && !shiftKey) {
        this.selectSingle(newIndex);
      }
      // Se Ctrl está pressionado, não altera seleção
    }
  };
}

// ============================================
// NAVEGAÇÃO
// ============================================

/**
 * Processa teclas de navegação comuns (Home, End, Escape, etc.)
 * Retorna { handled: boolean, newIndex?: number, action?: string }
 * 
 * @param {KeyboardEvent} event
 * @param {Object} options
 * @param {number} options.currentIndex - Índice atual
 * @param {number} options.itemCount - Total de items
 * @param {boolean} options.multiSelect - Se multi-seleção está ativa
 */
export function handleCommonGridKeys(event, { currentIndex, itemCount, multiSelect }) {
  const { key, ctrlKey } = event;
  
  switch (key) {
    case 'Home':
      return {
        handled: true,
        newIndex: ctrlKey ? 0 : currentIndex // Ctrl+Home = primeiro item
      };
      
    case 'End':
      return {
        handled: true,
        newIndex: ctrlKey ? itemCount - 1 : currentIndex // Ctrl+End = último item
      };
      
    case 'Escape':
      return {
        handled: true,
        action: multiSelect ? 'clearSelection' : 'blur'
      };
      
    case 'a':
      if (ctrlKey && multiSelect) {
        return {
          handled: true,
          action: 'selectAll'
        };
      }
      break;
      
    case 'Delete':
      return {
        handled: true,
        action: 'delete'
      };
      
    case 'Enter':
      return {
        handled: true,
        action: 'activate'
      };
      
    case ' ':
      if (ctrlKey && multiSelect) {
        return {
          handled: true,
          action: 'toggleSelection'
        };
      }
      return {
        handled: true,
        action: 'activate'
      };
  }
  
  return { handled: false };
}

/**
 * Calcula novo índice para navegação linear (1D)
 * Usado pelo CardGrid
 * 
 * @param {string} key - Tecla pressionada
 * @param {number} currentIndex - Índice atual
 * @param {number} itemCount - Total de items
 * @param {number} columnsCount - Número de colunas visíveis
 * @param {boolean} ctrlKey - Se Ctrl está pressionado
 */
export function calculateLinearNavigation(key, currentIndex, itemCount, columnsCount, ctrlKey) {
  let newIndex = currentIndex;
  let handled = false;

  switch (key) {
    case 'ArrowRight':
      if (currentIndex < itemCount - 1) {
        newIndex = currentIndex + 1;
        handled = true;
      }
      break;
      
    case 'ArrowLeft':
      if (currentIndex > 0) {
        newIndex = currentIndex - 1;
        handled = true;
      }
      break;
      
    case 'ArrowDown':
      {
        const targetIndex = currentIndex + columnsCount;
        if (targetIndex < itemCount) {
          newIndex = targetIndex;
        } else if (currentIndex < itemCount - 1) {
          newIndex = itemCount - 1;
        }
        handled = true;
      }
      break;
      
    case 'ArrowUp':
      {
        const targetIndex = currentIndex - columnsCount;
        if (targetIndex >= 0) {
          newIndex = targetIndex;
        } else if (currentIndex > 0) {
          newIndex = 0;
        }
        handled = true;
      }
      break;
      
    case 'Home':
      if (ctrlKey) {
        newIndex = 0;
      } else {
        // Início da linha atual
        newIndex = Math.floor(currentIndex / columnsCount) * columnsCount;
      }
      handled = true;
      break;
      
    case 'End':
      if (ctrlKey) {
        newIndex = itemCount - 1;
      } else {
        // Fim da linha atual
        const rowStart = Math.floor(currentIndex / columnsCount) * columnsCount;
        newIndex = Math.min(rowStart + columnsCount - 1, itemCount - 1);
      }
      handled = true;
      break;
      
    case 'PageDown':
      if (ctrlKey) {
        newIndex = itemCount - 1;
      } else {
        newIndex = Math.min(currentIndex + (columnsCount * 3), itemCount - 1);
      }
      handled = true;
      break;
      
    case 'PageUp':
      if (ctrlKey) {
        newIndex = 0;
      } else {
        newIndex = Math.max(currentIndex - (columnsCount * 3), 0);
      }
      handled = true;
      break;
  }

  return { newIndex, handled };
}

/**
 * Calcula novo índice para navegação 2D (row/col)
 * Usado pelo DataGrid
 * 
 * @param {string} key - Tecla pressionada
 * @param {number} currentRow - Linha atual
 * @param {number} currentCol - Coluna atual
 * @param {number} rowCount - Total de linhas
 * @param {number} colCount - Total de colunas
 * @param {boolean} ctrlKey - Se Ctrl está pressionado
 */
export function calculate2DNavigation(key, currentRow, currentCol, rowCount, colCount, ctrlKey) {
  let newRow = currentRow;
  let newCol = currentCol;
  let handled = false;

  switch (key) {
    case 'ArrowRight':
      if (currentCol < colCount - 1) {
        newCol = currentCol + 1;
        handled = true;
      }
      break;
      
    case 'ArrowLeft':
      if (currentCol > 0) {
        newCol = currentCol - 1;
        handled = true;
      }
      break;
      
    case 'ArrowDown':
      if (currentRow < rowCount - 1) {
        newRow = currentRow + 1;
        handled = true;
      }
      break;
      
    case 'ArrowUp':
      if (currentRow > 0) {
        newRow = currentRow - 1;
        handled = true;
      }
      break;
      
    case 'Home':
      if (ctrlKey) {
        newRow = 0;
        newCol = 0;
      } else {
        newCol = 0;
      }
      handled = true;
      break;
      
    case 'End':
      if (ctrlKey) {
        newRow = rowCount - 1;
        newCol = colCount - 1;
      } else {
        newCol = colCount - 1;
      }
      handled = true;
      break;
      
    case 'PageDown':
      if (ctrlKey) {
        newRow = rowCount - 1;
      } else {
        newRow = Math.min(currentRow + 10, rowCount - 1);
      }
      handled = true;
      break;
      
    case 'PageUp':
      if (ctrlKey) {
        newRow = 0;
      } else {
        newRow = Math.max(currentRow - 10, 0);
      }
      handled = true;
      break;
  }

  return { newRow, newCol, handled };
}

// ============================================
// HELPERS
// ============================================

/**
 * Clamp um valor entre min e max
 */
export function clamp(value, min, max) {
  return Math.min(Math.max(value, min), max);
}

/**
 * Calcula posição no grid baseado em índice linear
 */
export function getGridPosition(index, columnsCount) {
  return {
    row: Math.floor(index / columnsCount) + 1,
    col: (index % columnsCount) + 1
  };
}



