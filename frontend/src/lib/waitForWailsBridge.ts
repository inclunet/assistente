type WailsWindow = Window & { go?: unknown };

function hasWailsBridge(target: WailsWindow): boolean {
  return typeof target.go !== 'undefined';
}

interface WaitForWailsBridgeOptions {
  signal?: AbortSignal;
  target?: WailsWindow;
  timeoutMs?: number;
}

export async function waitForWailsBridge(
  options: WaitForWailsBridgeOptions = {},
): Promise<void> {
  if (typeof window === 'undefined') return;

  const target = options.target ?? (window as WailsWindow);
  const timeoutMs = options.timeoutMs ?? 5000;
  if (hasWailsBridge(target)) return;

  await new Promise<void>((resolve, reject) => {
    let rafId: number | null = null;
    let intervalId: ReturnType<typeof setTimeout> | null = null;
    let timeoutId: ReturnType<typeof setTimeout> | null = null;

    const cleanup = () => {
      if (rafId !== null && typeof target.cancelAnimationFrame === 'function') {
        target.cancelAnimationFrame(rafId);
      }
      if (intervalId !== null) {
        clearTimeout(intervalId);
      }
      if (timeoutId !== null) {
        clearTimeout(timeoutId);
      }
      options.signal?.removeEventListener('abort', handleAbort);
    };

    const handleAbort = () => {
      cleanup();
      reject(new DOMException('Aborted', 'AbortError'));
    };

    const schedule = () => {
      if (typeof target.requestAnimationFrame === 'function') {
        rafId = target.requestAnimationFrame(check);
        return;
      }
      intervalId = setTimeout(check, 16);
    };

    const check = () => {
      if (hasWailsBridge(target)) {
        cleanup();
        resolve();
        return;
      }
      if (options.signal?.aborted) {
        handleAbort();
        return;
      }
      schedule();
    };

    if (options.signal?.aborted) {
      handleAbort();
      return;
    }

    if (timeoutMs > 0) {
      timeoutId = setTimeout(() => {
        cleanup();
        reject(new Error(`Timed out waiting for Wails bridge after ${timeoutMs}ms`));
      }, timeoutMs);
    }

    options.signal?.addEventListener('abort', handleAbort);
    schedule();
  });
}
