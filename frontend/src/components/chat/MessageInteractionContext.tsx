import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useRef, type ReactNode } from 'react';

export type RetainMessageInteraction = () => () => void;
const noop = () => {};
const Context = createContext<(itemKey?: string) => () => void>(() => noop);
const ActiveContext = createContext<() => boolean>(() => false);
export const useMessageInteractionsActive = () => useContext(ActiveContext);

export function useMessageInteraction(itemKey?: string): RetainMessageInteraction {
  const acquire = useContext(Context);
  return useCallback(() => acquire(itemKey), [acquire, itemKey]);
}

export function MessageInteractionProvider({ itemKey, retain, children }: {
  itemKey: string;
  retain: (key: string) => () => void;
  children: ReactNode;
}) {
  const releases = useRef(new Set<() => void>());
  const acquire = useCallback((targetKey?: string) => {
    const releaseRoot = retain(itemKey);
    const releaseTarget = targetKey && targetKey !== itemKey ? retain(targetKey) : noop;
    const release = () => { releaseTarget(); releaseRoot(); };
    const dispose = () => {
      if (releases.current.delete(dispose)) release();
    };
    releases.current.add(dispose);
    return dispose;
  }, [itemKey, retain]);
  useEffect(() => {
    const owned = releases.current;
    return () => owned.forEach(release => release());
  }, []);
  const active = useCallback(() => releases.current.size > 0, []);
  return <ActiveContext.Provider value={active}><Context.Provider value={acquire}>{children}</Context.Provider></ActiveContext.Provider>;
}

/** Aquisição no commit cobre a transferência do menu antes do autofocus tardio. */
export function useActiveMessageInteraction(active: boolean, itemKey?: string) {
  const retain = useMessageInteraction(itemKey);
  useLayoutEffect(() => active ? retain() : undefined, [active, retain]);
}
