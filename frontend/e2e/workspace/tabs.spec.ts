import { test, expect } from '../fixtures';

const now = new Date().toISOString();

test.describe('Abas do workspace — renderização', () => {
  test('lista de abas está visível', async ({ page, wails }) => {
    await wails.waitForApp();

    const tabList = page.locator('.ws-tabs__list');
    await expect(tabList).toBeVisible();
  });

  test('aba ativa tem classe --active', async ({ page, wails }) => {
    await wails.waitForApp();

    const activeTab = page.locator('.ws-tabs__tab--active');
    await expect(activeTab).toBeVisible();
  });

  test('aba mostra título da conversa', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', {
      id: 'ws-1',
      name: 'Workspace',
      snapshot_epoch: 'e2e-workspace-epoch',
      snapshot_sequence: '1',
      profile: '',
      created_at: now,
      last_used: now,
      tabs: {
        active: 'tab-1',
        items: [
          {
            id: 'tab-1',
            type: 'chat',
            conversation_id: '01970a9e-0001-7000-8000-000000000001',
            title: 'Minha conversa',
            position: 0,
          },
        ],
      },
    });

    await wails.waitForApp();

    const tabTitle = page.locator('.ws-tabs__tab-title');
    await expect(tabTitle).toContainText('Minha conversa');
  });
});

test.describe('Abas do workspace — múltiplas abas', () => {
  const workspaceWithTabs = {
    id: 'ws-1',
    name: 'Workspace',
    snapshot_epoch: 'e2e-workspace-epoch',
    snapshot_sequence: '1',
    profile: '',
    created_at: now,
    last_used: now,
    tabs: {
      active: 'tab-1',
      items: [
        {
          id: 'tab-1',
          type: 'chat',
          conversation_id: '01970a9e-0001-7000-8000-000000000001',
          title: 'Conversa 1',
          position: 0,
        },
        {
          id: 'tab-2',
          type: 'chat',
          conversation_id: '01970a9e-0002-7000-8000-000000000002',
          title: 'Conversa 2',
          position: 1,
        },
        {
          id: 'tab-3',
          type: 'chat',
          conversation_id: '01970a9e-0003-7000-8000-000000000003',
          title: 'Conversa 3',
          position: 2,
        },
      ],
    },
  };

  const workspaceAfterSelectingTab2 = {
    ...workspaceWithTabs,
    snapshot_sequence: '2',
    tabs: {
      ...workspaceWithTabs.tabs,
      active: 'tab-2',
      items: workspaceWithTabs.tabs.items.map(tab => tab.id === 'tab-2'
        ? { ...tab, title: 'Conversa 2 confirmada' } : tab),
    },
  };

  test('renderiza múltiplas abas', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', workspaceWithTabs);
    await wails.waitForApp();

    const tabs = page.locator('.ws-tabs__tab');
    await expect(tabs).toHaveCount(3);
  });

  test('clicar em outra aba usa o RPC escopado e mantém a seleção após a confirmação', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', workspaceWithTabs);
    await wails.setResponse('SetActiveWorkspaceTabForWorkspace', workspaceAfterSelectingTab2);
    await wails.waitForApp();

    // Clica na segunda aba
    const secondTab = page.locator('.ws-tabs__tab').nth(1);
    await secondTab.click();

    await page.waitForFunction(() => window.__wailsMock.getCallLog().some(c => c.fn === 'SetActiveWorkspaceTabForWorkspace'));
    const log = await wails.getCallLog();
    const setCalls = log.filter(c => c.fn === 'SetActiveWorkspaceTabForWorkspace');
    expect(setCalls).toHaveLength(1);
    expect(setCalls[0].args).toEqual(['ws-1', 'tab-2']);
    await expect(secondTab).toHaveAttribute('aria-selected', 'true');
    // Este título só existe no ACK: a seleção otimista sozinha não basta.
    await expect(secondTab).toContainText('Conversa 2 confirmada');
  });

  test('botão de fechar aba está visível quando há múltiplas abas', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', workspaceWithTabs);
    await wails.waitForApp();

    // Hover na aba para mostrar botão de fechar
    const firstTab = page.locator('.ws-tabs__tab-wrapper').first();
    await firstTab.hover();

    const closeBtn = firstTab.locator('.ws-tabs__tab-close');
    await expect(closeBtn).toBeVisible();
  });

  test('abas têm role=tab para acessibilidade', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', workspaceWithTabs);
    await wails.waitForApp();

    const tabButtons = page.locator('button[role="tab"]');
    const count = await tabButtons.count();
    expect(count).toBeGreaterThanOrEqual(3);
  });
});
