import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useRef, type ReactNode } from 'react';

export type RetainMessageInteraction = () => () => void;
const noop = () => {};
const Context = createContext<RetainMessageInteraction>(() => noop);

export const useMessageInteraction = () => useContext(Context);

export function MessageInteractionProvider({ itemKey, retain, children }: {
  itemKey: string;
  retain: (key: string) => () => void;
  children: ReactNode;
}) {
  const releases = useRef(new Set<() => void>());
  const acquire = useCallback(() => {
    const release = retain(itemKey);
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
  return <Context.Provider value={acquire}>{children}</Context.Provider>;
}

/** Aquisição no commit cobre a transferência do menu antes do autofocus tardio. */
export function useActiveMessageInteraction(active: boolean) {
  const retain = useMessageInteraction();
  useLayoutEffect(() => active ? retain() : undefined, [active, retain]);
}
