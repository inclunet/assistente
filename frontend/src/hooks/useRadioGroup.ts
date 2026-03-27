import { useCallback, useEffect, useRef } from 'react';
import { playBumpSound } from '../services/audioFeedback';

/**
 * WAI-ARIA Radio Group pattern with roving tabindex.
 *
 * - Arrow keys move focus AND select the item (fires onChange).
 * - Home / End jump to first / last item.
 * - Only the selected item has tabindex="0"; others get tabindex="-1".
 * - The group is a single Tab stop.
 * - Wrapping is optional (default: true). When disabled, bump sound plays at edges.
 */
export interface UseRadioGroupOptions<T> {
  items: readonly T[];
  selectedId: T;
  onChange: (id: T) => void;
  wrap?: boolean;
}

export function useRadioGroup<T extends string>({
  items,
  selectedId,
  onChange,
  wrap = true,
}: UseRadioGroupOptions<T>) {
  const containerRef = useRef<HTMLDivElement>(null);

  const getRadioElements = useCallback((): HTMLElement[] => {
    if (!containerRef.current) return [];
    return Array.from(
      containerRef.current.querySelectorAll<HTMLElement>('[role="radio"]'),
    );
  }, []);

  useEffect(() => {
    const radios = getRadioElements();
    radios.forEach((el) => {
      const isSelected = el.getAttribute('aria-checked') === 'true';
      el.setAttribute('tabindex', isSelected ? '0' : '-1');
    });
  });

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      const radios = getRadioElements();
      if (radios.length === 0) return;

      const target = e.target as HTMLElement;
      if (target.getAttribute('role') !== 'radio') return;

      const currentIdx = radios.indexOf(target);
      if (currentIdx < 0) return;

      let nextIdx: number | null = null;

      switch (e.key) {
        case 'ArrowRight':
        case 'ArrowDown':
          if (currentIdx === radios.length - 1) {
            nextIdx = wrap ? 0 : null;
          } else {
            nextIdx = currentIdx + 1;
          }
          break;

        case 'ArrowLeft':
        case 'ArrowUp':
          if (currentIdx === 0) {
            nextIdx = wrap ? radios.length - 1 : null;
          } else {
            nextIdx = currentIdx - 1;
          }
          break;

        case 'Home':
          nextIdx = currentIdx === 0 ? null : 0;
          break;

        case 'End':
          nextIdx = currentIdx === radios.length - 1 ? null : radios.length - 1;
          break;

        case ' ':
          e.preventDefault();
          radios[currentIdx].click();
          return;

        default:
          return;
      }

      e.preventDefault();
      e.stopPropagation();

      if (nextIdx === null) {
        playBumpSound();
        return;
      }

      const nextEl = radios[nextIdx];
      nextEl.focus();
      nextEl.click();
    };

    const handleFocusIn = (e: FocusEvent) => {
      const target = e.target as HTMLElement;
      if (target.getAttribute('role') !== 'radio') return;

      const radios = getRadioElements();
      radios.forEach((el) => {
        el.setAttribute('tabindex', el === target ? '0' : '-1');
      });
    };

    container.addEventListener('keydown', handleKeyDown);
    container.addEventListener('focusin', handleFocusIn);
    return () => {
      container.removeEventListener('keydown', handleKeyDown);
      container.removeEventListener('focusin', handleFocusIn);
    };
  }, [items, selectedId, onChange, wrap, getRadioElements]);

  return containerRef;
}
