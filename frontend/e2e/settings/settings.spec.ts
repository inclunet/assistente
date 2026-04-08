import { test, expect } from '../fixtures';

test.describe('Settings — navegação por abas', () => {
  test('página de settings carrega com abas visíveis', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    const tabList = page.locator('.settings-tabs__list');
    await expect(tabList).toBeVisible();
  });

  test('abas essenciais estão presentes', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    const expectedTabs = ['providers', 'appearance', 'credentials'];
    for (const tabId of expectedTabs) {
      const tab = page.locator(`button[role="tab"][data-tab-value="${tabId}"]`);
      await expect(tab).toBeVisible();
    }
  });

  test('clicar em uma aba exibe seu conteúdo', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    const appearanceTab = page.locator('button[role="tab"][data-tab-value="appearance"]');
    await appearanceTab.click();

    // O painel de aparência deve conter o grid de temas
    const themeGrid = page.locator('.theme-grid');
    await expect(themeGrid).toBeVisible({ timeout: 5_000 });
  });

  test('abas são acessíveis por teclado', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    // Foca na primeira aba
    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.focus();
    await expect(firstTab).toBeFocused();

    // Navega com seta direita
    await page.keyboard.press('ArrowRight');
    const secondTab = page.locator('button[role="tab"]').nth(1);
    await expect(secondTab).toBeFocused();
  });

  test('aba ativa tem aria-selected=true', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    const activeTab = page.locator('button[role="tab"][aria-selected="true"]');
    await expect(activeTab).toBeVisible();
  });
});

test.describe('Settings — provedores', () => {
  test('aba de provedores carrega sem erro', async ({ page, wails }) => {
    await wails.setResponse('GetLLMProvidersWithStatus', [
      {
        slug: 'openai',
        name: 'OpenAI',
        type: 'openai',
        isConfigured: true,
        isDefault: true,
      },
    ]);

    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    const providersTab = page.locator('button[role="tab"][data-tab-value="providers"]');
    await providersTab.click();

    // Deve carregar o conteúdo do painel sem erro
    const panel = page.locator('[role="tabpanel"]').first();
    await expect(panel).toBeVisible({ timeout: 5_000 });
  });
});
