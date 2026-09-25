import { test, expect } from '../fixtures';
import {
  configureProfileMutation,
  configureProfilePage,
  expectProfileMutationPreparation,
  profileCommandTarget,
  profileCommandTicket,
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

test.describe('Perfis — validação de formulário', () => {
  test('criar perfil sem nome mostra erro de validação', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await configureProfilePage(wails);

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Abre editor de novo perfil
    await page.keyboard.press('Control+n');

    const editor = page.locator('.profiles-editor');
    await expect(editor).toBeVisible({ timeout: 5_000 });

    // Limpa o campo de nome
    const nameInput = editor.locator('input').first();
    await expect(nameInput).toBeVisible();
    await nameInput.fill('');

    // Tenta salvar sem nome; o aviso da validação é o sinal de sincronização.
    const saveBtn = editor.locator('button', { hasText: /salvar|save/i });
    await expect(saveBtn.first()).toBeVisible();
    await saveBtn.first().click();
    await expect(page.locator('.toast__message').filter({ hasText: /nome é obrigatório|name is required/i })).toBeVisible();
    const log = await wails.getCallLog();
    expect(log.some((call) => call.fn === 'BeginUICommand')).toBe(false);
    expect(log.some((call) => call.fn === 'PreparePageMutationCommand' || call.fn === 'CreateProfile' || call.fn === 'UpdateProfile')).toBe(false);
  });

  test('erro do backend ao criar perfil é exibido', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await configureProfilePage(wails);
    await configureProfileMutation(wails, 'profiles.create');

    await wails.waitForApp();
    await page.goto('/#/profiles');
    await page.waitForSelector('.profiles-page', { timeout: 10_000 });

    // Simula recusa na preparação transacional, antes do handoff/efeito.
    await wails.setError('PreparePageMutationCommand', 'Nome de perfil já existe');

    await page.keyboard.press('Control+n');

    const editor = page.locator('.profiles-editor');
    await expect(editor).toBeVisible({ timeout: 5_000 });

    const nameInput = editor.locator('input').first();
    await expect(nameInput).toBeVisible();
    await nameInput.fill('Perfil Duplicado');

    const saveBtn = editor.locator('button', { hasText: /salvar|save/i });
    await expect(saveBtn.first()).toBeVisible();
    await saveBtn.first().click();
    await page.waitForFunction((ticket) => window.__wailsMock.getCallLog().some(
      (call: { fn: string; args: unknown[] }) => call.fn === 'PreparePageMutationCommand' && call.args[0] === ticket,
    ), profileCommandTicket, { timeout: 5_000 });
    const log = await wails.getCallLog();
    expect(log.filter((call) => call.fn === 'BeginUICommand' && call.args[0] === 'profiles.create')).toHaveLength(1);
    expectProfileMutationPreparation(log, { targetId: '', profileName: 'Perfil Duplicado' });
    expect(log.some((call) => call.fn === 'TakeUICommand' || call.fn === 'CommitWorkspaceTabCommand')).toBe(false);
    expect(log.some((call) => call.fn === 'CreateProfile')).toBe(false);
    await expect(page.locator('.toast__message').filter({ hasText: /^(erro|error)$/i })).toBeVisible();
  });

  test('campo name é obrigatório no editor', async ({ page, wails }) => {
    await wails.setResponse('GetProfiles', [defaultProfile]);
    await wails.setResponse('GetActiveProfileSlug', 'default');
    await configureProfilePage(wails, { ...profileCommandTarget, name: defaultProfile.name });

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
