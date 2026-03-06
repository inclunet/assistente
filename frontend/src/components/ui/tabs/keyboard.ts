export type TabsNavKey =
  | 'ArrowLeft'
  | 'ArrowRight'
  | 'ArrowUp'
  | 'ArrowDown'
  | 'Home'
  | 'End'
  | 'PageUp'
  | 'PageDown'
  | 'Delete';

export interface TabsNavInput {
  key: string;
  currentIndex: number;
  count: number;
  pageJump?: number;
}

export interface TabsNavResult {
  handled: boolean;
  nextIndex: number;
  bump: boolean;
  action?: 'close';
}

export function getTabsNavResult({
  key,
  currentIndex,
  count,
  pageJump = 10,
}: TabsNavInput): TabsNavResult {
  if (count <= 0) {
    return { handled: false, nextIndex: -1, bump: false };
  }

  const safeCurrent = Math.min(Math.max(currentIndex, 0), count - 1);

  const bumpResult = (action?: TabsNavResult['action']): TabsNavResult => ({
    handled: true,
    nextIndex: safeCurrent,
    bump: true,
    action,
  });

  switch (key as TabsNavKey) {
    case 'ArrowLeft':
    case 'ArrowUp': {
      if (safeCurrent === 0) return bumpResult();
      return { handled: true, nextIndex: safeCurrent - 1, bump: false };
    }

    case 'ArrowRight':
    case 'ArrowDown': {
      if (safeCurrent === count - 1) return bumpResult();
      return { handled: true, nextIndex: safeCurrent + 1, bump: false };
    }

    case 'Home': {
      if (safeCurrent === 0) return bumpResult();
      return { handled: true, nextIndex: 0, bump: false };
    }

    case 'End': {
      if (safeCurrent === count - 1) return bumpResult();
      return { handled: true, nextIndex: count - 1, bump: false };
    }

    case 'PageUp': {
      if (safeCurrent === 0) return bumpResult();
      return {
        handled: true,
        nextIndex: Math.max(0, safeCurrent - pageJump),
        bump: false,
      };
    }

    case 'PageDown': {
      if (safeCurrent === count - 1) return bumpResult();
      return {
        handled: true,
        nextIndex: Math.min(count - 1, safeCurrent + pageJump),
        bump: false,
      };
    }

    case 'Delete': {
      return { handled: true, nextIndex: safeCurrent, bump: false, action: 'close' };
    }

    default:
      return { handled: false, nextIndex: safeCurrent, bump: false };
  }
}

export function isTabsNavKey(key: string): key is TabsNavKey {
  return (
    key === 'ArrowLeft' ||
    key === 'ArrowRight' ||
    key === 'ArrowUp' ||
    key === 'ArrowDown' ||
    key === 'Home' ||
    key === 'End' ||
    key === 'PageUp' ||
    key === 'PageDown' ||
    key === 'Delete'
  );
}
