import { test, expect } from '../fixtures';
import {
  configureProfileMutation,
  configureProfilePage,
  expectProfileMutationProtocol,
  installProfileDeleteDecision,
  profileCommandTicket,
  profileCommandTarget,
  waitForProfileMutationResult,
} from '../helpers/profileCommand';

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
    await configureProfilePage(wails);

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
    await configureProfilePage(wails);
    await configureProfileMutation(wails, 'profiles.create', 'new-profile');

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    await page.getByRole('button', { name: /novo perfil|new profile/i }).click();

    const editor = page.locator('.profiles-editor');
    await expect(editor).toBeVisible({ timeout: 5_000 });

    // Preenche o nome
    const nameInput = editor.locator('input').first();
    await expect(nameInput).toBeVisible();
    await nameInput.fill('Meu Perfil Novo');

    // Tenta salvar
    const saveBtn = editor.locator('button', { hasText: /salvar|save/i });
    await expect(saveBtn.first()).toBeVisible();
    await saveBtn.first().click();
    await waitForProfileMutationResult(wails, profileCommandTicket);
    const log = await wails.getCallLog();
    expectProfileMutationProtocol(log, 'profiles.create', { targetId: '', profileName: 'Meu Perfil Novo' });
    expect(log.some((call) => call.fn === 'CreateProfile' || call.fn === 'UpdateProfile')).toBe(false);
    await expect(editor).toBeHidden();
  });
});

test.describe('Perfis — edição', () => {
  test('Enter em perfil no grid abre editor', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile, customProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await configureProfilePage(wails, { ...profileCommandTarget, name: customProfile.name });

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
    await configureProfilePage(wails, { ...profileCommandTarget, name: customProfile.name });
    await configureProfileMutation(wails, 'profiles.update');

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

    await waitForProfileMutationResult(wails, profileCommandTicket);
    const log = await wails.getCallLog();
    expectProfileMutationProtocol(log, 'profiles.update', { targetId: 'coder', profileName: 'Nome Editado' });
    expect(log.some((call) => call.fn === 'UpdateProfile')).toBe(false);
  });
});

test.describe('Perfis — exclusão', () => {
  test('deletar perfil inativo via botão da toolbar', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile, customProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await configureProfilePage(wails, { ...profileCommandTarget, name: customProfile.name });
    await configureProfileMutation(wails, 'profiles.delete');

    await wails.waitForApp();
    await installProfileDeleteDecision(page);
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Aguarda o perfil custom aparecer no grid
    await page.waitForSelector('[role="gridcell"]', { timeout: 5_000 });

    // Seleciona diretamente o perfil inativo. O teste de navegação por teclado
    // já cobre o roving tabindex; aqui o foco é a ação de exclusão.
    await page.getByRole('gridcell', { name: 'Programador' }).click();
    await page.keyboard.press('ArrowDown');

    const deleteButton = page.getByRole('button', { name: 'Delete' });
    await expect(deleteButton).toBeVisible();
    await expect(deleteButton).toBeEnabled();
    await deleteButton.click();

    // Confirma exclusão no DecisionDialog (AEP-0091), não no window.confirm nativo
    const confirmDialog = page.locator('.decision-dialog-modal');
    await expect(confirmDialog).toBeVisible({ timeout: 5_000 });
    await confirmDialog.locator('[data-decision-action="apply"]').click();
    await expect(confirmDialog).not.toBeVisible({ timeout: 3_000 });

    // Aguarda a mutação confirmada pelo protocolo page-mutation.
    await waitForProfileMutationResult(wails, profileCommandTicket);

    // Verifica chamada
    const log = await wails.getCallLog();
    expectProfileMutationProtocol(log, 'profiles.delete', { targetId: 'coder' });
    expect(log.some((call) => call.fn === 'DeleteProfile')).toBe(false);
  });
});

test.describe('Perfis — ativação', () => {
  test('ativar perfil via menu de ações na linha', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile, customProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await configureProfilePage(wails, { ...profileCommandTarget, name: customProfile.name });
    await configureProfileMutation(wails, 'profiles.activate');

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Encontra o botão de ações (MenuButton ⋮) da segunda linha
    const rows = page.locator('[role="row"]');
    const customRow = rows.filter({ hasText: 'Programador' });

    const actionBtn = customRow.getByRole('button', { name: /^(ações|actions)$/i });
    await expect(actionBtn.first()).toBeVisible();
    await actionBtn.first().click();

    // Menu de contexto aparece — clica em "Ativar"
    const activateItem = page.locator('[role="menuitem"]', { hasText: /ativar|activate/i });
    await expect(activateItem.first()).toBeVisible();
    await activateItem.first().click();

    await page.waitForFunction((ticket) => window.__wailsMock.getCallLog().some(
      (call: { fn: string; args: unknown[] }) => call.fn === 'GetPageMutationCommandResult' && call.args[0] === ticket,
    ), profileCommandTicket, { timeout: 5_000 });
    const log = await wails.getCallLog();
    expectProfileMutationProtocol(log, 'profiles.activate', { targetId: 'coder' });
    expect(log.some((call) => call.fn === 'SetActiveProfile')).toBe(false);
    await expect(page.locator('.toast__message').filter({ hasText: /ativado|activated/i })).toBeVisible();
  });
});

test.describe('Perfis — duplicação', () => {
  test('duplicar perfil via menu de ações', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile, customProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await configureProfilePage(wails, { ...profileCommandTarget, name: customProfile.name });
    await configureProfileMutation(wails, 'profiles.duplicate', 'coder-copy');

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Encontra a linha do perfil customizado
    const rows = page.locator('[role="row"]');
    const customRow = rows.filter({ hasText: 'Programador' });

    const actionBtn = customRow.getByRole('button', { name: /^(ações|actions)$/i });
    await expect(actionBtn.first()).toBeVisible();
    await actionBtn.first().click();

    // Clica em "Duplicar"
    const dupItem = page.locator('[role="menuitem"]', { hasText: /duplic/i });
    await expect(dupItem.first()).toBeVisible();
    await dupItem.first().click();

    await page.waitForFunction((ticket) => window.__wailsMock.getCallLog().some(
      (call: { fn: string; args: unknown[] }) => call.fn === 'GetPageMutationCommandResult' && call.args[0] === ticket,
    ), profileCommandTicket, { timeout: 5_000 });
    const log = await wails.getCallLog();
    expectProfileMutationProtocol(log, 'profiles.duplicate', { targetId: 'coder' });
    expect(log.some((call) => call.fn === 'DuplicateProfile')).toBe(false);
    await expect(page.locator('.profiles-editor')).toBeVisible();
  });
});
