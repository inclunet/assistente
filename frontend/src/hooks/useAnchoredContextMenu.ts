import { useCallback, useRef, useState } from 'react';
import type { MenuItem } from '../components/menu';

export type MenuCloseReason = 'dismiss' | 'select';

export interface AnchoredContextMenuState {
  visible: boolean;
  x: number;
  y: number;
  items: MenuItem[];
  ariaLabel: string;
}

export interface UseAnchoredContextMenuOptions {
  /**
   * Chamado quando o menu fecha após uma seleção.
   * Use para devolver foco ao editor / input, etc.
   */
  onAfterSelect?: () => void;

  /**
   * Se true (default), ao fechar após seleção o foco volta para o elemento que abriu o menu
   * (padrão ARIA Menu Button). Se `onAfterSelect` for fornecido, ele tem precedência.
   */
  restoreTriggerFocusOnSelect?: boolean;

  /**
   * Chamado quando o menu fecha por dismiss (Escape/clique fora).
   * Se não for fornecido, o hook tenta restaurar foco no gatilho.
   */
  onAfterDismiss?: () => void;

  /**
   * Se true (default), ao fechar por dismiss o foco volta para o elemento que abriu o menu.
   */
  restoreTriggerFocusOnDismiss?: boolean;
}

export interface UseAnchoredContextMenuResult {
  menu: AnchoredContextMenuState;
  triggerElementRef: React.MutableRefObject<HTMLElement | null>;
  openForTrigger: (trigger: HTMLElement, ariaLabel: string, items: MenuItem[]) => void;
  openAtPoint: (x: number, y: number, ariaLabel: string, items: MenuItem[], trigger?: HTMLElement | null) => void;
  closeMenu: () => void;
  onSelectItem: (item: MenuItem) => void;
}

/**
 * Estado/regras comuns de ContextMenu:
 * - Abre ancorado em um elemento (x/y via getBoundingClientRect)
 * - Fecha e restaura foco no gatilho (dismiss)
 * - Permite comportamento customizado após seleção (ex: focar editor)
 */
export function useAnchoredContextMenu(options: UseAnchoredContextMenuOptions = {}): UseAnchoredContextMenuResult {
  const {
    onAfterSelect,
    restoreTriggerFocusOnSelect = true,
    onAfterDismiss,
    restoreTriggerFocusOnDismiss = true,
  } = options;

  const triggerElementRef = useRef<HTMLElement | null>(null);
  const closeReasonRef = useRef<MenuCloseReason>('dismiss');

  const [menu, setMenu] = useState<AnchoredContextMenuState>({
    visible: false,
    x: 0,
    y: 0,
    items: [],
    ariaLabel: 'Menu',
  });

  const openForTrigger = useCallback((trigger: HTMLElement, ariaLabel: string, items: MenuItem[]) => {
    const rect = trigger.getBoundingClientRect();
    triggerElementRef.current = trigger;
    setMenu({
      visible: true,
      x: rect.left,
      y: rect.bottom,
      items,
      ariaLabel,
    });
  }, []);

  const openAtPoint = useCallback(
    (x: number, y: number, ariaLabel: string, items: MenuItem[], trigger?: HTMLElement | null) => {
      triggerElementRef.current = trigger ?? null;
      setMenu({ visible: true, x, y, ariaLabel, items });
    },
    []
  );

  const onSelectItem = useCallback((_item: MenuItem) => {
    // O ContextMenu chama onClose logo após onSelect.
    closeReasonRef.current = 'select';
  }, []);

  const closeMenu = useCallback(() => {
    const reason = closeReasonRef.current;
    closeReasonRef.current = 'dismiss';

    setMenu((prev) => (prev.visible ? { ...prev, visible: false } : prev));

    window.setTimeout(() => {
      try {
        if (reason === 'select') {
          if (onAfterSelect) {
            onAfterSelect();
            return;
          }

          if (restoreTriggerFocusOnSelect) {
            triggerElementRef.current?.focus?.();
          }
          return;
        }

        if (onAfterDismiss) {
          onAfterDismiss();
          return;
        }

        if (restoreTriggerFocusOnDismiss) {
          triggerElementRef.current?.focus?.();
        }
      } catch {
        // best-effort
      }
    }, 10);
  }, [onAfterDismiss, onAfterSelect, restoreTriggerFocusOnDismiss, restoreTriggerFocusOnSelect]);

  return {
    menu,
    triggerElementRef,
    openForTrigger,
    openAtPoint,
    closeMenu,
    onSelectItem,
  };
}
