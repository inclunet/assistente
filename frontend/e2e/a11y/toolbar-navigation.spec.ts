import { test, expect } from '../fixtures';
import { pauseRAF, resumeRAF } from '../helpers/pauseRaf';

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

test.describe('Toolbar — navegação por setas (workspace toolbar)', () => {
  test('ArrowRight move roving tabindex para o próximo elemento', async ({ page, wails }) => {
    await wails.waitForApp();
    await pauseRAF(page);

    const wsToolbar = page.locator('.workspace-toolbar[role="toolbar"]');
    const firstBtn = wsToolbar.locator('button:not([disabled])').first();
    await firstBtn.focus();
    await expect(firstBtn).toBeFocused({ timeout: 3_000 });
    await expect(firstBtn).toHaveAttribute('tabindex', '0');

    await firstBtn.press('ArrowRight');

    const secondItem = wsToolbar.locator('button:not([disabled]), [role="combobox"]').nth(1);
    await expect(secondItem).toHaveAttribute('tabindex', '0', { timeout: 5_000 });
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

    await items.first().focus();
    await expect(items.first()).toHaveAttribute('tabindex', '0');
    await items.first().press('ArrowRight');
    await expect(items.nth(1)).toHaveAttribute('tabindex', '0', { timeout: 3_000 });
    await items.nth(1).press('ArrowLeft');

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

    await items.first().focus();
    await expect(items.first()).toHaveAttribute('tabindex', '0');
    await items.first().press('ArrowRight');
    await expect(items.nth(1)).toHaveAttribute('tabindex', '0', { timeout: 3_000 });

    // Home → primeiro
    await items.nth(1).press('Home');
    await expect(items.first()).toBeFocused({ timeout: 3_000 });
    await expect(items.first()).toHaveAttribute('tabindex', '0', { timeout: 3_000 });

    // End → último
    await items.first().press('End');
    const lastItem = items.last();
    await expect(lastItem).toBeFocused({ timeout: 5_000 });
    await expect(lastItem).toHaveAttribute('tabindex', '0', { timeout: 5_000 });
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
    await items.first().press('ArrowRight');
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
    await items.first().press('ArrowRight');
    await expect(items.first()).toHaveAttribute('tabindex', '-1', { timeout: 3_000 });
    await resumeRAF(page);
  });
});
