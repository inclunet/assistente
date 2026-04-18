import { describe, expect, it, vi, afterEach } from 'vitest';
import { waitForWailsBridge } from './waitForWailsBridge';

describe('waitForWailsBridge', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('resolve no próximo frame quando o bridge aparece sem esperar 100ms', async () => {
    let scheduled: FrameRequestCallback | null = null;
    const target = {
      go: undefined,
      requestAnimationFrame: vi.fn((cb: FrameRequestCallback) => {
        scheduled = cb;
        return 1;
      }),
      cancelAnimationFrame: vi.fn(),
    } as unknown as Window & { go?: unknown };

    const pending = waitForWailsBridge({ target });

    expect(target.requestAnimationFrame).toHaveBeenCalledTimes(1);

    target.go = { main: { App: {} } };
    expect(scheduled).not.toBeNull();
    scheduled!(performance.now());

    await expect(pending).resolves.toBeUndefined();
  });

  it('usa fallback curto quando requestAnimationFrame não está disponível', async () => {
    vi.useFakeTimers();

    const target = {
      go: undefined,
    } as unknown as Window & { go?: unknown };

    const pending = waitForWailsBridge({ target });

    target.go = { main: { App: {} } };
    await vi.advanceTimersByTimeAsync(16);

    await expect(pending).resolves.toBeUndefined();
  });

  it('rejeita com timeout controlado quando o bridge não aparece', async () => {
    vi.useFakeTimers();

    const target = {
      go: undefined,
    } as unknown as Window & { go?: unknown };

    const pending = waitForWailsBridge({ target, timeoutMs: 50 });
    const assertion = expect(pending).rejects.toThrow('Timed out waiting for Wails bridge after 50ms');

    await vi.advanceTimersByTimeAsync(50);

    await assertion;
  });
});
