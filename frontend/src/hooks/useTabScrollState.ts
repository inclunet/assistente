import { useEffect, useRef } from 'react';
import { useWorkspaceStore } from '../store/workspaceStore';

/**
 * Preserva e restaura scroll position ao trocar de aba no workspace.
 * - No mount, restaura scrollTop do tab.state.scrollTop
 * - No unmount, salva scrollTop atual no tab.state
 */
export function useTabScrollState(
  scrollRef: React.RefObject<HTMLElement | null>,
  tabId: string,
) {
  const scrollTop = useWorkspaceStore((s) => (
    s.workspace?.tabs.find((tab) => tab.id === tabId)?.state?.scrollTop
  ));
  const updateTab = useWorkspaceStore((s) => s.updateTab);
  const tabIdRef = useRef(tabId);
  const scrollTopRef = useRef(0);
  const persistedScrollTopRef = useRef<number | null>(null);
  const didScrollRef = useRef(false);

  tabIdRef.current = tabId;

  // Restaura scroll quando a aba monta
  useEffect(() => {
    if (scrollTop === null || scrollTop === undefined || !scrollRef.current) {
      persistedScrollTopRef.current = null;
      return;
    }
    const saved = Number(scrollTop);
    if (!Number.isFinite(saved)) return;
    scrollTopRef.current = saved;
    persistedScrollTopRef.current = saved;
    didScrollRef.current = false;
    scrollRef.current.scrollTop = saved;
    requestAnimationFrame(() => {
      if (scrollRef.current) {
        scrollRef.current.scrollTop = saved;
      }
    });
  }, [scrollRef, scrollTop, tabId]);

  // Rastreia scroll position
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const handleScroll = () => {
      scrollTopRef.current = el.scrollTop;
      didScrollRef.current = true;
    };
    el.addEventListener('scroll', handleScroll, { passive: true });
    return () => el.removeEventListener('scroll', handleScroll);
  }, [scrollRef]);

  // Salva scroll no unmount (troca de aba ou fechamento)
  useEffect(() => {
    return () => {
      const tabId = tabIdRef.current;
      if (tabId) {
        const currentScrollTop = scrollRef.current?.scrollTop ?? scrollTopRef.current;
        if (persistedScrollTopRef.current === null && !didScrollRef.current) return;
        if (persistedScrollTopRef.current === currentScrollTop) return;
        void updateTab(tabId, { state: { scrollTop: currentScrollTop } });
      }
    };
  }, [scrollRef, updateTab]);
}
