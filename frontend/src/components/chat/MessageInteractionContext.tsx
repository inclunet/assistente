import { createContext, useCallback, useContext, useEffect, useRef, type ReactNode } from 'react';

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

/** Leitura permanece proprietária mesmo quando um diálogo filho recebe foco. */
export function useReadingInteraction(active: boolean) {
  const retain = useMessageInteraction();
  useEffect(() => active ? retain() : undefined, [active, retain]);
}
