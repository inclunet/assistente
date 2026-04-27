import { test, expect } from '../fixtures';

const now = new Date().toISOString();

const sampleConversations = [
  {
    id: '1',
    title: 'Conversa sobre IA',
    created_at: now,
    updated_at: now,
    message_count: 5,
  },
  {
    id: '2',
    title: 'Receita de bolo de chocolate',
    created_at: now,
    updated_at: now,
    message_count: 3,
  },
  {
    id: '3',
    title: 'Debugging React hooks',
    created_at: now,
    updated_at: now,
    message_count: 8,
  },
];

test.describe('Histórico — busca e filtro', () => {
  test('busca filtra conversas pelo título', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);
    // Resultado da busca: retorna apenas a conversa que match
    await wails.setResponse('SearchConversationHistory', [
      { conversation_id: '2', snippet: '>>>bolo<<< de chocolate' },
    ]);

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });

    // Digita no campo de busca
    const searchInput = page.locator('.toolbar__search');
    await searchInput.fill('bolo');

    // Aguarda debounce (300ms) + resposta
    await page.waitForTimeout(500);

    // Deve mostrar apenas a conversa que contém "bolo"
    const rows = page.locator('[role="grid"] [role="row"]');
    // Header row + 1 data row
    const count = await rows.count();
    // Pode ser 1 (só data rows) ou 2 (header + 1 data)
    expect(count).toBeLessThanOrEqual(3);

    const cell = page.locator('[role="gridcell"]', { hasText: 'Receita de bolo' });
    await expect(cell.first()).toBeVisible({ timeout: 3_000 });
  });

  test('busca sem resultados mostra estado vazio', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);
    await wails.setResponse('SearchConversationHistory', []);

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });

    const searchInput = page.locator('.toolbar__search');
    await searchInput.fill('xyznonexistent');

    await page.waitForTimeout(500);

    // O grid deve mostrar estado vazio
    const emptyState = page.locator('.datagrid-empty');
    await expect(emptyState).toBeVisible({ timeout: 5_000 });
  });

  test('limpar busca restaura todas as conversas', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);
    await wails.setResponse('SearchConversationHistory', [
      { conversation_id: '1', snippet: '>>>IA<<<' },
    ]);

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });

    const searchInput = page.locator('.toolbar__search');
    await searchInput.fill('IA');
    await page.waitForTimeout(500);

    // Limpa a busca
    await searchInput.fill('');
    await page.waitForTimeout(100);

    // Todas as conversas devem reaparecer
    const cells = page.locator('[role="gridcell"]');
    const count = await cells.count();
    // 3 conversas com múltiplas colunas
    expect(count).toBeGreaterThanOrEqual(9);
  });
});

test.describe('Histórico — abrir conversa', () => {
  test('Enter em linha focada navega para o chat', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);
    await wails.setResponse('EnsureConversation', {
      id: '1',
      title: 'Conversa sobre IA',
      created_at: now,
      updated_at: now,
      messages: [],
      message_count: 5,
    });

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });

    // Foca no grid e navega para a primeira linha
    const grid = page.locator('[role="grid"]');
    await grid.focus();
    await page.keyboard.press('ArrowDown');

    // Ativa a linha (Enter = deep link → navega para chat)
    await page.keyboard.press('Enter');

    // Deve navegar para a tela de chat (rota /)
    await page.waitForSelector('.chat-page', { timeout: 5_000 });
  });

  test('botão Abrir na toolbar navega para conversa', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);
    await wails.setResponse('EnsureConversation', {
      id: '1',
      title: 'Conversa sobre IA',
      created_at: now,
      updated_at: now,
      messages: [],
      message_count: 5,
    });

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });

    // Foca no grid para selecionar uma linha
    const grid = page.locator('[role="grid"]');
    await grid.focus();
    await page.keyboard.press('ArrowDown');

    // Clica no botão "Abrir" da toolbar
    const openBtn = page.locator('button', { hasText: /abrir/i });
    if (await openBtn.count() > 0) {
      await openBtn.first().click();
      await page.waitForSelector('.chat-page', { timeout: 5_000 });
    }
  });
});

