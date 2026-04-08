import { test, expect } from '../fixtures';

declare global {
  interface Window {
    __origRAF?: typeof requestAnimationFrame;
    __rafQueue?: FrameRequestCallback[];
  }
}

/**
 * Testes de navegação por setas dentro de toolbars.
 *
 * Padrão ARIA Toolbar com roving tabindex:
 * - Arrow Right/Down → próximo botão
 * - Arrow Left/Up → botão anterior
 * - Home → primeiro botão
 * - End → último botão
 * - Apenas um botão tem tabindex="0" (roving)
 *
 * Para evitar race conditions com restoreDefaultFocus (rAF),
 * os testes pausam o requestAnimationFrame durante as interações.
 */

/**
 * Pausa requestAnimationFrame para evitar que restoreDefaultFocus roube o foco
 * durante a interação com a toolbar.
 */
async function pauseRAF(page: import('@playwright/test').Page) {
  await page.evaluate(() => {
    window.__origRAF = window.requestAnimationFrame;
    window.requestAnimationFrame = (cb: FrameRequestCallback) => {
      // Armazena callbacks para executar depois
      window.__rafQueue = window.__rafQueue || [];
      window.__rafQueue.push(cb);
      return 0;
    };
  });
}

async function resumeRAF(page: import('@playwright/test').Page) {
  await page.evaluate(() => {
    if (window.__origRAF) {
      window.requestAnimationFrame = window.__origRAF;
      // Executa callbacks pendentes
      const queue = window.__rafQueue || [];
      window.__rafQueue = [];
      for (const cb of queue) {
        try { cb(performance.now()); } catch (_) { /* ignore */ }
      }
    }
  });
}

test.describe('Toolbar — navegação por setas (workspace toolbar)', () => {
  test('ArrowRight move roving tabindex para o próximo elemento', async ({ page, wails }) => {
    await wails.waitForApp();
    await pauseRAF(page);

    const wsToolbar = page.locator('.workspace-toolbar[role="toolbar"]');
    const firstBtn = wsToolbar.locator('button:not([disabled])').first();
    await firstBtn.focus();
    await expect(firstBtn).toHaveAttribute('tabindex', '0');

    await page.keyboard.press('ArrowRight');

    // Primeiro botão volta para tabindex="-1"
    await expect(firstBtn).toHaveAttribute('tabindex', '-1', { timeout: 3_000 });

    // Outro elemento recebe tabindex="0"
    const activeItem = wsToolbar.locator('[tabindex="0"]:not(.toolbar__search)');
    await expect(activeItem).toHaveCount(1);
    await resumeRAF(page);
  });

  test('ArrowLeft move roving tabindex para o elemento anterior', async ({ page, wails }) => {
    await wails.waitForApp();
    await pauseRAF(page);

    const wsToolbar = page.locator('.workspace-toolbar[role="toolbar"]');
    const items = wsToolbar.locator('button:not([disabled]), [role="combobox"]');
    const count = await items.count();
    if (count < 2) return;

    // Foca segundo item
    await items.nth(1).focus();
    await expect(items.nth(1)).toHaveAttribute('tabindex', '0');
    await page.keyboard.press('ArrowLeft');

    await expect(items.first()).toHaveAttribute('tabindex', '0', { timeout: 3_000 });
    await expect(items.nth(1)).toHaveAttribute('tabindex', '-1');
    await resumeRAF(page);
  });

  test('Home move roving para o primeiro, End para o último', async ({ page, wails }) => {
    await wails.waitForApp();
    await pauseRAF(page);

    const wsToolbar = page.locator('.workspace-toolbar[role="toolbar"]');
    const items = wsToolbar.locator('button:not([disabled]), [role="combobox"]');
    const count = await items.count();
    if (count < 2) return;

    // Move para o segundo
    await items.nth(1).focus();
    await expect(items.nth(1)).toHaveAttribute('tabindex', '0');

    // Home → primeiro
    await page.keyboard.press('Home');
    await expect(items.first()).toHaveAttribute('tabindex', '0', { timeout: 3_000 });

    // End → último
    await page.keyboard.press('End');
    await expect(items.nth(count - 1)).toHaveAttribute('tabindex', '0', { timeout: 3_000 });
    await expect(items.first()).toHaveAttribute('tabindex', '-1');

    await resumeRAF(page);
  });

  test('roving tabindex: apenas um elemento tem tabindex=0 por vez', async ({ page, wails }) => {
    await wails.waitForApp();
    await pauseRAF(page);

    const wsToolbar = page.locator('.workspace-toolbar[role="toolbar"]');
    const items = wsToolbar.locator('button:not([disabled]), [role="combobox"]');
    const count = await items.count();
    if (count < 2) return;

    await items.first().focus();
    await expect(items.first()).toHaveAttribute('tabindex', '0');
    for (let i = 1; i < count; i++) {
      await expect(items.nth(i)).toHaveAttribute('tabindex', '-1');
    }

    await pauseRAF(page);
    await page.keyboard.press('ArrowRight');
    await expect(items.first()).toHaveAttribute('tabindex', '-1', { timeout: 3_000 });
    await expect(items.nth(1)).toHaveAttribute('tabindex', '0');
    await resumeRAF(page);
  });
});

test.describe('Toolbar — navegação por setas (topbar)', () => {
  test('setas atualizam roving tabindex no topbar', async ({ page, wails }) => {
    await wails.waitForApp();
    await pauseRAF(page);

    const topbar = page.locator('.topbar__toolbar[role="toolbar"]');
    const items = topbar.locator('button:not([disabled]), [role="combobox"]');
    const count = await items.count();
    if (count < 2) return;

    await items.first().focus();
    await expect(items.first()).toHaveAttribute('tabindex', '0');
    await page.keyboard.press('ArrowRight');
    await expect(items.first()).toHaveAttribute('tabindex', '-1', { timeout: 3_000 });
    await resumeRAF(page);
  });
});
