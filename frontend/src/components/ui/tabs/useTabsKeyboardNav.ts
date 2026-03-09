import { RefObject, useCallback } from 'react';
import { getTabsNavResult, isTabsNavKey } from './keyboard';

export type TabsActivationMode = 'auto' | 'manual';

export interface UseTabsKeyboardNavOptions {
  tabListRef: RefObject<HTMLElement | null>;
  activationMode?: TabsActivationMode;
  pageJump?: number;
  onBump?: () => void;
  onValueChange?: (value: string) => void;
  onDelete?: (value: string) => void;
}

function getTabsInList(tabList: HTMLElement): HTMLButtonElement[] {
  return Array.from(
    tabList.querySelectorAll<HTMLButtonElement>('button[role="tab"]:not([disabled])')
  );
}

function getTabValue(tab: HTMLElement): string | null {
  const v = tab.getAttribute('data-tab-value');
  return v && v.trim() ? v : null;
}

export function useTabsKeyboardNav({
  tabListRef,
  activationMode = 'auto',
  pageJump = 10,
  onBump,
  onValueChange,
  onDelete,
}: UseTabsKeyboardNavOptions) {
  const focusTabAtIndex = useCallback(
    (index: number) => {
      const list = tabListRef.current;
      if (!list) return;
      const tabs = getTabsInList(list);
      const tab = tabs[index];
      tab?.focus();
    },
    [tabListRef]
  );

  const onKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      if (!isTabsNavKey(event.key)) return;

      // Delete com modificadores costuma significar outra ação em alguns layouts;
      // não deve fechar abas nesses casos.
      if (
        event.key === 'Delete' &&
        (event.shiftKey || event.ctrlKey || event.metaKey || event.altKey)
      ) {
        return;
      }

      const list = tabListRef.current;
      if (!list) return;

      const tabs = getTabsInList(list);
      if (tabs.length === 0) return;

      const targetEl = event.target as HTMLElement | null;
      const targetTab = targetEl?.closest?.('button[role="tab"]') as HTMLButtonElement | null;

      const currentIndexFromTarget = targetTab ? tabs.indexOf(targetTab) : -1;
      const selectedIndex = tabs.findIndex((t) => t.getAttribute('aria-selected') === 'true');

      const currentIndex =
        currentIndexFromTarget >= 0 ? currentIndexFromTarget : selectedIndex >= 0 ? selectedIndex : 0;

      const result = getTabsNavResult({
        key: event.key,
        currentIndex,
        count: tabs.length,
        pageJump,
      });

      if (!result.handled) return;

      event.preventDefault();

      if (result.bump) {
        onBump?.();
        return;
      }

      if (result.action === 'close') {
        const currentTab = tabs[currentIndex];
        const value = currentTab ? getTabValue(currentTab) : null;
        if (value) onDelete?.(value);
        return;
      }

      if (result.nextIndex === currentIndex) return;

      const nextTab = tabs[result.nextIndex];
      const nextValue = nextTab ? getTabValue(nextTab) : null;

      if (activationMode === 'auto' && nextValue) {
        onValueChange?.(nextValue);
        // Aguarda re-render ajustar tabindex/aria-selected.
        setTimeout(() => focusTabAtIndex(result.nextIndex), 0);
        return;
      }

      focusTabAtIndex(result.nextIndex);
    },
    [
      activationMode,
      focusTabAtIndex,
      onBump,
      onDelete,
      onValueChange,
      pageJump,
      tabListRef,
    ]
  );

  return { onKeyDown };
}
