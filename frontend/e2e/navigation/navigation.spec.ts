import { test, expect } from '../fixtures';

test.describe('Navegação — rotas principais', () => {
  test('rota raiz carrega o workspace com chat', async ({ page, wails }) => {
    await wails.waitForApp();

    // O layout do workspace deve estar visível
    const layout = page.locator('.workspace-layout');
    await expect(layout).toBeVisible();
  });

  test('navegar para settings carrega a página', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page, .settings-tabs', { timeout: 10_000 });
  });

  test('navegar para history carrega a página', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', []);
    await wails.waitForApp();

    await page.goto('/#/history');
    await page.waitForSelector('.history-page, .conversation-list', { timeout: 10_000 });
  });

  test('navegar para profiles carrega a página', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', []);
    await wails.waitForApp();

    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page, [role="grid"]', { timeout: 10_000 });
  });
});

test.describe('Navegação — topbar', () => {
  test('topbar está visível e contém botões de navegação', async ({ page, wails }) => {
    await wails.waitForApp();

    const topbar = page.locator('.topbar');
    await expect(topbar).toBeVisible();
  });
});