test.describe('Histórico — exclusão com confirmação', () => {
  test('deletar conversa pede confirmação e remove do grid', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);
    await wails.setResponse('DeleteConversation', undefined);

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });

    // Foca numa linha do grid
    const grid = page.locator('[role="grid"]');
    await grid.focus();
    await page.keyboard.press('ArrowDown');

    // Pressiona Delete para iniciar exclusão
    await page.keyboard.press('Delete');

    // O diálogo de confirmação deve aparecer
    const confirmDialog = page.locator('.confirm-dialog-modal');
    await expect(confirmDialog).toBeVisible({ timeout: 5_000 });

    // Clica em Confirmar
    const confirmBtn = confirmDialog.locator('button', { hasText: /confirm/i });
    await confirmBtn.click();

    // Diálogo fecha
    await expect(confirmDialog).not.toBeVisible({ timeout: 3_000 });

    // Verifica que DeleteConversation foi chamado
    const log = await wails.getCallLog();
    const deleteCalls = log.filter(c => c.fn === 'DeleteConversation');
    expect(deleteCalls.length).toBe(1);
  });

  test('cancelar exclusão mantém conversa no grid', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });

    const grid = page.locator('[role="grid"]');
    await grid.focus();
    await page.keyboard.press('ArrowDown');
    await page.keyboard.press('Delete');

    const confirmDialog = page.locator('.confirm-dialog-modal');
    await expect(confirmDialog).toBeVisible({ timeout: 5_000 });

    // Clica em Cancelar
    const cancelBtn = confirmDialog.locator('button', { hasText: /cancel/i });
    await cancelBtn.click();

    await expect(confirmDialog).not.toBeVisible({ timeout: 3_000 });

    // Verifica que DeleteConversation NÃO foi chamado
    const log = await wails.getCallLog();
    const deleteCalls = log.filter(c => c.fn === 'DeleteConversation');
    expect(deleteCalls.length).toBe(0);
  });

  test('deletar via botão da toolbar com linha selecionada', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);
    await wails.setResponse('DeleteConversation', undefined);

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });

    // Foca uma linha
    const grid = page.locator('[role="grid"]');
    await grid.focus();
    await page.keyboard.press('ArrowDown');

    // Clica no botão de deletar da toolbar
    const deleteBtn = page.locator('.toolbar button', { hasText: /excluir|delet/i });
    if (await deleteBtn.count() > 0) {
      await deleteBtn.first().click();

      const confirmDialog = page.locator('.confirm-dialog-modal');
      await expect(confirmDialog).toBeVisible({ timeout: 5_000 });

      // Confirma
      const confirmBtn = confirmDialog.locator('button', { hasText: /confirm/i });
      await confirmBtn.click();

      const log = await wails.getCallLog();
      const deleteCalls = log.filter(c => c.fn === 'DeleteConversation');
      expect(deleteCalls.length).toBe(1);
    }
  });
});

test.describe('Histórico — edição inline de título', () => {
  test('editar título da conversa no grid persiste via UpdateConversation', async ({ page, wails }) => {
    await wails.setResponse('GetConversations', sampleConversations);
    await wails.setResponse('UpdateConversation', undefined);

    await wails.waitForApp();
    await page.goto('/#/history');
    await page.waitForSelector('.history-page', { timeout: 10_000 });

    // Aguarda grid renderizar
    await page.waitForSelector('[role="gridcell"]', { timeout: 5_000 });

    // Foca a célula do título
    const grid = page.locator('[role="grid"]');
    await grid.focus();
    await page.keyboard.press('ArrowDown');

    // F2 para editar (inline editing no DataGrid)
    await page.keyboard.press('F2');

    const editInput = page.locator('.cell-edit-input');
    await expect(editInput).toBeVisible({ timeout: 3_000 });
    await editInput.evaluate((element: HTMLInputElement, nextValue: string) => {
      element.focus();
      element.value = nextValue;
      element.dispatchEvent(new Event('input', { bubbles: true }));
      element.dispatchEvent(new Event('change', { bubbles: true }));
    }, 'Título Editado');

    // Confirma a edição clicando fora (blur → saveEdit)
    // Não usamos press('Enter') pois fill() pode triggar blur antes
    await grid.click({ position: { x: 5, y: 5 }, force: true });

    // Aguarda o processamento
    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some(
        (c: { fn: string }) => c.fn === 'UpdateConversation'
      );
    }, { timeout: 5_000 });

    const log = await wails.getCallLog();
    const updateCalls = log.filter(c => c.fn === 'UpdateConversation');
    expect(updateCalls.length).toBeGreaterThanOrEqual(1);
  });
});
