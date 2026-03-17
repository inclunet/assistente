import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useLandmarkNavigation, type Landmark } from './useLandmarkNavigation';

export interface UseGridPageLandmarksOptions {
  /** CSS class of the page root, used to scope queries (e.g. 'history-page') */
  pageClass: string;
  /** Extra landmarks inserted after toolbar and before the grid. */
  extraLandmarks?: Landmark[];
}

/**
 * Ready-made F6 landmark navigation for the standard Toolbar + DataGrid pages.
 *
 * Zones: toolbar → (extra) → grid.
 */
export function useGridPageLandmarks({ pageClass, extraLandmarks }: UseGridPageLandmarksOptions) {
  const { t } = useTranslation();

  const landmarks = useMemo<Landmark[]>(() => {
    const scope = () => document.querySelector(`.${pageClass}`) as HTMLElement | null;

    const base: Landmark[] = [
      {
        id: 'toolbar',
        label: t('landmarks.toolbar'),
        focus: () => {
          const page = scope();
          if (!page) return false;
          const toolbar = page.querySelector('[role="toolbar"]') as Element | null;
          if (!toolbar) return false;
          const btn = toolbar.querySelector('button:not([disabled])') as HTMLButtonElement | null;
          if (!btn) return false;
          btn.focus();
          return true;
        },
        contains: () => !!document.activeElement?.closest?.(`${`.${pageClass}`} [role="toolbar"]`),
      },
    ];

    if (extraLandmarks) {
      base.push(...extraLandmarks);
    }

    base.push({
      id: 'grid',
      label: t('landmarks.grid'),
      focus: () => {
        const page = scope();
        if (!page) return false;
        const cell = page.querySelector('.datagrid-container [role="gridcell"][tabindex="0"], .datagrid-container [role="gridcell"]') as HTMLElement | null;
        if (cell) { cell.focus(); return true; }
        const grid = page.querySelector('[role="grid"]') as HTMLElement | null;
        if (grid) { grid.focus(); return true; }
        return false;
      },
      contains: () => !!document.activeElement?.closest?.('.datagrid-container'),
    });

    return base;
  }, [pageClass, extraLandmarks, t]);

  useLandmarkNavigation({ landmarks, defaultLandmarkId: 'grid' });
}
