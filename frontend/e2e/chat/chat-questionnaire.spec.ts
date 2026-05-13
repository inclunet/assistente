import { test, expect } from '../fixtures';

test.describe('Chat — questionnaire response flow', () => {
  test('mantém questionário aberto quando RespondQuestionnaire falha', async ({ page, wails }) => {
    await wails.waitForApp();
    await wails.setError('RespondQuestionnaire', 'backend unavailable');

    await wails.emit('tool:questionnaire', {
      id: 'q-1',
      title: 'Confirmar comando',
      description: 'Autorize a execução',
      questions: [
        {
          id: 'approval',
          type: 'text',
          prompt: 'Digite ok para confirmar',
          required: true,
        },
      ],
      allowCancel: true,
      submitLabel: 'Enviar',
    });

    const dialog = page.getByRole('dialog', { name: 'Confirmar comando' });
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    await page.locator('#question-approval').fill('ok');
    await page.getByRole('button', { name: 'Enviar' }).click();

    await page.waitForFunction(
      () => window.__wailsMock.getCallLog().some((c) => c.fn === 'RespondQuestionnaire'),
      undefined,
      { timeout: 5_000 },
    );

    await expect(dialog).toBeVisible({ timeout: 5_000 });
  });

  test('fecha questionário quando RespondQuestionnaire resolve', async ({ page, wails }) => {
    await wails.waitForApp();
    await wails.setResponse('RespondQuestionnaire', undefined);

    await wails.emit('tool:questionnaire', {
      id: 'q-2',
      title: 'Confirmar comando',
      description: 'Autorize a execução',
      questions: [
        {
          id: 'approval',
          type: 'text',
          prompt: 'Digite ok para confirmar',
          required: true,
        },
      ],
      allowCancel: true,
      submitLabel: 'Enviar',
    });

    const dialog = page.getByRole('dialog', { name: 'Confirmar comando' });
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    await page.locator('#question-approval').fill('ok');
    await page.getByRole('button', { name: 'Enviar' }).click();

    await page.waitForFunction(
      () => window.__wailsMock.getCallLog().some((c) => c.fn === 'RespondQuestionnaire'),
      undefined,
      { timeout: 5_000 },
    );

    await expect(dialog).not.toBeVisible({ timeout: 5_000 });
  });
});
