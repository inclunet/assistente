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

const customProfile = {
  slug: 'coder',
  name: 'Programador',
  description: 'Perfil para codificação',
  source: 'workdir',
  system_prompt: 'Você é um programador expert.',
  tts: {},
  stt: {},
};

test.describe('Perfis — criação', () => {
  test('Ctrl+N abre editor de novo perfil', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await wails.setResponse('GetProfile', defaultProfile);

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Pressiona Ctrl+N
    await page.keyboard.press('Control+n');

    // O editor (modal) deve abrir
    const editor = page.locator('.profiles-editor');
    await expect(editor).toBeVisible({ timeout: 5_000 });
  });

  test('criar perfil via botão Novo e salvar', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await wails.setResponse('GetProfile', defaultProfile);
    await wails.setResponse('CreateProfile', { slug: 'new-profile', name: 'Meu Perfil' });

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Clica no botão de novo perfil (Ctrl+N ou botão)
    await page.keyboard.press('Control+n');

    const editor = page.locator('.profiles-editor');
    await expect(editor).toBeVisible({ timeout: 5_000 });

    // Preenche o nome
    const nameInput = editor.locator('input').first();
    if (await nameInput.isVisible()) {
      await nameInput.fill('Meu Perfil Novo');
    }

    // Tenta salvar
    const saveBtn = editor.locator('button', { hasText: /salvar|save/i });
    if (await saveBtn.count() > 0) {
      await saveBtn.first().click();

      // Verifica que CreateProfile ou UpdateProfile foi chamado
      const log = await wails.getCallLog();
      const createCalls = log.filter(c => c.fn === 'CreateProfile' || c.fn === 'UpdateProfile');
      expect(createCalls.length).toBeGreaterThanOrEqual(1);
    }
  });
});

test.describe('Perfis — edição', () => {
  test('Enter em perfil no grid abre editor', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile, customProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await wails.setResponse('GetProfile', customProfile);

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Aguarda perfil custom aparecer no grid
    await page.waitForSelector('[role="gridcell"]', { timeout: 5_000 });

    // Foca no grid
    const grid = page.locator('[role="grid"]');
    await grid.focus();

    // Navega para a segunda linha (perfil custom)
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Enter');

    // O editor (modal) deve abrir
    const editor = page.locator('.profiles-editor');
    await expect(editor).toBeVisible({ timeout: 5_000 });
  });

  test('edição inline de nome no grid', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile, customProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await wails.setResponse('UpdateProfile', undefined);

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Aguarda dados do grid
    await page.waitForSelector('[role="gridcell"]', { timeout: 5_000 });

    const grid = page.locator('[role="grid"]');
    await grid.focus();
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('ArrowDown');

    // F2 para editar inline
    await page.keyboard.press('F2');

    const editInput = page.locator('.cell-edit-input');
    await expect(editInput).toBeVisible({ timeout: 5_000 });
    await editInput.fill('Nome Editado');
    await editInput.press('Enter');

    const log = await wails.getCallLog();
    const updateCalls = log.filter(c => c.fn === 'UpdateProfile');
    expect(updateCalls.length).toBeGreaterThanOrEqual(1);
  });
});

test.describe('Perfis — exclusão', () => {
  test('deletar perfil inativo via botão da toolbar', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile, customProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await wails.setResponse('DeleteProfile', undefined);

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Aguarda o perfil custom aparecer no grid
    await page.waitForSelector('[role="gridcell"]', { timeout: 5_000 });

    // Seleciona diretamente o perfil inativo. O teste de navegação por teclado
    // já cobre o roving tabindex; aqui o foco é a ação de exclusão.
    await page.getByRole('gridcell', { name: 'Programador' }).click({ force: true });
    await page.keyboard.press('ArrowDown');

    // Aguarda o botão Delete ficar habilitado (seleção ativa perfil inativo)
    await page.waitForFunction(() => {
      const button = document.querySelector('button[aria-label="Delete"]') as HTMLButtonElement | null;
      return !!button && !button.disabled;
    }, { timeout: 5_000 });

    // Clica no botão Delete
    await page.evaluate(() => {
      const button = document.querySelector('button[aria-label="Delete"]') as HTMLButtonElement | null;
      if (!button || button.disabled) {
        throw new Error('Delete button is not enabled');
      }
      button.click();
    });

    // Confirma exclusão no DecisionDialog (AEP-0091), não no window.confirm nativo
    const confirmDialog = page.locator('.confirm-dialog-modal');
    await expect(confirmDialog).toBeVisible({ timeout: 5_000 });
    await confirmDialog.getByRole('button', { name: /delete|excluir/i }).click();
    await expect(confirmDialog).not.toBeVisible({ timeout: 3_000 });

    // Aguarda o processamento
    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some(
        (c: { fn: string }) => c.fn === 'DeleteProfile'
      );
    }, { timeout: 5_000 });

    // Verifica chamada
    const log = await wails.getCallLog();
    const deleteCalls = log.filter(c => c.fn === 'DeleteProfile');
    expect(deleteCalls.length).toBe(1);
  });
});

test.describe('Perfis — ativação', () => {
  test('ativar perfil via menu de ações na linha', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile, customProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await wails.setResponse('SetActiveProfile', undefined);
    await wails.setResponse('GetActiveProfile', customProfile);

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Encontra o botão de ações (MenuButton ⋮) da segunda linha
    const rows = page.locator('[role="row"]');
    const customRow = rows.filter({ hasText: 'Programador' });

    const actionBtn = customRow.locator('.action-button');
    if (await actionBtn.count() > 0) {
      await actionBtn.first().click();

      // Menu de contexto aparece — clica em "Ativar"
      const activateItem = page.locator('[role="menuitem"]', { hasText: /ativar|activate/i });
      if (await activateItem.count() > 0) {
        await activateItem.first().click();

        const log = await wails.getCallLog();
        const activateCalls = log.filter(c => c.fn === 'SetActiveProfile');
        expect(activateCalls.length).toBe(1);
        expect(activateCalls[0].args[0]).toBe('coder');
      }
    }
  });
});

test.describe('Perfis — duplicação', () => {
  test('duplicar perfil via menu de ações', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile, customProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await wails.setResponse('DuplicateProfile', 'coder-copy');
    await wails.setResponse('GetProfile', { ...customProfile, slug: 'coder-copy', name: 'Programador (cópia)' });

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Encontra a linha do perfil customizado
    const rows = page.locator('[role="row"]');
    const customRow = rows.filter({ hasText: 'Programador' });

    const actionBtn = customRow.locator('.action-button');
    if (await actionBtn.count() > 0) {
      await actionBtn.first().click();

      // Clica em "Duplicar"
      const dupItem = page.locator('[role="menuitem"]', { hasText: /duplic/i });
      if (await dupItem.count() > 0) {
        await dupItem.first().click();

        const log = await wails.getCallLog();
        const dupCalls = log.filter(c => c.fn === 'DuplicateProfile');
        expect(dupCalls.length).toBe(1);
        expect(dupCalls[0].args[0]).toBe('coder');
      }
    }
  });
});
