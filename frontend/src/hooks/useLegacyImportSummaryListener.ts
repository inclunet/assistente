import { useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { useAuthStore } from '../store/authStore';
import { useUIStore } from '../store/uiStore';
import { formatPortabilityMessage, type PortabilityMessage } from '../lib/portabilityMessages';

// Event payload mirrors portability.LegacyImportSummary; it is not a Wails binding.
interface LegacyImportSummaryEvent {
  userId?: string;
  imported?: number;
  skipped?: number;
  failed?: number;
  warningCount?: number;
  errorCount?: number;
  entries?: { warningMessages?: PortabilityMessage[] }[];
}

/** One global listener: preserves pre-login delivery and announces via addToast. */
export function useLegacyImportSummaryListener() {
  const { t } = useTranslation();
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const user = useAuthStore((s) => s.user);
  const loading = useAuthStore((s) => s.isLoading);
  const addToast = useUIStore((s) => s.addToast);
  const pending = useRef<LegacyImportSummaryEvent | null>(null);

  const show = useCallback((event: LegacyImportSummaryEvent) => {
    const auth = useAuthStore.getState();
    if (!auth.isAuthenticated || !auth.user || (event.userId && event.userId !== auth.user.userId)) return;
    const imported = event.imported ?? 0;
    const skipped = event.skipped ?? 0;
    const failed = event.failed ?? 0;
    const warnings = event.warningCount ?? 0;
    const errors = event.errorCount ?? 0;
    if (imported === 0 && skipped === 0 && failed === 0 && warnings === 0 && errors === 0) return;
    const details = (event.entries ?? []).flatMap((entry) => entry.warningMessages ?? [])
      .map((message) => formatPortabilityMessage(message, t)).filter(Boolean);
    const message = [t('app.legacyImport.summary', { imported, skipped, failed, warnings }), ...details].join('\n');
    addToast(message, failed > 0 || errors > 0 ? 'error' : warnings > 0 ? 'warning' : 'success', 10000);
  }, [addToast, t]);

  useEffect(() => {
    if (isAuthenticated && user && pending.current) {
      const event = pending.current;
      pending.current = null;
      show(event);
    } else if (!isAuthenticated && !loading) {
      pending.current = null;
    }
  }, [isAuthenticated, user, loading, show]);

  useEffect(() => EventsOn('legacy:import_summary', (data: unknown) => {
    if (!data || typeof data !== 'object') return;
    const event = data as LegacyImportSummaryEvent;
    const auth = useAuthStore.getState();
    if (!auth.isAuthenticated || !auth.user) {
      if (auth.isLoading) pending.current = event;
      return;
    }
    show(event);
  }), [show]);
}
