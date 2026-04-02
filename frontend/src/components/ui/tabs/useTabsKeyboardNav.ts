import { RefObject, useCallback } from 'react';
import { getTabsNavResult, isTabsNavKey } from './keyboard';
import { restoreDefaultFocus } from '../../../hooks/useDefaultFocus';

export type TabsActivationMode = 'auto' | 'manual';

export interface UseTabsKeyboardNavOptions {
  tabListRef: RefObject<HTMLElement | null>;
  activationMode?: TabsActivationMode;
  pageJump?: number;
  onBump?: () => void;
  onValueChange?: (value: string) => void;
  onDelete?: (value: string) => void;
  /**
   * Called when Enter is pressed on a tab. If it returns true, the default
   * behavior (restoreDefaultFocus) is suppressed. Use this for example when
   * the tab is being renamed via inline editing.
   */
  onActivate?: () => boolean;
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
  onActivate,
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
      // Ctrl+W / Ctrl+F4: fecha aba e restaura foco na default area
      if ((event.ctrlKey && event.key === 'w') || (event.ctrlKey && event.key === 'F4')) {
        event.preventDefault();
        const list = tabListRef.current;
        if (!list) return;
        const tabs = getTabsInList(list);
        const targetEl = event.target as HTMLElement | null;
        const targetTab = targetEl?.closest?.('button[role="tab"]') as HTMLButtonElement | null;
        const currentTab = targetTab || tabs.find((t) => t.getAttribute('aria-selected') === 'true');
        const value = currentTab ? getTabValue(currentTab) : null;
        if (value) {
          onDelete?.(value);
          requestAnimationFrame(() => restoreDefaultFocus());
        }
        return;
      }

      if (!isTabsNavKey(event.key)) return;

      // Alt+Arrows são reservados para reordenação no nível do consumidor
      if (event.altKey) return;

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

      if (result.action === 'activate') {
        const suppressed = onActivate?.();
        if (!suppressed) {
          restoreDefaultFocus();
        }
        return;
      }

      if (result.nextIndex === currentIndex) return;

      const nextTab = tabs[result.nextIndex];
      const nextValue = nextTab ? getTabValue(nextTab) : null;

      if (activationMode === 'auto' && nextValue) {
        const prevTab = tabs[currentIndex];

        // 1. Disable current tab
        if (prevTab) {
          prevTab.setAttribute('aria-selected', 'false');
          prevTab.tabIndex = -1;
        }

        // 2. Select next tab immediately (before focus)
        nextTab.setAttribute('aria-selected', 'true');
        nextTab.tabIndex = 0;

        // 3. Update state (this triggers re-render but our guards in useLayoutEffect will skip redundant DOM writes)
        onValueChange?.(nextValue);

        // 4. Focus last. This is the crucial event.
        // Screen readers usually announce the element receiving focus. 
        // If the state is already correctly "selected", they announce "Tab Name, Selected, 1 of X".
        nextTab.focus();

        return;
      }

      focusTabAtIndex(result.nextIndex);
    },
    [
      activationMode,
      focusTabAtIndex,
      onActivate,
      onBump,
      onDelete,
      onValueChange,
      pageJump,
      tabListRef,
    ]
  );

  return { onKeyDown };
}
