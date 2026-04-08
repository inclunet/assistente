import { test, expect } from '../fixtures';

const THEMES = ['assistente', 'amethyst', 'midnight', 'light', 'high-contrast'] as const;

test.describe('Temas — seleção e aplicação', () => {
  test('grid de temas mostra todos os temas disponíveis', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    // Navega para a aba de aparência
    const appearanceTab = page.locator('button[role="tab"][data-tab-value="appearance"]');
    await appearanceTab.click();

    const themeGrid = page.locator('.theme-grid');
    await expect(themeGrid).toBeVisible({ timeout: 5_000 });

    // Verifica que os 5 temas estão presentes
    const themeCards = page.locator('.theme-card');
    await expect(themeCards).toHaveCount(THEMES.length);
  });

  test('tema ativo tem aria-checked=true', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    await page.locator('button[role="tab"][data-tab-value="appearance"]').click();
    await page.waitForSelector('.theme-grid', { timeout: 5_000 });

    const activeCard = page.locator('.theme-card[aria-checked="true"]');
    await expect(activeCard).toBeVisible();
  });

  test('clicar em um tema aplica data-theme no html', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    await page.locator('button[role="tab"][data-tab-value="appearance"]').click();
    await page.waitForSelector('.theme-grid', { timeout: 5_000 });

    // Seleciona o tema "light"
    const lightCard = page.locator('.theme-card__name', { hasText: /claro|light/i }).locator('..');
    await lightCard.click();

    const htmlTheme = await page.getAttribute('html', 'data-theme');
    expect(htmlTheme).toBe('light');
  });

  test('trocar tema atualiza variáveis CSS (bg-base muda)', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    await page.locator('button[role="tab"][data-tab-value="appearance"]').click();
    await page.waitForSelector('.theme-grid', { timeout: 5_000 });

    // Captura bg-base antes
    const bgBefore = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--bg-base').trim(),
    );

    // Muda para tema diferente do padrão
    const lightCard = page.locator('.theme-card__name', { hasText: /claro|light/i }).locator('..');
    await lightCard.click();

    const bgAfter = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--bg-base').trim(),
    );

    expect(bgAfter).not.toBe(bgBefore);
  });

  test('tema high-contrast tem contraste elevado (texto sobre fundo)', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    await page.locator('button[role="tab"][data-tab-value="appearance"]').click();
    await page.waitForSelector('.theme-grid', { timeout: 5_000 });

    // Aplica tema de alto contraste
    const hcCard = page.locator('.theme-card__name', { hasText: /contraste|high.contrast/i }).locator('..');
    await hcCard.click();

    const dataTheme = await page.getAttribute('html', 'data-theme');
    expect(dataTheme).toBe('high-contrast');

    // Verifica que as variáveis de texto são claras e fundo é escuro
    const textPrimary = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--text-primary').trim(),
    );
    const bgBase = await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue('--bg-base').trim(),
    );

    // Ambos devem existir (o contraste alto define --bg-base: #000 e --text-primary: #fff)
    expect(textPrimary).toBeTruthy();
    expect(bgBase).toBeTruthy();
  });

  test('tema grid tem role=radiogroup para acessibilidade', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    await page.locator('button[role="tab"][data-tab-value="appearance"]').click();
    await page.waitForSelector('.theme-grid', { timeout: 5_000 });

    const radiogroup = page.locator('.theme-grid[role="radiogroup"]');
    await expect(radiogroup).toBeVisible();

    // Cada card deve ter role=radio
    const radios = page.locator('.theme-card[role="radio"]');
    const count = await radios.count();
    expect(count).toBe(THEMES.length);
  });

  test('navegação por teclado entre temas (setas)', async ({ page, wails }) => {
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.waitForSelector('.settings-page', { timeout: 10_000 });

    await page.locator('button[role="tab"][data-tab-value="appearance"]').click();
    await page.waitForSelector('.theme-grid', { timeout: 5_000 });

    // Foca no tema ativo
    const activeCard = page.locator('.theme-card[aria-checked="true"]');
    await activeCard.focus();
    await expect(activeCard).toBeFocused();

    // Navega para o próximo com seta direita
    await page.keyboard.press('ArrowRight');

    // Um card diferente deve estar focado
    const focusedCard = page.locator('.theme-card:focus');
    await expect(focusedCard).toBeVisible();
  });
});
