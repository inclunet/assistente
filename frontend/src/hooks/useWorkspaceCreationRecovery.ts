import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { useAuthStore } from '../store/authStore';
import { useUIStore } from '../store/uiStore';

interface WorkspaceCreationRecoveryEvent { userId: string; sessionId: string; path: string }

export function useWorkspaceCreationRecovery() {
  const { t } = useTranslation();
  useEffect(() => EventsOn('workspace:creation-recovery', (event: WorkspaceCreationRecoveryEvent) => {
    const auth = useAuthStore.getState();
    if (!auth.isAuthenticated || !event || auth.user?.userId !== event.userId ||
      auth.user?.sessionId !== event.sessionId || typeof event.path !== 'string' || !event.path) return;
    useUIStore.getState().addToast(t('workspace.creationRecovery', { path: event.path }), 'warning', 15000);
  }), [t]);
}
