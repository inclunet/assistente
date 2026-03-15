import { useRef, useState, useCallback, useEffect } from 'react';
import { playBumpSound } from '../services/audioFeedback';

/**
 * Roving tabindex para listas de checkboxes.
 * Apenas um checkbox recebe tabIndex=0 por vez;
 * setas (↑↓) navegam, Space/Enter toggle (nativo do checkbox).
 *
 * Retorna um callback ref — funciona mesmo com renderização condicional.
 */
export function useCheckboxListNav() {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const focusedIndexRef = useRef(0);
  const [, forceUpdate] = useState(0);
  type CheckboxListNode = HTMLDivElement & { __cbListObserver?: MutationObserver };

  const getCheckboxes = (): HTMLInputElement[] => {
    if (!containerRef.current) return [];
    return Array.from(
      containerRef.current.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')
    );
  };

  const syncTabIndexes = useCallback((activeIdx: number) => {
    const cbs = getCheckboxes();
    if (cbs.length === 0) return;
    const idx = Math.min(activeIdx, cbs.length - 1);
    cbs.forEach((cb, i) => {
      cb.setAttribute('tabindex', i === idx ? '0' : '-1');
    });
    focusedIndexRef.current = idx;
  }, []);

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    const target = e.target as HTMLElement;
    if (!target.matches('input[type="checkbox"]')) return;

    const cbs = getCheckboxes();
    if (cbs.length === 0) return;

    const cur = cbs.indexOf(target as HTMLInputElement);
    if (cur < 0) return;

    let next = cur;

    switch (e.key) {
      case 'ArrowDown':
      case 'ArrowRight':
        e.preventDefault();
        if (cur >= cbs.length - 1) { playBumpSound(); return; }
        next = cur + 1;
        break;
      case 'ArrowUp':
      case 'ArrowLeft':
        e.preventDefault();
        if (cur <= 0) { playBumpSound(); return; }
        next = cur - 1;
        break;
      case 'Home':
        e.preventDefault();
        if (cur === 0) { playBumpSound(); return; }
        next = 0;
        break;
      case 'End':
        e.preventDefault();
        if (cur === cbs.length - 1) { playBumpSound(); return; }
        next = cbs.length - 1;
        break;
      default:
        return;
    }

    syncTabIndexes(next);
    cbs[next]?.focus();
  }, [syncTabIndexes]);

  const handleFocusIn = useCallback((e: FocusEvent) => {
    const target = e.target as HTMLElement;
    if (!target.matches('input[type="checkbox"]')) return;
    const cbs = getCheckboxes();
    const idx = cbs.indexOf(target as HTMLInputElement);
    if (idx >= 0) {
      syncTabIndexes(idx);
    }
  }, [syncTabIndexes]);

  const callbackRef = useCallback((node: HTMLDivElement | null) => {
    const prev = containerRef.current;
    if (prev) {
      prev.removeEventListener('keydown', handleKeyDown);
      prev.removeEventListener('focusin', handleFocusIn);
    }

    containerRef.current = node;

    if (node) {
      syncTabIndexes(focusedIndexRef.current);
      node.addEventListener('keydown', handleKeyDown);
      node.addEventListener('focusin', handleFocusIn);

      const observer = new MutationObserver(() => {
        syncTabIndexes(focusedIndexRef.current);
      });
      observer.observe(node, { childList: true, subtree: true });

      (node as CheckboxListNode).__cbListObserver = observer;
    }

    if (prev && !node) {
      const obs = (prev as CheckboxListNode).__cbListObserver;
      obs?.disconnect();
    }

    forceUpdate((c) => c + 1);
  }, [handleKeyDown, handleFocusIn, syncTabIndexes]);

  useEffect(() => {
    return () => {
      const node = containerRef.current;
      if (node) {
        node.removeEventListener('keydown', handleKeyDown);
        node.removeEventListener('focusin', handleFocusIn);
        const obs = (node as CheckboxListNode).__cbListObserver;
        obs?.disconnect();
      }
    };
  }, [handleKeyDown, handleFocusIn]);

  return callbackRef;
}
