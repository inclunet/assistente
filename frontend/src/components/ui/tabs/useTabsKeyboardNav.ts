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
        onValueChange?.(nextValue);

        // Focus the target tab synchronously, then guard against content
        // components that steal focus during mount/state-change effects.
        const targetBtn = tabs[result.nextIndex];
        targetBtn?.focus();

        let guardActive = true;
        const focusGuard = (e: Event) => {
          if (!guardActive) return;
          const el = e.target as HTMLElement;
          if (el === targetBtn || el?.closest('[role="tablist"]')) return;
          targetBtn?.focus();
        };
        document.addEventListener('focusin', focusGuard, true);
        setTimeout(() => {
          guardActive = false;
          document.removeEventListener('focusin', focusGuard, true);
        }, 350);

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
