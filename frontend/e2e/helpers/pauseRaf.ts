import type { Page } from '@playwright/test';

declare global {
  interface Window {
    __origRAF?: typeof requestAnimationFrame;
    __rafQueue?: FrameRequestCallback[];
  }
}

/**
 * Pausa requestAnimationFrame para testes que dependem do foco não ser
 * redirecionado por restoreDefaultFocus (ex.: roving tabindex em tablist/toolbar).
 */
export async function pauseRAF(page: Page) {
  await page.evaluate(() => {
    if (window.__origRAF) return;
    window.__origRAF = window.requestAnimationFrame;
    window.requestAnimationFrame = (cb: FrameRequestCallback) => {
      window.__rafQueue = window.__rafQueue || [];
      window.__rafQueue.push(cb);
      return 0;
    };
  });
}

export async function resumeRAF(page: Page) {
  await page.evaluate(() => {
    if (!window.__origRAF) return;
    window.requestAnimationFrame = window.__origRAF;
    window.__origRAF = undefined;
    const queue = window.__rafQueue || [];
    window.__rafQueue = [];
    for (const cb of queue) {
      try { cb(performance.now()); } catch (_) { /* ignore */ }
    }
  });
}
