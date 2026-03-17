import { useCallback, useRef } from 'react';

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
  const focusFnRef = useRef<(() => void) | null>(null);

  const handleGridReady = useCallback((fn: () => void) => {
    focusFnRef.current = fn;
  }, []);

  return {
    /** Callback para receber a função de foco do DataGrid (passar para DataGrid.onGridReady) */
    handleGridReady,
  };
}
