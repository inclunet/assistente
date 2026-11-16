import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { GetLogSettings } from '@wailsjs/go/wailsapi/Settings';
import { useAuthStore } from '../store/authStore';
import { useUIStore } from '../store/uiStore';

/** Avisa sobre falhas de logs na inicialização ou durante a sessão. */
export function useLogStartupWarning() {
  const { t } = useTranslation();
  const sessionId = useAuthStore((state) => state.user?.sessionId ?? null);
  const authenticated = useAuthStore((state) => state.isAuthenticated);
  const addToast = useUIStore((state) => state.addToast);
  const notifiedSession = useRef<string | null>(null);

  useEffect(() => {
    if (!authenticated || !sessionId) { notifiedSession.current = null; return; }
    let cancelled = false;
    const check = () => {
      void Promise.resolve().then(GetLogSettings).then((result) => {
        if (cancelled) return;
        if (result.error && notifiedSession.current !== sessionId) addToast(t('logSettings.startupToast'), 'warning', 15000);
        notifiedSession.current = result.error ? sessionId : null;
      }).catch(() => { /* A configuração de logs pode estar indisponível durante o teardown. */ });
    };
    check();
    const timer = window.setInterval(check, 30000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [authenticated, sessionId, addToast, t]);
}
