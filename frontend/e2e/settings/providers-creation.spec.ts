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
  await page.goto('/#/settings/providers');
  const trigger = page.getByRole('button', { name: /^Novo provedor(?:, Ctrl\+N)?$/ });
  await expect(trigger).toBeVisible();
  await trigger.click();
  await expect(page.getByRole('menuitem', { name: 'Conectar conta ChatGPT', exact: true })).toBeFocused();
  await expect(page.getByRole('menuitem')).toHaveCount(3);
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
});
