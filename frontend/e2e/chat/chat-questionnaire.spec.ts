import { test, expect } from '../fixtures';

test.describe('Chat — questionnaire response flow', () => {
  for (const response of ['submit', 'cancel'] as const) {
    test(`resposta tardia (${response}) não fecha nem rouba foco da próxima decisão`, async ({ page, wails }) => {
      await wails.waitForApp();
      await expect(page.locator('.chat-input__textarea')).toBeFocused();
      await page.evaluate(() => {
        let finish: (() => void) | undefined;
        window.__wailsMock.setResponse('RespondQuestionnaire', () => new Promise<void>((resolve) => {
          finish = resolve;
        }));
        // Porta do backend simulado: a UI continua usando RespondQuestionnaire.
        window.addEventListener('e2e:finish-questionnaire-response', () => finish?.(), { once: true });
      });
      const decision = (id: string, title: string) => ({
        id, title, kind: 'decision', description: 'Confirme a ação.', questions: [], allowCancel: true,
        actions: [{ id: 'apply', label: 'Confirmar', primary: true }],
      });
      await wails.emit('tool:questionnaire', decision('decision-a', 'Decisão A'));
      const first = page.getByRole('alertdialog', { name: 'Decisão A' });
      await expect(first).toBeVisible();
      if (response === 'submit') await first.getByRole('button', { name: 'Confirmar', exact: true }).click();
      else await page.keyboard.press('Escape');
      await expect.poll(async () => (await wails.getCallLog()).filter(call => call.fn === 'RespondQuestionnaire').length).toBe(1);

      await wails.emit('tool:questionnaire', decision('decision-b', 'Decisão B'));
      const second = page.getByRole('alertdialog', { name: 'Decisão B' });
      await expect(second).toBeVisible();
      const currentAction = second.getByRole('button', { name: 'Confirmar', exact: true });
      await currentAction.focus();
      await expect(currentAction).toBeFocused();
      // Registra qualquer fuga de foco, inclusive uma restauração transitória
      // que o focus trap pudesse corrigir antes da asserção final.
      await second.evaluate((dialog) => {
        const onFocus = (event: FocusEvent) => {
          if (!dialog.contains(event.target as Node)) dialog.setAttribute('data-e2e-focus-escaped', 'true');
        };
        document.addEventListener('focusin', onFocus, true);
      });
      await page.evaluate(async () => {
        window.dispatchEvent(new Event('e2e:finish-questionnaire-response'));
        await new Promise<void>(resolve => requestAnimationFrame(() => requestAnimationFrame(() => resolve())));
      });
      await expect(second).toBeVisible();
      await expect(currentAction).toBeFocused();
      await expect(second).not.toHaveAttribute('data-e2e-focus-escaped', 'true');
      const calls = (await wails.getCallLog()).filter(call => call.fn === 'RespondQuestionnaire');
      expect(calls).toHaveLength(1);
      expect(calls[0].args[0]).toBe('decision-a');
      expect(calls[0].args[2]).toBe(response === 'cancel');
    });
  }

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
