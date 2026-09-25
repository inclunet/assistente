import { test, expect } from '../fixtures';

const now = new Date().toISOString();

const twoWorkspaces = [
  { id: 'ws-1', name: 'Principal', path: '', profile: '', tab_count: 3, is_active: true },
  { id: 'ws-2', name: 'Secundário', path: '', profile: '', tab_count: 1, is_active: false },
];

const activeWorkspace = {
  id: 'ws-1',
  name: 'Principal',
  snapshot_epoch: 'e2e-workspace-epoch',
  snapshot_sequence: '1',
  profile: '',
  created_at: now,
  last_used: now,
  tabs: {
    active: 'tab-1',
    items: [
      { id: 'tab-1', type: 'chat', conversation_id: '01970a9e-0001-7000-8000-000000000001', title: 'Conversa 1', position: 0 },
      { id: 'tab-2', type: 'chat', conversation_id: '01970a9e-0002-7000-8000-000000000002', title: 'Conversa 2', position: 1 },
    ],
  },
};

const workspaceAfterMove = {
  ...activeWorkspace,
  snapshot_sequence: '2',
  tabs: {
    active: 'tab-2',
    items: [{ ...activeWorkspace.tabs.items[1], position: 0 }],
  },
};

const workspaceAfterCloseOthers = {
  ...activeWorkspace,
  snapshot_sequence: '2',
  tabs: {
    active: 'tab-1',
    items: [activeWorkspace.tabs.items[0]],
  },
};

test.describe('Abas — context menu avançado', () => {
  test('right-click mostra opções do menu de contexto', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', activeWorkspace);
    await wails.setResponse('ListWorkspaces', twoWorkspaces);

    await wails.waitForApp();

    await expect(page.locator('button[role="tab"]')).toHaveCount(2);
    const firstTab = page.locator('button[role="tab"]').first();
    await expect(firstTab).toBeVisible();
    await firstTab.click({ button: 'right' });

    // Usa .first() pois pode haver submenu também
    const menu = page.locator('[role="menu"]').first();
    if (!(await menu.isVisible().catch(() => false))) {
      await firstTab.dispatchEvent('mousedown', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
      await firstTab.dispatchEvent('contextmenu', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
    }
    await expect(menu).toBeVisible({ timeout: 3_000 });
    await expect(page.locator('#close')).toBeVisible({ timeout: 3_000 });
    await expect(page.locator('#close-others')).toBeVisible({ timeout: 3_000 });
  });

  test('Move To submenu aparece com outros workspaces', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', activeWorkspace);
    await wails.setResponse('ListWorkspaces', twoWorkspaces);
    await wails.setResponse('MoveWorkspaceTabTo', workspaceAfterMove);

    await wails.waitForApp();

    await expect(page.locator('button[role="tab"]')).toHaveCount(2);
    const firstTab = page.locator('button[role="tab"]').first();
    await expect(firstTab).toBeVisible();
    await firstTab.dispatchEvent('mousedown', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
    await firstTab.dispatchEvent('contextmenu', { button: 2, buttons: 2, clientX: 16, clientY: 16 });

    const menu = page.locator('[role="menu"]').first();
    if (!(await menu.isVisible().catch(() => false))) {
      await firstTab.click({ button: 'right' });
    }
    await expect(menu).toBeVisible({ timeout: 3_000 });

    const moveToItem = page.locator('#move-to');
    await expect(moveToItem).toBeVisible({ timeout: 3_000 });
    await moveToItem.evaluate((element: HTMLButtonElement) => element.click());

    // O submenu deve mostrar outros workspaces
    const submenuItem = page.locator('#move-ws-2');
    await expect(submenuItem).toBeVisible({ timeout: 3_000 });
  });

  test('selecionar workspace no submenu chama MoveWorkspaceTabTo', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', activeWorkspace);
    await wails.setResponse('ListWorkspaces', twoWorkspaces);
    await wails.setResponse('MoveWorkspaceTabTo', workspaceAfterMove);

    await wails.waitForApp();

    await expect(page.locator('button[role="tab"]')).toHaveCount(2);
    const firstTab = page.locator('button[role="tab"]').first();
    await expect(firstTab).toBeVisible();
    await firstTab.click({ button: 'right' });

    const menu = page.locator('[role="menu"]').first();
    if (!(await menu.isVisible().catch(() => false))) {
      await firstTab.dispatchEvent('mousedown', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
      await firstTab.dispatchEvent('contextmenu', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
    }
    await expect(menu).toBeVisible({ timeout: 3_000 });

    await menu.locator('#move-to').evaluate((element: HTMLButtonElement) => element.click());

    const submenuItem = page.locator('#move-ws-2');
    await expect(submenuItem).toBeVisible({ timeout: 3_000 });
    await submenuItem.evaluate((element: HTMLButtonElement) => element.click());

    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some(
        (c: { fn: string }) => c.fn === 'MoveWorkspaceTabTo'
      );
    }, { timeout: 5_000 });

    const log = await wails.getCallLog();
    const moveCalls = log.filter(c => c.fn === 'MoveWorkspaceTabTo');
    expect(moveCalls.length).toBe(1);
    expect(moveCalls[0].args).toEqual(['tab-1', 'ws-2']);
    await expect(page.locator('button[role="tab"]')).toHaveCount(1);
    await expect(page.locator('button[role="tab"]')).toContainText('Conversa 2');
  });

  test('Close Others via context menu chama RemoveWorkspaceTab', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', activeWorkspace);
    await wails.setResponse('ListWorkspaces', twoWorkspaces);
    await wails.setResponse('RemoveWorkspaceTab', workspaceAfterCloseOthers);

    await wails.waitForApp();

    await expect(page.locator('button[role="tab"]')).toHaveCount(2);
    const firstTab = page.locator('button[role="tab"]').first();
    await expect(firstTab).toBeVisible();
    await firstTab.click({ button: 'right' });

    const menu = page.locator('[role="menu"]').first();
    if (!(await menu.isVisible().catch(() => false))) {
      await firstTab.dispatchEvent('mousedown', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
      await firstTab.dispatchEvent('contextmenu', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
    }
    await expect(menu).toBeVisible({ timeout: 3_000 });

    const closeOthers = menu.locator('#close-others');
    await expect(closeOthers).toHaveCount(1);
    await closeOthers.click();
    await expect(page.locator('button[role="tab"]')).toHaveCount(1);
    await expect(page.locator('button[role="tab"]')).toContainText('Conversa 1');
    const log = await wails.getCallLog();
    const removeCalls = log.filter(c => c.fn === 'RemoveWorkspaceTab');
    expect(removeCalls).toHaveLength(1);
    expect(removeCalls[0].args).toEqual(['tab-2']);
  });
});
