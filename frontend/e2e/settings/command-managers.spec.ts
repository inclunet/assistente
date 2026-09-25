import { test, expect } from '../fixtures';

test.use({ locale: 'pt-BR' });

const configuration = {
  scope: 'global', revision: 1, fingerprint: 'manager-fixture', keyboardOperational: true,
  layers: [{ id: 'personal-layer', name: 'Camada pessoal', description: 'Atalhos de teste', builtin: false,
    enabled: true, active: true, manualReady: true, manualActive: true }],
  commands: [{ id: 'workspace.tab.next', name: 'Próxima aba', description: 'Selecionar a próxima aba',
    allowedSources: ['keyboard.local', 'palette'] }],
  bindings: [{ id: 'personal-binding', layerId: 'personal-layer', commandId: 'workspace.tab.next',
    triggerType: 'keyboard.local', triggerSpec: '{"version":1,"code":"KeyY","modifiers":["Control"]}',
    enabled: true, customized: false, readOnly: false, reviewStatus: 'active', effect: 'execute' }],
  rules: [{ id: 'personal-rule', layerId: 'personal-layer', mode: 'manual', lifecycle: 'persistent',
    enabled: true, manualActive: true, reviewStatus: 'active', condition: { version: 1, clauses: [] } }],
};

test.beforeEach(async ({ page, wails }) => {
  await wails.setResponse('GetCommandSettingsForScope', configuration);
  await wails.waitForApp();
  await page.goto('/#/settings/commands');
  await expect(page.getByRole('grid', { name: 'Camadas', exact: true })).toBeVisible();
});

