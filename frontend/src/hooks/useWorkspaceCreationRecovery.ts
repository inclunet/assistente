import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { useAuthStore } from '../store/authStore';
import { useUIStore } from '../store/uiStore';

interface WorkspaceCreationRecoveryEvent { userId: string; sessionId: string; path: string }

export function useWorkspaceCreationRecovery() {
  const { t } = useTranslation();
  useEffect(() => {
    let active = true;
    const notices = new Set<string>();
    const clear = () => {
      notices.forEach(id => useUIStore.getState().removeToast(id));
      notices.clear();
    };
    const unsubscribeAuth = useAuthStore.subscribe((auth, previous) => {
      if (auth.isAuthenticated !== previous.isAuthenticated || auth.user?.userId !== previous.user?.userId ||
        auth.user?.sessionId !== previous.user?.sessionId) clear();
    });
    const unsubscribeEvents = EventsOn('workspace:creation-recovery', (event: WorkspaceCreationRecoveryEvent) => {
      const auth = useAuthStore.getState();
      if (!active || !auth.isAuthenticated || !event || auth.user?.userId !== event.userId ||
        auth.user?.sessionId !== event.sessionId || typeof event.path !== 'string' || !event.path) return;
      const ui = useUIStore.getState();
      for (const id of notices) if (!ui.toasts.some(toast => toast.id === id)) notices.delete(id);
      notices.add(ui.addToast(t('workspace.creationRecovery', { path: event.path }), 'warning', 15000));
    });
    return () => { active = false; unsubscribeEvents(); unsubscribeAuth(); clear(); };
  }, [t]);
}
