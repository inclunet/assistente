import { useEffect, useRef } from 'react';
import { isModalOpen, useIsInsideModal, useModalIsTopmost } from '../components/ui/Modal';

/**
 * Ctrl+N cria um item na tela atual (ex.: novo status, nova custom action).
 *
 * Dentro de um `Modal`, só age quando esse modal está no topo da stack — com o
 * modal de edição do item aberto por cima, o atalho não empilha outro. Fora de
 * modal, não age enquanto houver algum modal aberto.
 */
export function useNewItemShortcut(onNew: () => void, enabled = true) {
  const insideModal = useIsInsideModal();
  const isTopmost = useModalIsTopmost();
  const onNewRef = useRef(onNew);
  onNewRef.current = onNew;

  useEffect(() => {
    if (!enabled) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (!e.ctrlKey || e.shiftKey || e.altKey || e.metaKey || e.repeat) return;
      if (e.key !== 'n' && e.key !== 'N') return;
      if (insideModal ? !isTopmost() : isModalOpen()) return;
      e.preventDefault();
      e.stopPropagation();
      onNewRef.current();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [enabled, insideModal, isTopmost]);
}
