import { useEffect } from 'react';

type Args = {
  flushNow: () => void;
};

export function useRichEditorFlushEvents({ flushNow }: Args) {
  useEffect(() => {
    const onFlushRequest = () => {
      flushNow();
    };

    const onVisibilityChange = () => {
      try {
        if (document.visibilityState === 'hidden') flushNow();
      } catch {
        // best-effort
      }
    };

    const onPageHide = () => {
      flushNow();
    };

    const onBeforeUnload = () => {
      flushNow();
    };

    const onBlur = () => {
      flushNow();
    };

    window.addEventListener('assistente:flush-rich-editor', onFlushRequest);
    document.addEventListener('visibilitychange', onVisibilityChange);
    window.addEventListener('pagehide', onPageHide);
    window.addEventListener('beforeunload', onBeforeUnload);
    window.addEventListener('blur', onBlur);

    return () => {
      window.removeEventListener('assistente:flush-rich-editor', onFlushRequest);
      document.removeEventListener('visibilitychange', onVisibilityChange);
      window.removeEventListener('pagehide', onPageHide);
      window.removeEventListener('beforeunload', onBeforeUnload);
      window.removeEventListener('blur', onBlur);
    };
  }, [flushNow]);
}
