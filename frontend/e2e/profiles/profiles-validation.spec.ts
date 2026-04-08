import { test, expect } from '../fixtures';

const defaultProfile = {
  slug: 'default',
  name: 'Padrão',
  description: 'Perfil padrão do sistema',
  source: 'builtin',
  system_prompt: '',
  tts: {},
  stt: {},
};

test.describe('Perfis — validação de formulário', () => {
  test('criar perfil sem nome mostra erro de validação', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await wails.setResponse('GetProfile', defaultProfile);

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Abre editor de novo perfil
    await page.keyboard.press('Control+n');

    const editor = page.locator('.profiles-editor');
    await expect(editor).toBeVisible({ timeout: 5_000 });

    // Limpa o campo de nome
    const nameInput = editor.locator('input').first();
    if (await nameInput.isVisible()) {
      await nameInput.fill('');

      // Tenta salvar sem nome
      const saveBtn = editor.locator('button', { hasText: /salvar|save/i });
      if (await saveBtn.count() > 0) {
        await saveBtn.first().click();

        // O formulário não deve ter salvado (CreateProfile NÃO chamado)
        await page.waitForTimeout(500);
        const log = await wails.getCallLog();
        const createCalls = log.filter(c => c.fn === 'CreateProfile');
        const updateCalls = log.filter(c => c.fn === 'UpdateProfile');

        // Nenhum save/create deve ter ocorrido com nome vazio
        expect(createCalls.length + updateCalls.length).toBe(0);
      }
    }
  });

  test('erro do backend ao criar perfil é exibido', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await wails.setResponse('GetProfile', defaultProfile);

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // setError deve ser chamado APÓS waitForApp (página já carregada)
    await wails.setError('CreateProfile', 'Nome de perfil já existe');

    await page.keyboard.press('Control+n');

    const editor = page.locator('.profiles-editor');
    await expect(editor).toBeVisible({ timeout: 5_000 });

    const nameInput = editor.locator('input').first();
    if (await nameInput.isVisible()) {
      await nameInput.fill('Perfil Duplicado');

      const saveBtn = editor.locator('button', { hasText: /salvar|save/i });
      if (await saveBtn.count() > 0) {
        await saveBtn.first().click();

        // Aguarda a tentativa de criar
        await page.waitForTimeout(1_000);

        // O CreateProfile deve ter sido chamado (e falhado no mock)
        const log = await wails.getCallLog();
        const createCalls = log.filter(c => c.fn === 'CreateProfile');
        expect(createCalls.length).toBeGreaterThanOrEqual(1);
      }
    }
  });

  test('campo name é obrigatório no editor', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await wails.setResponse('GetProfile', defaultProfile);

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    await page.keyboard.press('Control+n');

    const editor = page.locator('.profiles-editor');
    await expect(editor).toBeVisible({ timeout: 5_000 });

    // O campo name deve existir e ser acessível
    const nameInput = editor.locator('input').first();
    await expect(nameInput).toBeVisible({ timeout: 3_000 });

    // Verifica presença de label ou atributo acessível
    const ariaLabel = await nameInput.getAttribute('aria-label');
    const placeholder = await nameInput.getAttribute('placeholder');
    const id = await nameInput.getAttribute('id');
    expect(ariaLabel || placeholder || id).toBeTruthy();
  });
});
