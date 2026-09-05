import { createContext, useContext, useEffect } from 'react';
import { isModalOpen } from '../lib/modalRegistry';

/**
 * Indica se a superfície que contém o componente está ativa.
 * Fora de hosts keep-alive, a superfície é ativa por padrão.
 */
export const ActivePanelContext = createContext(true);

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return (
    target.tagName === 'INPUT' ||
    target.tagName === 'TEXTAREA' ||
    target.tagName === 'SELECT' ||
    target.isContentEditable
  );
}

/**
 * Registra o atalho CRUD Ctrl+N somente para o painel ativo.
 * Painéis preservados por keep-alive continuam montados, mas não respondem.
 */
export function useActivePanelNewShortcut(onNew: () => void): void {
  const isActive = useContext(ActivePanelContext);

  useEffect(() => {
    if (!isActive) return;

    const handleKeyDown = (event: KeyboardEvent) => {
      if (isModalOpen()) return;
      if (!event.ctrlKey || event.shiftKey || event.altKey || event.metaKey) return;
      if (event.key.toLowerCase() !== 'n') return;
      if (isEditableTarget(event.target)) return;

      event.preventDefault();
      onNew();
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [isActive, onNew]);
}
