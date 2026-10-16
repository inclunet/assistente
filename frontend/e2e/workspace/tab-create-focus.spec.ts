import { test, expect } from '../fixtures';

const now = new Date().toISOString();
const sourceConversationId = '01970a9e-1000-7000-8000-000000000001';
const createdConversationId = '';
const ticket = 'e2e-tab-create-chat-ticket';
const invocationId = '01970a9e-1000-7000-8000-000000000003';
const handoffId = 'e2e-tab-create-chat-handoff';

test('criar chat pelo menu Nova aba foca o textarea da aba recém-criada', async ({ page, wails }) => {
  const initialWorkspace = {
    id: 'ws-1',
    name: 'Workspace',
    snapshot_epoch: 'e2e-tab-create-focus-epoch',
    snapshot_sequence: '1',
    profile: '',
    created_at: now,
    last_used: now,
    tabs: {
      active: 'tab-source-chat',
      items: [{
        id: 'tab-source-chat',
        type: 'chat',
        conversation_id: sourceConversationId,
        title: 'Conversa origem',
        position: 0,
      }],
    },
  };

  await wails.setResponse('GetActiveWorkspace', initialWorkspace);
  await wails.setResponse('ListCommands', [{
    id: 'workspace.tab.chat.create',
    name: 'Chat',
    available: true,
  }]);
  await wails.waitForApp();

  await expect(page.locator(
    '.ws-content__panel[data-tab-id="tab-source-chat"] .chat-input__textarea',
  )).toBeVisible();

  await page.evaluate(({ initialWorkspace, now, sourceConversationId, createdConversationId, ticket, invocationId, handoffId }) => {
    type Snapshot = typeof initialWorkspace;
    let workspace: Snapshot = structuredClone(initialWorkspace);
    let status: 'pending' | 'taken' | 'succeeded' = 'pending';
    const state = {
      emittedSnapshots: [] as Snapshot[],
      observedSnapshots: [] as Snapshot[],
      resultReads: [] as string[],
      releaseCommit: null as (() => void) | null,
    };
    (window as Window & { __tabCreateFocusE2EState?: typeof state }).__tabCreateFocusE2EState = state;

    (window.runtime as unknown as { EventsOn: (event: string, callback: (snapshot: unknown) => void) => unknown })
      .EventsOn('workspace:tab_added', (snapshot) => state.observedSnapshots.push(snapshot as Snapshot));

    window.__wailsMock.setResponse('GetActiveWorkspace', () => structuredClone(workspace));
    window.__wailsMock.setResponse('GetConversationInfo', (conversationId: string) => ({
      id: conversationId,
      title: conversationId === createdConversationId ? 'Nova conversa criada' : 'Conversa origem',
      created_at: now,
      updated_at: now,
      messages: [],
      message_count: 0,
    }));
    window.__wailsMock.setResponse('BeginUICommand', (commandId: string) => {
      if (commandId !== 'workspace.tab.chat.create' || status !== 'pending') {
        throw new Error(`unexpected-tab-create-command:${commandId}`);
      }
      return { ticket, invocationId, commandId };
    });
    window.__wailsMock.setResponse('TakeUICommand', (receivedTicket: string) => {
      if (receivedTicket !== ticket || status !== 'pending') throw new Error('tab-create-command-not-takeable');
      status = 'taken';
      return { ticket, invocationId, commandId: 'workspace.tab.chat.create', handoffId };
    });
    window.__wailsMock.setResponse('CommitWorkspaceTabCommand', (receivedTicket: string, receivedHandoffId: string) => {
      if (receivedTicket !== ticket || receivedHandoffId !== handoffId || status !== 'taken') {
        throw new Error('tab-create-command-commit-mismatch');
      }
      workspace = {
        ...workspace,
        snapshot_sequence: '2',
        tabs: {
          active: 'tab-created-chat',
          items: [
            ...workspace.tabs.items,
            {
              id: 'tab-created-chat',
              type: 'chat',
              conversation_id: createdConversationId,
              title: 'Nova conversa criada',
              position: 1,
            },
          ],
        },
      };
      const committed = structuredClone(workspace);
      state.emittedSnapshots.push(committed);
      window.__wailsMock.emit('workspace:tab_added', committed);
      return new Promise<void>((resolve) => {
        state.releaseCommit = () => {
          status = 'succeeded';
          state.releaseCommit = null;
          resolve();
        };
      });
    });
    window.__wailsMock.setResponse('GetUICommandResult', (receivedTicket: string) => {
      if (receivedTicket !== ticket || status !== 'succeeded') throw new Error('tab-create-result-read-too-early');
      state.resultReads.push(receivedTicket);
      return { invocationId, status };
    });
  }, { initialWorkspace, now, sourceConversationId, createdConversationId, ticket, invocationId, handoffId });

  await page.getByRole('button', { name: /new tab/i }).click();
  const menu = page.getByRole('menu', { name: /new tab/i });
  await expect(menu).toBeVisible();
  await menu.getByRole('menuitem', { name: /chat/i }).click();

  const createdPanel = page.locator('.ws-content__panel[data-tab-id="tab-created-chat"]');
  await expect(createdPanel).toBeVisible();
  const textarea = createdPanel.locator('.chat-input__textarea');
  const newTabButton = page.getByRole('button', { name: 'New tab' });
  await page.waitForFunction(() => (
    (window as Window & { __tabCreateFocusE2EState?: { observedSnapshots: unknown[] } })
      .__tabCreateFocusE2EState?.observedSnapshots.length === 1
  ));
  await expect(newTabButton).toBeFocused();
  await expect(textarea).not.toBeFocused();
  await expect.poll(async () => (await wails.getCallLog()).filter((call) => call.fn === 'GetUICommandResult').length)
    .toBe(0);

  await page.evaluate(() => {
    const state = (window as Window & { __tabCreateFocusE2EState?: { releaseCommit: (() => void) | null } })
      .__tabCreateFocusE2EState;
    if (!state?.releaseCommit) throw new Error('tab-create-commit-gate-not-ready');
    state.releaseCommit();
  });

  await expect.poll(
    () => page.evaluate(() => {
      const active = document.activeElement as HTMLElement | null;
      return active ? `${active.tagName.toLowerCase()}.${String(active.className)}[${active.getAttribute('aria-label') ?? ''}]` : 'none';
    }),
    { timeout: 5_000, message: 'Foco final deve estar no textarea default do chat recém-criado' },
  ).toContain('chat-input__textarea');
  await expect(textarea).toBeFocused();

  const log = await wails.getCallLog();
  const protocol = ['BeginUICommand', 'TakeUICommand', 'CommitWorkspaceTabCommand', 'GetUICommandResult'];
  const protocolCalls = protocol.map((fn) => log.findIndex((call) => call.fn === fn));
  expect(protocolCalls.every((index) => index >= 0)).toBe(true);
  expect(protocolCalls).toEqual([...protocolCalls].sort((a, b) => a - b));
  expect(log.find((call) => call.fn === 'BeginUICommand')?.args).toEqual(['workspace.tab.chat.create']);
  expect(log.find((call) => call.fn === 'TakeUICommand')?.args).toEqual([ticket]);
  expect(log.find((call) => call.fn === 'CommitWorkspaceTabCommand')?.args).toEqual([ticket, handoffId]);
  expect(log.find((call) => call.fn === 'GetUICommandResult')?.args).toEqual([ticket]);

  const state = await page.evaluate(() => (
    (window as Window & { __tabCreateFocusE2EState?: {
      emittedSnapshots: Array<{ snapshot_sequence: string; tabs: { active: string } }>;
      observedSnapshots: Array<{ snapshot_sequence: string; tabs: { active: string } }>;
      resultReads: string[];
    } }).__tabCreateFocusE2EState
  ));
  expect(state?.emittedSnapshots).toHaveLength(1);
  expect(state?.observedSnapshots).toHaveLength(1);
  expect(state?.emittedSnapshots[0]).toMatchObject({
    snapshot_sequence: '2',
    tabs: { active: 'tab-created-chat' },
  });
  expect(state?.resultReads).toEqual([ticket]);
});

