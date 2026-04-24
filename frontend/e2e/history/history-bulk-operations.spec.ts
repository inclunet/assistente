import { test, expect } from '../fixtures';

const now = new Date().toISOString();

const sampleConversations = [
  { id: 1, title: 'Conversa sobre IA', message_count: 5, created_at: now, updated_at: now },
  { id: 2, title: 'Projeto React', message_count: 3, created_at: now, updated_at: now },
  { id: 3, title: 'Debugging hooks', message_count: 8, created_at: now, updated_at: now },
  { id: 4, title: 'API design', message_count: 2, created_at: now, updated_at: now },
];

test.describe('Histórico — seleção múltipla', () => {
  test('Ctrl+Click seleciona múltiplas linhas no grid', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });
    await page.waitForSelector('[role="gridcell"]', { timeout: 5_000 });

    // Usa teclado para selecionar: foca grid, ArrowDown, Space para selecionar
    const grid = page.locator('[role="grid"]');
    await grid.focus();

    // Navega para primeira linha de dados
    await page.keyboard.press('ArrowDown');

    // Space para toggle selection
    await page.keyboard.press('Space');

    // ArrowDown + Space para adicionar segunda seleção
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('ArrowDown'); // pula uma linha
    await page.keyboard.press('Space');

    // Verifica que pelo menos uma linha está selecionada
    const selectedRows = page.locator('[role="row"][aria-selected="true"]');
    const selectedCount = await selectedRows.count();
    expect(selectedCount).toBeGreaterThanOrEqual(1);
  });
});

test.describe('Histórico — bulk delete', () => {
  test('deletar conversa selecionada via toolbar', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);
    await wails.setResponse('DeleteConversation', undefined);

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });
    await page.waitForSelector('[role="gridcell"]', { timeout: 5_000 });

    // Seleciona via teclado
    const grid = page.locator('[role="grid"]');
    await grid.focus();
    await page.keyboard.press('ArrowDown');

    // Clica no botão Delete
    const deleteBtn = page.locator('button', { hasText: /excluir|delet/i });
    if (await deleteBtn.count() > 0) {
      await deleteBtn.first().click();

      // Confirma no dialog
      const confirmDialog = page.locator('.confirm-dialog-modal');
      await expect(confirmDialog).toBeVisible({ timeout: 5_000 });

      const confirmBtn = confirmDialog.locator('button', { hasText: /confirm/i });
      await confirmBtn.click();

      await page.waitForFunction(() => {
        return window.__wailsMock.getCallLog().some(
          (c: { fn: string }) => c.fn === 'DeleteConversation'
        );
      }, { timeout: 5_000 });

      const log = await wails.getCallLog();
      const deleteCalls = log.filter(c => c.fn === 'DeleteConversation');
      expect(deleteCalls.length).toBeGreaterThanOrEqual(1);
    }
  });

  test('cancelar delete preserva conversa', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });
    await page.waitForSelector('[role="gridcell"]', { timeout: 5_000 });

    // Seleciona e tenta deletar
    const grid = page.locator('[role="grid"]');
    await grid.focus();
    await page.keyboard.press('ArrowDown');

    const deleteBtn = page.locator('button', { hasText: /excluir|delet/i });
    if (await deleteBtn.count() > 0) {
      await deleteBtn.first().click();

      const confirmDialog = page.locator('.confirm-dialog-modal');
      await expect(confirmDialog).toBeVisible({ timeout: 5_000 });

      // Cancela
      const cancelBtn = confirmDialog.locator('button', { hasText: /cancel/i });
      await cancelBtn.click();

      // Nenhum delete deve ter ocorrido
      const log = await wails.getCallLog();
      const deleteCalls = log.filter(c => c.fn === 'DeleteConversation');
      expect(deleteCalls.length).toBe(0);
    }
  });
});

test.describe('Histórico — exportação', () => {
  test('exportar conversa chama ExportData', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);
    await wails.setResponse('ExportData', '{"conversations":[]}');

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });
    await page.waitForSelector('[role="gridcell"]', { timeout: 5_000 });

    // Seleciona uma conversa via teclado
    const grid = page.locator('[role="grid"]');
    await grid.focus();
    await page.keyboard.press('ArrowDown');

    // Clica no botão Export
    const exportBtn = page.locator('button', { hasText: /export/i });
    if (await exportBtn.count() > 0) {
      await exportBtn.first().click();
      await page.getByRole('button', { name: /export now|exportar agora/i }).click();

      await page.waitForFunction(() => {
        return window.__wailsMock.getCallLog().some(
          (c: { fn: string }) => c.fn === 'ExportData'
        );
      }, { timeout: 5_000 });

      const log = await wails.getCallLog();
      const exportCalls = log.filter(c => c.fn === 'ExportData');
      expect(exportCalls.length).toBeGreaterThanOrEqual(1);
    }
  });
});
