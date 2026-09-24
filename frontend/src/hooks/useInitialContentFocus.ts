import { useEffect, useRef, type RefObject } from 'react';

/**
 * Dá o foco inicial ao conteúdo principal de uma tela quando ela fica pronta
 * (ex.: o grid de um editor aberto em modal, depois do lazy load e dos dados).
 *
 * Roda uma única vez, depois do paint (double-rAF, como o `Modal`). Só move o
 * foco se ele estiver ocioso (body), dentro da própria tela ou no foco
 * provisório do `Modal` que a hospeda (botão Fechar / container) — nunca
 * rouba foco de fora.
 */
export function useInitialContentFocus(
  rootRef: RefObject<HTMLElement | null>,
  ready: boolean,
  focus: () => void,
) {
  const doneRef = useRef(false);
  const focusRef = useRef(focus);
  focusRef.current = focus;

  useEffect(() => {
    if (!ready || doneRef.current) return;

    let secondRafId: number | undefined;
    const firstRafId = requestAnimationFrame(() => {
      secondRafId = requestAnimationFrame(() => {
        const root = rootRef.current;
        if (!root) return;
        doneRef.current = true;
        const active = document.activeElement;
        const host = root.closest('.modal-content') ?? root;
        if (!active || active === document.body || host.contains(active)) {
          focusRef.current();
        }
      });
    });

    return () => {
      cancelAnimationFrame(firstRafId);
      if (secondRafId !== undefined) cancelAnimationFrame(secondRafId);
    };
  }, [ready, rootRef]);
}
