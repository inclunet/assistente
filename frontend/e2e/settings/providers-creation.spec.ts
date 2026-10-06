import { test, expect } from '../fixtures';
import { readFileSync } from 'node:fs';
const wire = JSON.parse(readFileSync(new URL('../../src/lib/__fixtures__/commandKeyboardMap.json', import.meta.url), 'utf8')) as Record<string, unknown>;

test.use({ locale: 'pt-BR' });
test('Novo provedor compartilha menu por botão e Ctrl+N, separa API e restaura foco', async ({ page, wails }) => {
  await wails.setResponse('GetLocalCommandKeyboardMap', { ...wire,
    generation: 'e2e-local-command-map-v1', ownerId: 'user-e2e',
    sessionId: 'session-e2e', workspaceId: 'ws-1' });
  await wails.setResponse('ListCommands', [
    { id: 'providers.create.open', name: 'Novo provedor', available: true },
    { id: 'providers.chatgpt.create.open', name: 'Conectar conta ChatGPT', available: true },
    { id: 'providers.acp.create.open', name: 'Serviço ACP', available: true },
    { id: 'providers.api.create.open', name: 'Provedor API', available: true },
  ]);
  await wails.waitForApp();
  await page.goto('/#/settings');
  const trigger = page.getByRole('button', { name: /^Novo provedor(?:, Ctrl\+N)?$/ });
  await expect(trigger).toBeVisible();
  for (const activation of ['click', 'Enter', 'Space', 'Control+n']) {
    await trigger.focus();
    if (activation === 'click') await trigger.click();
    else await page.keyboard.press(activation);
    await expect(page.getByRole('menuitem', { name: 'Conectar conta ChatGPT', exact: true })).toBeFocused();
    await expect(page.getByRole('menuitem')).toHaveCount(3);
    await page.keyboard.press('Escape');
    await expect(trigger).toBeFocused();
  }
  await trigger.click();
  await page.keyboard.press('Escape');
  await expect(trigger).toBeFocused();
  await trigger.click();
  await expect(page.getByRole('menuitem', { name: 'Conectar conta ChatGPT', exact: true })).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(page.getByRole('menu')).toHaveCount(0);
  await expect(trigger).not.toBeFocused();
  await trigger.click();
  await expect(page.getByRole('menuitem', { name: 'Conectar conta ChatGPT', exact: true })).toBeFocused();
  await page.keyboard.press('Shift+Tab');
  await expect(page.getByRole('menu')).toHaveCount(0);
  await expect(page.getByRole('textbox', { name: 'Buscar provedores...' })).toBeFocused();
  await trigger.focus();
  await page.keyboard.press('Control+n');
  await expect(page.getByRole('menuitem', { name: 'Conectar conta ChatGPT', exact: true })).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await expect(page.getByRole('menuitem', { name: 'Serviço ACP', exact: true })).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Enter');
  await expect(page.locator('.provider-form')).toBeVisible();
  await expect(page.locator('.provider-form option[value="acp"]')).toHaveCount(0);
  await expect(page.getByRole('menu')).toHaveCount(0);
  await page.keyboard.press('Control+n');
  await expect(page.getByRole('menu')).toHaveCount(0);
  await expect(page.locator('.provider-form')).toBeVisible();
  await wails.setResponse('GetCredentialForURL', null);
  await wails.setResponse('ListModelsRaw', ['test-model']);
  await page.getByLabel(/^Nome/).fill('API via ambiente');
  await page.getByRole('button', { name: 'Configurar credencial', exact: true }).click();
  await page.getByLabel('Fonte', { exact: true }).selectOption('env');
  await page.getByLabel('Nome da variável', { exact: true }).fill('PROVIDER_API_TOKEN');
  await page.getByRole('button', { name: 'Carregar modelos do provedor', exact: true }).click();
  await page.getByRole('button', { name: 'Criar', exact: true }).click();
  await expect(page.locator('.provider-form')).toHaveCount(0);
  const calls = await wails.getCallLog();
  const create = calls.find(call => call.fn.endsWith('CreateLLMProvider'));
  expect(create?.args[0]).toMatchObject({ name: 'API via ambiente', credential: {
    pattern: 'api.openai.com', type: 'bearer', source: 'env', sourceConfig: { env: 'PROVIDER_API_TOKEN' },
  }});
  expect(calls.some(call => call.fn.endsWith('UpsertCredential'))).toBe(false);

});


test('Ações da toolbar reutiliza o menu da linha e permite editar e duplicar por teclado', async ({ page, wails }) => {
  await wails.setResponse('GetLLMProvidersWithStatus', [{
    id: 'api-provider', name: 'API de teste', type: 'openai',
    base_url: 'https://api.openai.com/v1', credential_status: 'configured', auth_mode: 'required',
  }]);
  await wails.waitForApp();
  await page.goto('/#/settings/providers');
  const toolbar = page.locator('.providers-page').getByRole('toolbar');
  const actions = toolbar.getByRole('button', { name: 'Ações', exact: true });
  await expect(toolbar.getByRole('button')).toHaveCount(2);
  await page.getByRole('gridcell', { name: 'API de teste', exact: true }).click();
  await actions.focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('menuitem', { name: 'Editar', exact: true })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.locator('.provider-form')).toBeVisible();
  await expect(page.getByLabel(/^Nome/)).toHaveValue('API de teste');
  await page.getByRole('button', { name: 'Cancelar', exact: true }).click();
  await actions.focus();
  await page.keyboard.press('Space');
  await expect(page.getByRole('menuitem', { name: 'Editar', exact: true })).toBeFocused();
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('ArrowDown');
  await expect(page.getByRole('menuitem', { name: 'Duplicar', exact: true })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect.poll(async () => (await wails.getCallLog()).filter(call => call.fn.endsWith('CreateLLMProvider')).length).toBe(1);
  const create = (await wails.getCallLog()).find(call => call.fn.endsWith('CreateLLMProvider'));
  expect(create?.args[0]).toMatchObject({ credential_from_provider_id: 'api-provider' });
  await page.getByRole('textbox', { name: 'Buscar provedores...' }).fill('sem correspondência');
  await actions.click();
  await expect(page.getByRole('menuitem', { name: 'Editar', exact: true })).toBeDisabled();
  await expect(page.getByRole('menuitem', { name: 'Duplicar', exact: true })).toBeDisabled();
  await page.keyboard.press('Escape');
  await expect(actions).toBeFocused();
});
