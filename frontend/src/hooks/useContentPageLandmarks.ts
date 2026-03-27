import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useLandmarkNavigation, useHasParentLandmarks, type Landmark } from './useLandmarkNavigation';

export interface UseContentPageLandmarksOptions {
  /** CSS class of the page root, used to scope queries (e.g. 'about-page') */
  pageClass: string;
  /** Extra landmarks inserted between topbar and page content. */
  extraLandmarks?: Landmark[];
}

/**
 * F6 landmark navigation for content pages without Toolbar/DataGrid.
 *
 * Zones: topbar → (extra) → pageContent.
 */
export function useContentPageLandmarks({ pageClass, extraLandmarks }: UseContentPageLandmarksOptions) {
  const { t } = useTranslation();
  const parentOwns = useHasParentLandmarks();

  const landmarks = useMemo<Landmark[]>(() => {
    const scope = () => document.querySelector(`.${pageClass}`) as HTMLElement | null;

    const base: Landmark[] = [
      {
        id: 'topbar',
        label: t('landmarks.topbar'),
        focus: () => {
          const topbar = document.querySelector('.topbar') as Element | null;
          if (!topbar) return false;
          const btn = topbar.querySelector('button:not([disabled])') as HTMLButtonElement | null;
          if (!btn) return false;
          btn.focus();
          return true;
        },
        contains: () => !!document.activeElement?.closest?.('.topbar'),
      },
    ];

    if (extraLandmarks) {
      base.push(...extraLandmarks);
    }

    base.push({
      id: 'pageContent',
      label: t('landmarks.pageContent'),
      focus: () => {
        const page = scope();
        if (!page) return false;
        const focusable = page.querySelector(
          'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ) as HTMLElement | null;
        if (focusable) { focusable.focus(); return true; }
        page.setAttribute('tabindex', '-1');
        page.focus();
        return true;
      },
      contains: () => {
        const page = scope();
        return !!page && page.contains(document.activeElement);
      },
    });

    return base;
  }, [pageClass, extraLandmarks, t]);

  useLandmarkNavigation({ landmarks, defaultLandmarkId: 'pageContent', enabled: !parentOwns });
}
