import { test, expect } from '../fixtures';

test.use({ locale: 'pt-BR' });

test('percorre o catálogo com roda e teclado sem trocar de aba ou perder o foco', async ({ page, wails }) => {
  const entries = Array.from({length: 240}, (_, i) => ({name: 'Plugin ' + i}));
  await wails.setResponse('ListRepositories', [{id: 'loja', revision: 'sha', origin: 'local',
    updated: new Date().toISOString(), indices: [{name: 'Loja', format: 'cursor', entries}]}]);
  await wails.setResponse('GetLocalCommandKeyboardMap', {
    generation: 'catalog-landmarks', validUntil: Date.now() + 30 * 60 * 1000,
    ownerId: 'user-e2e', sessionId: 'session-e2e', workspaceId: 'ws-1',
    bindings: [
      {shortcut: {version: 1, code: 'F8', modifiers: ['Control']}, commandId: 'navigation.landmark.next', handler: 'local_ui'},
      {shortcut: {version: 1, code: 'F8', modifiers: ['Control', 'Shift']}, commandId: 'navigation.landmark.previous', handler: 'local_ui'},
    ],
    localPaletteCommands: ['navigation.landmark.next', 'navigation.landmark.previous', 'navigation.landmark.default'],
  });
  await wails.waitForApp();
  await page.goto('/#/plugins');
  await page.getByRole('tab', {name: 'Catálogo'}).click();
  const grid = page.getByRole('grid', {name: 'Catálogo'});
  const plugin = (i: number) => grid.getByRole('gridcell', {name: 'Plugin ' + i, exact: true});
  const search = page.getByRole('toolbar', {name: 'Catálogo'}).getByRole('textbox');
  await search.focus();
  await page.keyboard.press('Control+F8');
  await expect(plugin(0)).toBeFocused();
  await expect(plugin(239)).toHaveCount(0);
  await grid.hover();
  await page.mouse.wheel(0, 2600);
  await expect.poll(() => grid.locator('.datagrid-body').evaluate(el => el.scrollTop)).toBeGreaterThan(1000);
  await expect(plugin(0)).toBeFocused();
  await page.keyboard.press('PageDown');
  await expect(plugin(10)).toBeFocused();
  await page.keyboard.press('Control+Shift+F8');
  await expect(search).toBeFocused();
  await page.keyboard.press('Control+F8');
  await expect(plugin(10)).toBeFocused();
  await page.keyboard.press('PageUp');
  await expect(plugin(0)).toBeFocused();
  await page.keyboard.press('Control+End');
  await expect(plugin(239)).toBeInViewport();
  await expect(grid.locator('[role="row"][aria-rowindex="241"] :focus')).toHaveCount(1);
  await page.keyboard.press('Control+Home');
  await expect(plugin(0)).toBeFocused();
  await expect(plugin(0)).toBeInViewport();
});
