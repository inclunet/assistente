import { test, expect } from '../fixtures';

test.use({ locale: 'pt-BR' });

test('percorre o catálogo com roda e teclado sem trocar de aba ou perder o foco', async ({ page, wails }) => {
  const entries = Array.from({length: 240}, (_, i) => ({name: 'Plugin ' + i}));
  await wails.setResponse('ListRepositories', [{id: 'loja', revision: 'sha', origin: 'local',
    updated: new Date().toISOString(), indices: [{name: 'Loja', format: 'cursor', entries}]}]);
  await wails.waitForApp();
  await page.goto('/#/plugins');
  await page.getByRole('tab', {name: 'Catálogo'}).click();
  const grid = page.getByRole('grid', {name: 'Catálogo'});
  const plugin = (i: number) => grid.getByRole('gridcell', {name: 'Plugin ' + i, exact: true});
  await plugin(0).click();
  await expect(plugin(0)).toBeFocused();
  await expect(plugin(239)).toHaveCount(0);
  await grid.hover();
  await page.mouse.wheel(0, 2600);
  await expect.poll(() => grid.locator('.datagrid-body').evaluate(el => el.scrollTop)).toBeGreaterThan(1000);
  await expect(plugin(0)).toBeFocused();
  await page.keyboard.press('Control+PageDown');
  await expect(plugin(10)).toBeFocused();
  await page.keyboard.press('Control+PageUp');
  await expect(plugin(0)).toBeFocused();
  await page.keyboard.press('Control+End');
  await expect(plugin(239)).toBeInViewport();
  await expect(grid.locator('[role="row"][aria-rowindex="241"] :focus')).toHaveCount(1);
  await page.keyboard.press('Control+Home');
  await expect(plugin(0)).toBeFocused();
  await expect(plugin(0)).toBeInViewport();
});
