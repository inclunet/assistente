import { test, expect } from '../fixtures';
import { pauseRAF, resumeRAF } from '../helpers/pauseRaf';

/**
 * Testes de navegação por teclado na lista de abas (tablist).
 *
 * Padrão ARIA tabs com roving tabindex (modo "auto"):
 * - ArrowLeft / ArrowUp → aba anterior (ativa imediatamente)
 * - ArrowRight / ArrowDown → próxima aba (ativa imediatamente)
 * - Home → primeira aba
 * - End → última aba
 * - Delete → fecha aba
 */

function multiTabWorkspace() {
  const now = new Date().toISOString();
  return {
    id: 'ws-1',
    name: 'Workspace',
    profile: '',
    created_at: now,
    last_used: now,
    tabs: {
      active: 'tab-1',
      items: [
        { id: 'tab-1', type: 'chat', conversation_id: 1, title: 'Aba 1', position: 0 },
        { id: 'tab-2', type: 'chat', conversation_id: 2, title: 'Aba 2', position: 1 },
        { id: 'tab-3', type: 'chat', conversation_id: 3, title: 'Aba 3', position: 2 },
      ],
    },
  };
}

test.describe('Tab list — navegação por setas', () => {
  test('ArrowRight ativa a próxima aba e atualiza roving tabindex', async ({ page, wails }) => {
    const ws = multiTabWorkspace();
    await wails.setResponse('GetActiveWorkspace', ws);
    await wails.setResponse('SetActiveWorkspaceTab', undefined);
    await wails.setResponse('EnsureConversation', {
      id: 2, title: 'Aba 2',
      created_at: ws.created_at, updated_at: ws.created_at,
      messages: [], message_count: 0,
    });
    await wails.waitForApp();
    await pauseRAF(page);

    // Foca na primeira aba
    const firstTab = page.locator('.ws-tabs [role="tab"]').first();
    await firstTab.focus();
    await expect(firstTab).toHaveAttribute('aria-selected', 'true');

    // ArrowRight → ativa a segunda aba
    await firstTab.press('ArrowRight');
    const secondTab = page.locator('.ws-tabs [role="tab"]').nth(1);
    await resumeRAF(page);
    await expect(secondTab).toHaveAttribute('aria-selected', 'true', { timeout: 3_000 });
    await expect.poll(async () => secondTab.evaluate((el) => (el as HTMLButtonElement).tabIndex)).toBe(0);
    await expect.poll(async () => firstTab.evaluate((el) => (el as HTMLButtonElement).tabIndex)).toBe(-1);
  });

  test('ArrowLeft ativa a aba anterior e atualiza roving tabindex', async ({ page, wails }) => {
    const ws = multiTabWorkspace();
    ws.tabs.active = 'tab-2';
    await wails.setResponse('GetActiveWorkspace', ws);
    await wails.setResponse('SetActiveWorkspaceTab', undefined);
    await wails.setResponse('EnsureConversation', {
      id: 1, title: 'Aba 1',
      created_at: ws.created_at, updated_at: ws.created_at,
      messages: [], message_count: 0,
    });
    await wails.waitForApp();
    await pauseRAF(page);

    const secondTab = page.locator('.ws-tabs [role="tab"]').nth(1);
    await secondTab.focus();
    await expect(secondTab).toHaveAttribute('aria-selected', 'true');

    await secondTab.press('ArrowLeft');
    const firstTab = page.locator('.ws-tabs [role="tab"]').first();
    await resumeRAF(page);
    await expect(firstTab).toHaveAttribute('aria-selected', 'true', { timeout: 3_000 });
    await expect.poll(async () => firstTab.evaluate((el) => (el as HTMLButtonElement).tabIndex)).toBe(0);
    await expect.poll(async () => secondTab.evaluate((el) => (el as HTMLButtonElement).tabIndex)).toBe(-1);
  });

  test('Home e End atualizam a aba ativa e o roving tabindex', async ({ page, wails }) => {
    const ws = multiTabWorkspace();
    ws.tabs.active = 'tab-2';
    await wails.setResponse('GetActiveWorkspace', ws);
    await wails.setResponse('SetActiveWorkspaceTab', undefined);
    await wails.setResponse('EnsureConversation', {
      id: 1, title: 'Aba 1',
      created_at: ws.created_at, updated_at: ws.created_at,
      messages: [], message_count: 0,
    });
    await wails.waitForApp();
    await pauseRAF(page);

    // Foca a segunda aba
    const secondTab = page.locator('.ws-tabs [role="tab"]').nth(1);
    await secondTab.focus();
    await expect(secondTab).toHaveAttribute('aria-selected', 'true');

    // Home → primeira aba
    await secondTab.press('Home');
    const firstTab = page.locator('.ws-tabs [role="tab"]').first();
    await resumeRAF(page);
    await expect(firstTab).toHaveAttribute('aria-selected', 'true', { timeout: 3_000 });
    await expect.poll(async () => firstTab.evaluate((el) => (el as HTMLButtonElement).tabIndex)).toBe(0);
    await expect.poll(async () => secondTab.evaluate((el) => (el as HTMLButtonElement).tabIndex)).toBe(-1);

    // End → última aba
    await firstTab.press('End');
    const lastTab = page.locator('.ws-tabs [role="tab"]').last();
    await expect(lastTab).toHaveAttribute('aria-selected', 'true', { timeout: 3_000 });
    await expect.poll(async () => lastTab.evaluate((el) => (el as HTMLButtonElement).tabIndex)).toBe(0);
    await expect.poll(async () => firstTab.evaluate((el) => (el as HTMLButtonElement).tabIndex)).toBe(-1);
  });

  test('Delete fecha a aba focada', async ({ page, wails }) => {
    const ws = multiTabWorkspace();
    await wails.setResponse('GetActiveWorkspace', ws);
    await wails.setResponse('SetActiveWorkspaceTab', undefined);
    // Após fechar, retorna workspace com 2 abas
    const wsAfterClose = {
      ...ws,
      tabs: {
        active: 'tab-1',
        items: ws.tabs.items.slice(0, 2),
      },
    };
    await wails.setResponse('RemoveWorkspaceTab', wsAfterClose);
    await wails.waitForApp();

    // Deve ter 3 abas antes
    const tabs = page.locator('.ws-tabs [role="tab"]');
    await expect(tabs).toHaveCount(3);

    // Foca a terceira aba e pressiona Delete
    const thirdTab = tabs.nth(2);
    await thirdTab.focus();
    await thirdTab.press('Delete');

    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some(
        (c: { fn: string }) => c.fn === 'RemoveWorkspaceTab',
      );
    }, { timeout: 5_000 });

    // Deve ter 2 abas após
    await expect(tabs).toHaveCount(2, { timeout: 5_000 });
  });

  test('abas têm role="tab" e a ativa tem aria-selected="true"', async ({ page, wails }) => {
    const ws = multiTabWorkspace();
    await wails.setResponse('GetActiveWorkspace', ws);
    await wails.waitForApp();

    const tabs = page.locator('.ws-tabs [role="tab"]');
    await expect(tabs).toHaveCount(3);

    // A primeira aba deve estar selecionada (tab-1 é a ativa)
    await expect(tabs.first()).toHaveAttribute('aria-selected', 'true');
    await expect(tabs.nth(1)).toHaveAttribute('aria-selected', 'false');
    await expect(tabs.nth(2)).toHaveAttribute('aria-selected', 'false');
  });
});