type KeyboardOrigin = 'textarea' | 'tablist';
type KeyboardChoice = 'letter' | 'arrow-down' | 'arrow-up';

async function createChatFromKeyboard(
  page: import('@playwright/test').Page,
  wails: import('../fixtures').WailsMock,
  origin: KeyboardOrigin,
  choice: KeyboardChoice,
): Promise<void> {
  const initialWorkspace = {
    id: 'ws-1',
    name: 'Workspace',
    snapshot_epoch: 'e2e-tab-create-focus-keyboard-epoch',
    snapshot_sequence: '1',
    profile: '',
    created_at: now,
    last_used: now,
    tabs: {
      active: 'tab-source-chat',
      items: [{
        id: 'tab-source-chat',
        type: 'chat',
        conversation_id: sourceConversationId,
        title: 'Conversa origem',
        position: 0,
      }],
    },
  };
  const keyboardMapGeneration = `e2e-tab-create-focus-${origin}-${choice}`;
  await wails.setResponse('GetActiveWorkspace', initialWorkspace);
  await wails.setResponse('GetLocalCommandKeyboardMap', {
    generation: keyboardMapGeneration,
    ownerId: 'user-e2e',
    sessionId: 'session-e2e',
    workspaceId: 'ws-1',
    bindings: ['chat', 'editor'].map((type) => ({
      shortcut: { version: 2, steps: [
        { code: 'KeyN', modifiers: ['Control'] },
        { code: type === 'chat' ? 'KeyC' : 'KeyE', modifiers: [] },
      ] },
      commandId: `workspace.tab.${type}.create`,
      handler: 'contextual',
    })),
    localPaletteCommands: [],
  });
  await wails.setResponse('ListCommands', [
    { id: 'workspace.tab.editor.create', name: 'Editor', available: true },
    { id: 'workspace.tab.chat.create', name: 'Chat', available: true },
  ]);
  await wails.setResponse('BeginUICommand', {
    ticket,
    invocationId,
    commandId: 'workspace.tab.chat.create',
  });
  await wails.setResponse('BeginLocalCommandUIKey', {
    ticket,
    invocationId,
    commandId: 'workspace.tab.chat.create',
  });
  await wails.setResponse('TakeUICommand', {
    ticket,
    invocationId,
    commandId: 'workspace.tab.chat.create',
    handoffId,
  });
  await wails.setResponse('GetUICommandResult', {
    invocationId,
    commandId: 'workspace.tab.chat.create',
    status: 'succeeded',
  });
  await wails.waitForApp();

  const sourceTextarea = page.locator('.ws-content__panel[data-tab-id="tab-source-chat"] .chat-input__textarea');
  await expect(sourceTextarea).toBeVisible();
  const sourceTab = page.getByRole('tab', { name: /Conversa origem/ });
  if (origin === 'textarea') {
    await sourceTextarea.click();
    await expect(sourceTextarea).toBeFocused();
  } else {
    await sourceTab.click();
    await expect(sourceTab).toBeFocused();
  }

  // O transporte Wails confirma a criação emitindo o snapshot que o backend
  // realmente produziria antes do primeiro envio: conversation_id vazio.
  await page.evaluate(({ initialWorkspace, ticket, handoffId }) => {
    const committedWorkspace = structuredClone(initialWorkspace);
    committedWorkspace.snapshot_sequence = '2';
    committedWorkspace.tabs = {
      active: 'tab-created-chat',
      items: [...committedWorkspace.tabs.items, {
        id: 'tab-created-chat',
        type: 'chat',
        conversation_id: '',
        title: 'Nova conversa',
        position: 1,
      }],
    };
    window.__wailsMock.setResponse('CommitWorkspaceTabCommand', (receivedTicket: string, receivedHandoffId: string) => {
      if (receivedTicket !== ticket || receivedHandoffId !== handoffId) throw new Error('tab-create-handoff-mismatch');
      window.__wailsMock.emit('workspace:tab_added', committedWorkspace);
    });
  }, { initialWorkspace, ticket, handoffId });

  await page.keyboard.press('Control+n');
  const menu = page.getByRole('menu', { name: /new tab/i });
  await expect(menu).toBeVisible();
  if (choice === 'letter') {
    await page.keyboard.press('c');
  } else {
    await page.keyboard.press(choice === 'arrow-down' ? 'ArrowDown' : 'ArrowUp');
    await page.keyboard.press('Enter');
  }

  const createdPanel = page.locator('.ws-content__panel[data-tab-id="tab-created-chat"]');
  await expect(createdPanel).toBeVisible();
  await expect.poll(async () => (await wails.getCallLog()).some((call) => (
    call.fn === 'GetUICommandResult' && call.args[0] === ticket
  ))).toBe(true);
  const log = await wails.getCallLog();
  const localBegin = log.find((call) => call.fn === 'BeginLocalCommandUIKey');
  const contextualBegin = log.find((call) => call.fn === 'BeginUICommand');
  expect(Boolean(localBegin || contextualBegin)).toBe(true);
  if (choice === 'letter') {
    expect(localBegin?.args[0]).toBe(keyboardMapGeneration);
    expect(contextualBegin).toBeUndefined();
  } else {
    expect(contextualBegin?.args).toEqual(['workspace.tab.chat.create']);
  }
  expect(log.some((call) => call.fn === 'TakeUICommand' && call.args[0] === ticket)).toBe(true);
  expect(log.some((call) => call.fn === 'CommitWorkspaceTabCommand' && call.args[0] === ticket && call.args[1] === handoffId)).toBe(true);
  expect(log.some((call) => call.fn === 'GetUICommandResult' && call.args[0] === ticket)).toBe(true);

  const createdTextarea = createdPanel.locator('.chat-input__textarea');
  await expect(createdTextarea).toBeFocused();
  // Observation window requested to catch autofocus/lifecycle races after command completion.
  await page.waitForTimeout(1_000);
  await expect(createdTextarea).toBeFocused();
}

for (const scenario of [
  { origin: 'textarea', choice: 'letter' },
  { origin: 'textarea', choice: 'arrow-down' },
  { origin: 'tablist', choice: 'letter' },
  { origin: 'tablist', choice: 'arrow-up' },
] as const) {
  test(`Ctrl+N via ${scenario.origin}, escolhe Chat por ${scenario.choice}, foca e mantém textarea`, async ({ page, wails }) => {
    await createChatFromKeyboard(page, wails, scenario.origin, scenario.choice);
  });
}
