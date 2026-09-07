import { test, expect } from '../fixtures';

test.describe('Allowlist de paths — gestão e decisão', () => {
  test.describe.configure({ timeout: 60_000 });

  test('cria allow persistente pelo formulário de gestão', async ({ page, wails }) => {
    await wails.setResponse('GetPathAllowlist', []);
    await wails.setResponse('AddPathAllowlistEntry', undefined);
    await wails.waitForApp();

    await page.goto('/#/settings');
    await page.getByRole('tab', { name: /path allowlist|allowlist de paths/i }).click();
    const form = page.locator('.path-allowlist-page__form');
    await expect(form).toBeVisible({ timeout: 10_000 });

    await page.locator('#path-rule-path').fill('C:\\dados\\manual.md');
    await page.locator('#path-rule-operation').fill('read');
    await page.locator('#path-rule-effect').selectOption('allow');
    await page.locator('#path-rule-scope').selectOption('workspace');
    await form.locator('button[type="submit"]').click();

    await expect.poll(async () => {
      const call = (await wails.getCallLog()).find((item) => item.fn === 'AddPathAllowlistEntry');
      return call?.args;
    }).toEqual(['C:\\dados\\manual.md', 'file', 'read', 'allow', 'workspace', '']);
  });

  test('nega e lembra por botão, enquanto ESC apenas cancela e restaura foco', async ({ page, wails }) => {
    await wails.setResponse('RespondQuestionnaire', undefined);
    await wails.waitForApp();
    const trigger = page.getByRole('button').first();
    await trigger.focus();

    const payload = {
      id: 'fstrust-e2e',
      kind: 'decision',
      title: { fallback: 'Autorizar path externo' },
      description: { fallback: 'Escolha permitir ou negar e lembrar.' },
      body: 'path: C:\\dados\\manual.md\noperation: read',
      allowCancel: true,
      actions: [
        { id: 'once', label: { fallback: 'Permitir esta tentativa' }, variant: 'primary', primary: true },
        { id: 'deny-workspace', label: { fallback: 'Negar neste workspace' }, variant: 'danger' },
        { id: 'deny', label: { fallback: 'Negar esta tentativa' }, variant: 'outline' },
      ],
      questions: [],
    };

    await wails.emit('tool:questionnaire', payload);
    const dialog = page.getByRole('alertdialog', { name: 'Autorizar path externo' });
    await expect(dialog).toBeVisible();
    const actions = dialog.getByRole('button');
    await expect(actions.nth(1)).toHaveText('Permitir esta tentativa');
    await expect(actions.nth(2)).toHaveText('Negar neste workspace');
    await dialog.getByRole('button', { name: 'Negar neste workspace' }).click();

    await expect.poll(async () => {
      const call = (await wails.getCallLog()).find((item) => item.fn === 'RespondQuestionnaire');
      return call?.args;
    }).toEqual(['fstrust-e2e', { actionId: 'deny-workspace' }, false]);
    await expect(dialog).not.toBeVisible();

    await wails.emit('tool:questionnaire', { ...payload, id: 'fstrust-cancel-e2e' });
    await expect(dialog).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(dialog).not.toBeVisible();
    await expect(trigger).toBeFocused();
    await expect.poll(async () => {
      const call = (await wails.getCallLog()).find((item) => item.fn === 'RespondQuestionnaire'
        && item.args[0] === 'fstrust-cancel-e2e');
      return call?.args;
    }).toEqual(['fstrust-cancel-e2e', {}, true]);
  });
});
