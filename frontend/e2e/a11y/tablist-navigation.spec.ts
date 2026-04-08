import { test, expect } from '../fixtures';

declare global {
  interface Window {
    __origRAF?: typeof requestAnimationFrame;
    __rafQueue?: FrameRequestCallback[];
  }
}

async function pauseRAF(page: import('@playwright/test').Page) {
  await page.evaluate(() => {
    window.__origRAF = window.requestAnimationFrame;
    window.requestAnimationFrame = (cb: FrameRequestCallback) => {
      window.__rafQueue = window.__rafQueue || [];
      window.__rafQueue.push(cb);
      return 0;
    };
  });
}

async function resumeRAF(page: import('@playwright/test').Page) {
  await page.evaluate(() => {
    if (window.__origRAF) {
      window.requestAnimationFrame = window.__origRAF;
      const queue = window.__rafQueue || [];
      window.__rafQueue = [];
      for (const cb of queue) {
        try { cb(performance.now()); } catch (_) { /* ignore */ }
      }
    }
  });
}

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
  test('ArrowRight move foco e ativa a próxima aba', async ({ page, wails }) => {
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
    await page.keyboard.press('ArrowRight');
    const secondTab = page.locator('.ws-tabs [role="tab"]').nth(1);
    await expect(secondTab).toBeFocused({ timeout: 3_000 });
    await resumeRAF(page);
  });

  test('ArrowLeft move foco para a aba anterior', async ({ page, wails }) => {
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

    await page.keyboard.press('ArrowLeft');
    const firstTab = page.locator('.ws-tabs [role="tab"]').first();
    await expect(firstTab).toBeFocused({ timeout: 3_000 });
    await resumeRAF(page);
  });

  test('Home foca a primeira aba, End foca a última', async ({ page, wails }) => {
    const ws = multiTabWorkspace();
    await wails.setResponse('GetActiveWorkspace', ws);
    await wails.setResponse('SetActiveWorkspaceTab', undefined);
    await wails.waitForApp();
    await pauseRAF(page);

    // Foca a segunda aba
    const secondTab = page.locator('.ws-tabs [role="tab"]').nth(1);
    await secondTab.focus();

    // Home → primeira aba
    await page.keyboard.press('Home');
    const firstTab = page.locator('.ws-tabs [role="tab"]').first();
    await expect(firstTab).toBeFocused({ timeout: 3_000 });

    // End → última aba
    await page.keyboard.press('End');
    const lastTab = page.locator('.ws-tabs [role="tab"]').last();
    await expect(lastTab).toBeFocused({ timeout: 3_000 });
    await resumeRAF(page);
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
    await tabs.nth(2).focus();
    await page.keyboard.press('Delete');

    // Deve ter 2 abas após
    await expect(tabs).toHaveCount(2, { timeout: 3_000 });
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