test('gerenciadores separados mantêm altura útil e retornam foco após fechar editor e gerenciador', async ({ page }) => {
  const layers = page.getByRole('grid', { name: 'Camadas', exact: true });
  const toolbar = page.getByRole('toolbar', { name: 'Comandos e acionadores', exact: true });
  await expect(toolbar.getByRole('button', { name: 'Nova camada', exact: true })).toBeVisible();
  await expect(toolbar.getByRole('button', { name: 'Editar camada', exact: true })).toBeEnabled();
  await expect(toolbar.getByRole('checkbox')).not.toBeChecked();
  await expect(page.locator('.command-settings-page [role="grid"]')).toHaveCount(1);
  await page.getByRole('button', { name: 'Configurações da camada', exact: true }).click();
  await page.getByRole('menuitem', { name: 'Comandos e acionadores', exact: true }).click();
  const manager = page.getByRole('dialog', { name: 'Comandos e acionadores', exact: true });
  const grid = manager.getByRole('grid');
  await expect(grid).toBeVisible();
  await expect(page.getByRole('dialog', { name: 'Regras de ativação', exact: true })).toHaveCount(0);
  await expect.poll(async () => (await grid.locator('.datagrid-body').boundingBox())?.height ?? 0).toBeGreaterThan(100);
  await manager.getByRole('button', { name: 'Novo acionador', exact: true }).click();
  const editor = page.getByRole('dialog', { name: 'Comando e acionador', exact: true });
  await expect(editor).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(editor).toHaveCount(0);
  await expect(manager).toBeVisible();
  await expect.poll(() => grid.evaluate(element => element.contains(document.activeElement))).toBe(true);
  await page.keyboard.press('Escape');
  await expect(manager).toHaveCount(0);
  await expect.poll(() => layers.evaluate(element => element.contains(document.activeElement))).toBe(true);

  await page.getByRole('button', { name: 'Configurações da camada', exact: true }).click();
  await page.getByRole('menuitem', { name: 'Regras de ativação', exact: true }).click();
  const rules = page.getByRole('dialog', { name: 'Regras de ativação', exact: true });
  await expect(rules.getByRole('grid')).toBeVisible();
  await expect(manager).toHaveCount(0);
  await rules.getByRole('button', { name: 'Nova regra', exact: true }).click();
  await expect(page.getByRole('dialog', { name: 'Regra de ativação', exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect.poll(() => rules.getByRole('grid').evaluate(element => element.contains(document.activeElement))).toBe(true);
  await page.keyboard.press('Escape');
  await expect.poll(() => layers.evaluate(element => element.contains(document.activeElement))).toBe(true);
});

test('toolbar edita a camada selecionada e alcança o menu por teclado', async ({ page, wails }) => {
  const toolbar = page.getByRole('toolbar', { name: 'Comandos e acionadores', exact: true });
  const create = toolbar.getByRole('button', { name: 'Nova camada', exact: true });
  const edit = toolbar.getByRole('button', { name: 'Editar camada', exact: true });
  await create.focus();
  await page.keyboard.press('ArrowRight');
  await expect(edit).toBeFocused();
  await page.keyboard.press('Enter');
  const editor = page.getByRole('dialog', { name: 'Camada de comandos', exact: true });
  await expect(editor.getByRole('textbox', { name: 'Nome', exact: true })).toHaveValue('Camada pessoal');
  await page.keyboard.press('Escape');
  await expect(editor).toHaveCount(0);
  await edit.focus();
  await page.keyboard.press('End');
  const consent = toolbar.getByRole('checkbox');
  await expect(consent).toBeFocused();
  await expect(consent).not.toBeChecked();
  await page.keyboard.press('ArrowLeft');
  await expect(toolbar.getByRole('button', { name: 'Configurações da camada', exact: true })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('menuitem', { name: 'Regras de ativação', exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('menuitem', { name: 'Regras de ativação', exact: true })).toHaveCount(0);
  await expect(toolbar.getByRole('button', { name: 'Configurações da camada', exact: true })).toBeFocused();
  await page.keyboard.press('ArrowRight');
  await expect(consent).toBeFocused();
  await page.keyboard.press('Space');
  await expect(consent).toBeChecked();
  await expect(page.getByRole('textbox', { name: 'Convite de uso único', exact: true })).toHaveCount(0);
  expect((await wails.getCallLog()).filter(call => call.fn === 'BeginExternalUIConnection')).toHaveLength(0);
  await expect(toolbar.locator('button[tabindex="0"], input[type="checkbox"][tabindex="0"]')).toHaveCount(1);
  await page.keyboard.press('Home');
  await expect(create).toBeFocused();
  await expect(consent).toBeChecked();
});

test('gerenciador vazio retorna ao botão de criação ao cancelar o primeiro acionador', async ({ page, wails }) => {
  await wails.setResponse('GetCommandSettingsForScope', { ...configuration, bindings: [], rules: [] });
  await wails.emit('command:keyboard-map-changed');
  await expect(page.getByText('Acionadores: 0', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Configurações da camada', exact: true }).click();
  await page.getByRole('menuitem', { name: 'Comandos e acionadores', exact: true }).click();
  const manager = page.getByRole('dialog', { name: 'Comandos e acionadores', exact: true });
  const create = manager.getByRole('button', { name: 'Novo acionador', exact: true });
  await create.click();
  await expect(page.getByRole('dialog', { name: 'Comando e acionador', exact: true })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(create).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(manager).toHaveCount(0);
});

test('falha de recarga fecha o gerenciador e oferece recuperação com foco em Recarregar', async ({ page, wails }) => {
  await page.getByRole('button', { name: 'Configurações da camada', exact: true }).click();
  await page.getByRole('menuitem', { name: 'Comandos e acionadores', exact: true }).click();
  const manager = page.getByRole('dialog', { name: 'Comandos e acionadores', exact: true });
  await expect(manager).toBeVisible();
  await wails.setError('GetCommandSettingsForScope', 'Falha de recarga simulada');
  await wails.emit('command:keyboard-map-changed');
  await expect(manager).toHaveCount(0);
  const reload = page.getByRole('button', { name: 'Recarregar', exact: true });
  await expect(reload).toBeFocused();
  await wails.setResponse('GetCommandSettingsForScope', configuration);
  await reload.click();
  await expect(page.getByRole('grid', { name: 'Camadas', exact: true }).getByText('Camada pessoal', { exact: true })).toBeVisible();
  await expect(manager).toHaveCount(0);
});
