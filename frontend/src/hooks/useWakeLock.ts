import { useEffect } from 'react';

import { SetWakeLock } from '@wailsjs/go/app/App';

import { useSettingsStore } from '../store/settingsStore';

// Hook que mantém a tela acordada enquanto a janela está em foco e a
// preferência preventScreenLock está ativa. Usa tanto o backend nativo
// (SetThreadExecutionState no Windows, cross-platform) quanto o Wake Lock API
// do navegador como camada extra quando disponível.
export function useWakeLock() {
  const preventScreenLock = useSettingsStore((s) => s.config.preventScreenLock);

  useEffect(() => {
    let wakeSentinel: WakeLockSentinel | null = null;

    const sync = async () => {
      const shouldLock = preventScreenLock && document.hasFocus() && document.visibilityState === 'visible';

      // Backend nativo (cross-platform) — fonte da verdade.
      try {
        await SetWakeLock(shouldLock);
      } catch {
        // Silencioso: binding pode não estar pronto em testes.
      }

      // Wake Lock API do navegador — camada extra quando disponível.
      if ('wakeLock' in navigator) {
        try {
          if (shouldLock && !wakeSentinel) {
            wakeSentinel = await (
              navigator as unknown as { wakeLock: { request: (t: string) => Promise<WakeLockSentinel> } }
            ).wakeLock.request('screen');
            wakeSentinel.addEventListener('release', () => {
              wakeSentinel = null;
            });
          } else if (!shouldLock && wakeSentinel) {
            await wakeSentinel.release();
            wakeSentinel = null;
          }
        } catch {
          // Permissão negada ou não suportado — silencioso.
        }
      }
    };

    sync();
    window.addEventListener('focus', sync);
    window.addEventListener('blur', sync);
    document.addEventListener('visibilitychange', sync);
    return () => {
      window.removeEventListener('focus', sync);
      window.removeEventListener('blur', sync);
      document.removeEventListener('visibilitychange', sync);
      if (wakeSentinel) void wakeSentinel.release();
      try {
        void SetWakeLock(false);
      } catch {
        // Silencioso.
      }
    };
  }, [preventScreenLock]);
}
