import { useState, useCallback } from 'react';

/**
 * Hook para conectar Toolbar e DataGrid, permitindo que Enter no campo de busca
 * foque a primeira célula do grid.
 * 
 * Uso:
 * ```tsx
 * const { focusFirstCell, handleGridReady } = useGridFocus();
 * 
 * <Toolbar onFocusGrid={focusFirstCell} ... />
 * <DataGrid onGridReady={handleGridReady} ... />
 * ```
 */
export function useGridFocus() {
  const [focusFirstCell, setFocusFirstCell] = useState<(() => void) | null>(null);

  const handleGridReady = useCallback((fn: () => void) => {
    setFocusFirstCell(() => fn);
  }, []);

  return {
    /** Função para focar a primeira célula do grid (passar para Toolbar.onFocusGrid) */
    focusFirstCell,
    /** Callback para receber a função de foco do DataGrid (passar para DataGrid.onGridReady) */
    handleGridReady,
  };
}
