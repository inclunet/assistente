import { test, expect } from '../fixtures';

test('Permissões abre a associação no editor real de perfis', async ({ page, wails }) => {
  const profile = {
    slug: 'default', name: 'Perfil para política', description: '', source: 'workdir',
    chat: { llm_provider: '', model: '' },
    voice: {
      assistant: { enabled: false, provider: 'disabled', voice_id: '', rate: 1, pitch: 1, volume: 1 },
      user: { enabled: false, provider: 'disabled', voice_id: '', rate: 1, pitch: 1, volume: 1 },
      system: { enabled: false, provider: 'disabled', voice_id: '', rate: 1, pitch: 1, volume: 1 },
    },
    input: { enabled: true, stt_provider: 'webspeech', language: 'pt-BR', feedback_sounds: true, triggers: [] },
    channels: { response_mode: 'mirror' }, context_providers: {},
  };
  await wails.setResponse('GetProfiles', [profile]);
  await wails.setResponse('ReadProfileCommandTarget', { profile, fingerprint: '0123456789abcdef'.repeat(4) });
  await wails.setResponse('GetActiveProfileSlug', 'default');
  await wails.waitForApp();
  await page.goto('/#/settings/permissions');
  await page.getByRole('button', { name: /associar políticas a perfis|assign policies to profiles/i }).click();
  await expect(page).toHaveURL(/#\/profiles$/);
  const cell = page.locator('[role="gridcell"]').filter({ hasText: 'Perfil para política' }).first();
  await cell.focus();
  await cell.press('Enter');
  await expect(page.locator('.profiles-editor')).toBeVisible();
  await expect(page.getByRole('button', { name: /cancelar|cancel/i })).toBeVisible();
});

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
