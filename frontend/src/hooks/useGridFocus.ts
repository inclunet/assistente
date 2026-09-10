import { useCallback, useRef } from 'react';
import type { GridFocusRequest, GridFocusTarget } from '../components/ui/DataGrid';

/**
 * Hook para conectar o DataGrid ao ciclo de vida da página.
 *
 * O DataGrid chama onGridReady com sua função interna de foco; este hook
 * apenas armazena a referência. A restauração de foco é gerenciada pelo
 * sistema de landmarks (useGridPageLandmarks + restoreDefaultFocus).
 *
 * Uso:
 * ```tsx
 * const { handleGridReady } = useGridFocus();
 * useGridPageLandmarks({ pageClass: 'minha-page' });
 *
 * <Toolbar left={<h1>...</h1>} actions={[...]} />
 * <DataGrid onGridReady={handleGridReady} ... />
 * ```
 */
export function useGridFocus() {
  const focusFnRef = useRef<GridFocusRequest | null>(null);

  const handleGridReady = useCallback((fn: GridFocusRequest) => {
    focusFnRef.current = fn;
  }, []);

  const requestGridFocus = useCallback((target?: GridFocusTarget) => {
    return focusFnRef.current?.(target) ?? false;
  }, []);

  return {
    /** Callback para receber a API de foco do DataGrid (passar para DataGrid.onGridReady). */
    handleGridReady,
    /** Restaura por ID/índice e preserva a coluna atual quando ela não é informada. */
    requestGridFocus,
  };
}
