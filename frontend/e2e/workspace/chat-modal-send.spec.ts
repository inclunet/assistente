import { test, expect } from '../fixtures';

const now = new Date().toISOString();
const conversationId = '01970a9e-0011-7000-8000-000000000011';
const commandTicket = 'e2e-workspace-chat-open-ticket';
const commandInvocationId = '01970a9e-0012-7000-8000-000000000012';
const commandHandoffId = 'e2e-workspace-chat-open-handoff';

type WorkspaceSnapshotEvidence = {
  id: string;
  snapshot_epoch: string;
  snapshot_sequence: string;
  tabs: {
    active: string;
    items: Array<{
      id: string;
      type?: string;
      conversation_id?: string;
      title?: string;
      position?: number;
      state?: { sessionId?: string };
    }>;
  };
};

test('workspace.chat.open cria conversa para terminal, vincula e envia mensagem ao alvo', async ({ page, wails }) => {
  await wails.setResponse('GetActiveWorkspace', {
    id: 'ws-1',
    name: 'Workspace',
    snapshot_epoch: 'e2e-workspace-epoch',
    snapshot_sequence: '1',
    profile: '',
    created_at: now,
    last_used: now,
    tabs: {
      active: 'tab-term-1',
      items: [
        {
          id: 'tab-term-1',
          type: 'terminal',
          conversation_id: '',
          title: 'Terminal',
          position: 0,
          state: { sessionId: 'sess-1' },
        },
      ],
    },
  });
  await wails.setResponse('GetLocalCommandKeyboardMap', {
    generation: 'e2e-command-map-workspace-chat',
    ownerId: 'user-e2e',
    sessionId: 'session-e2e',
    workspaceId: 'ws-1',
    bindings: [],
    localPaletteCommands: ['workspace.chat.open'],
  });
  await wails.setResponse('ListTerminalSessions', [
    {
      id: 'sess-1',
      name: 'Terminal',
      cwd: 'C:/tmp',
      createdAt: now,
      updatedAt: now,
    },
  ]);
  await wails.setResponse('GetTerminalHistory', [
    {
      id: 'entry-1',
      command: 'pwd',
      output: 'C:/tmp',
      exitCode: 0,
      startedAt: now,
      endedAt: now,
      source: 'user',
    },
  ]);
  await wails.setResponse('GetConversationInfo', {
    id: conversationId,
    title: 'Nova conversa',
    created_at: now,
    updated_at: now,
    messages: [],
    message_count: 0,
  });
  await wails.setResponse('GetMessages', []);
  await wails.waitForApp();

  // O callback local modela o efeito de workspace.chat.open: grava a conversa
  // no snapshot, avança a sequência e publica o evento somente após o commit.
  // O protocolo de chat.message.send permanece no adaptador compartilhado.
  await page.evaluate(({ conversationId, now, commandTicket, commandInvocationId, commandHandoffId }) => {
    const initialWorkspace = {
      id: 'ws-1',
      name: 'Workspace',
      snapshot_epoch: 'e2e-workspace-epoch',
      snapshot_sequence: '1',
      profile: '',
      created_at: now,
      last_used: now,
      tabs: {
        active: 'tab-term-1',
        items: [{
          id: 'tab-term-1',
          type: 'terminal',
          conversation_id: '',
          title: 'Terminal',
          position: 0,
          state: { sessionId: 'sess-1' },
        }],
      },
    };
    let workspace = structuredClone(initialWorkspace);
    const invocation = {
      ticket: commandTicket,
      invocationId: commandInvocationId,
      commandId: 'workspace.chat.open',
      handoffId: commandHandoffId,
      status: 'pending' as 'pending' | 'taken' | 'committing' | 'succeeded',
    };
    const state = {
      emittedBindings: [] as Array<{ id: string; snapshot_epoch: string; snapshot_sequence: string; tabs: typeof workspace.tabs }>,
      observedBindings: [] as Array<{ id: string; snapshot_epoch: string; snapshot_sequence: string; tabs: typeof workspace.tabs }>,
      observedCommandStatuses: [] as string[],
      resultReads: [] as Array<{ ticket: string; status: string }>,
      readSnapshots: [] as Array<{ id: string; snapshot_epoch: string; snapshot_sequence: string; tabs: typeof workspace.tabs }>,
    };
    (window as Window & { __workspaceChatE2EState?: typeof state }).__workspaceChatE2EState = state;
    (window.runtime as unknown as { EventsOn: (event: string, callback: (snapshot: unknown) => void) => unknown })
      .EventsOn('workspace:conversation_bound', (snapshot) => {
        state.observedBindings.push(snapshot as typeof state.emittedBindings[number]);
        state.observedCommandStatuses.push(invocation.status);
      });

    window.__wailsMock.setResponse('BeginUICommand', (commandId: string) => {
      if (commandId !== invocation.commandId || invocation.status !== 'pending') {
        throw new Error(`e2e-command-not-admitted:${commandId}`);
      }
      return { ticket: invocation.ticket, invocationId: invocation.invocationId, commandId };
    });
    window.__wailsMock.setResponse('TakeUICommand', (ticket: string) => {
      if (ticket !== invocation.ticket || invocation.status !== 'pending') throw new Error('e2e-command-not-takeable');
      invocation.status = 'taken';
      return {
        ticket: invocation.ticket,
        invocationId: invocation.invocationId,
        commandId: invocation.commandId,
        handoffId: invocation.handoffId,
      };
    });
    window.__wailsMock.setResponse('GetUICommandResult', (ticket: string) => {
      if (ticket !== invocation.ticket || invocation.status !== 'succeeded') throw new Error('e2e-command-not-succeeded');
      state.resultReads.push({ ticket, status: invocation.status });
      return { invocationId: invocation.invocationId, status: invocation.status };
    });
    window.__wailsMock.setResponse('GetActiveWorkspace', () => {
      const snapshot = structuredClone(workspace);
      state.readSnapshots.push(snapshot);
      return snapshot;
    });
    window.__wailsMock.setResponse('CommitWorkspaceTabCommand', (ticket: string, handoffId: string) => {
      const validInvocation = ticket === invocation.ticket && invocation.commandId === 'workspace.chat.open' &&
        invocation.status === 'taken' && invocation.handoffId === handoffId;
      const activeTab = workspace.tabs.items.find((tab) => tab.id === workspace.tabs.active);
      if (!validInvocation || !activeTab || activeTab.type !== 'terminal' ||
          activeTab.conversation_id !== '' || workspace.snapshot_sequence !== '1') {
        throw new Error('e2e-workspace-chat-commit-mismatch');
      }
      workspace = {
        ...workspace,
        snapshot_sequence: '2',
        tabs: {
          ...workspace.tabs,
          items: workspace.tabs.items.map((tab) => tab.id === activeTab.id
            ? { ...tab, conversation_id: conversationId }
            : tab),
        },
      };
      const committed = structuredClone(workspace);
      state.emittedBindings.push(committed);
      invocation.status = 'committing';
      window.__wailsMock.emit('workspace:conversation_bound', committed);
      invocation.status = 'succeeded';
    });
  }, { conversationId, now, commandTicket, commandInvocationId, commandHandoffId });

  await page.getByRole('button', { name: /chat/i }).click();
  const chatModal = page.getByRole('dialog', { name: /chat/i });
  await expect(chatModal).toBeVisible();

  // A abertura já terminou e consultou o resultado; restaura os adaptadores
  // compartilhados antes do envio para que chat.message.send use seu ledger.
  await wails.clearResponse('BeginUICommand');
  await wails.clearResponse('TakeUICommand');
  await wails.clearResponse('GetUICommandResult');
  await wails.clearResponse('CommitWorkspaceTabCommand');

  const textarea = chatModal.getByRole('combobox', { name: /message/i });
  await textarea.fill('teste terminal');
  await textarea.press('Enter');

  await page.waitForFunction(() => {
    const calls = window.__wailsMock.getCallLog();
    return calls.some((c: { fn: string }) => c.fn === 'SendMessage') &&
      calls.filter((c: { fn: string }) => c.fn === 'GetUICommandResult').length >= 2;
  });

  const log = await wails.getCallLog();
  const beginCalls = log.filter((c) => c.fn === 'BeginUICommand');
  const takeCalls = log.filter((c) => c.fn === 'TakeUICommand');
  const commitCalls = log.filter((c) => c.fn === 'CommitWorkspaceTabCommand');
  const resultCalls = log.filter((c) => c.fn === 'GetUICommandResult');
  const workspaceReads = log.filter((c) => c.fn === 'GetActiveWorkspace');
  const sendCalls = log.filter((c) => c.fn === 'SendMessage');
  const state = await page.evaluate(() => (
    (window as Window & { __workspaceChatE2EState?: {
      emittedBindings: WorkspaceSnapshotEvidence[];
      observedBindings: WorkspaceSnapshotEvidence[];
      observedCommandStatuses: string[];
      resultReads: Array<{ ticket: string; status: string }>;
      readSnapshots: WorkspaceSnapshotEvidence[];
    } })
      .__workspaceChatE2EState
  ));

  expect(beginCalls.map((call) => call.args)).toEqual([['workspace.chat.open'], ['chat.message.send']]);
  expect(takeCalls[0].args).toEqual([commandTicket]);
  expect(takeCalls).toHaveLength(2);
  expect(commitCalls).toHaveLength(1);
  expect(commitCalls[0].args).toEqual([commandTicket, commandHandoffId]);
  expect(resultCalls).toHaveLength(2);
  expect(resultCalls[0].args).toEqual([commandTicket]);
  expect(workspaceReads.length).toBeGreaterThanOrEqual(2);
  expect(state?.emittedBindings).toHaveLength(1);
  expect(state?.emittedBindings[0]).toMatchObject({
    id: 'ws-1',
    snapshot_epoch: 'e2e-workspace-epoch',
    snapshot_sequence: '2',
    tabs: expect.objectContaining({ active: 'tab-term-1' }),
  });
  expect(state?.emittedBindings[0].tabs.items).toHaveLength(1);
  expect(state?.emittedBindings[0].tabs.items).toMatchObject([{
    id: 'tab-term-1',
    type: 'terminal',
    conversation_id: conversationId,
    title: 'Terminal',
    position: 0,
    state: { sessionId: 'sess-1' },
  }]);
  expect(state?.observedBindings).toEqual(state?.emittedBindings);
  expect(state?.observedCommandStatuses).toEqual(['committing']);
  expect(state?.resultReads).toEqual([{ ticket: commandTicket, status: 'succeeded' }]);
  const reboundRead = state?.readSnapshots.find((snapshot) => snapshot.snapshot_sequence === '2');
  expect(reboundRead).toBeDefined();
  expect(reboundRead).toMatchObject({ id: 'ws-1', snapshot_epoch: 'e2e-workspace-epoch' });
  expect(reboundRead?.tabs.active).toBe('tab-term-1');
  expect(reboundRead?.tabs.items).toHaveLength(1);
  expect(reboundRead?.tabs.items).toMatchObject([{
    id: 'tab-term-1',
    type: 'terminal',
    conversation_id: conversationId,
    title: 'Terminal',
    position: 0,
    state: { sessionId: 'sess-1' },
  }]);
  expect(log.some((call) => call.fn === 'CreateConversation')).toBe(false);
  expect(log.some((call) => call.fn === 'UpdateWorkspaceTab')).toBe(false);
  expect(sendCalls.length).toBe(1);
  expect(sendCalls[0].args[0]).toBe(conversationId);
  expect(sendCalls[0].args[1]).toBe('teste terminal');
  const sendProof = sendCalls[0].args[3] as { command?: { ticket?: string; handoffId?: string } };
  expect(sendProof.command?.ticket).toMatch(/^e2e-chat-ticket-\d+$/);
  expect(sendProof.command?.handoffId).toMatch(/^e2e-chat-handoff-\d+$/);
  expect(takeCalls[1].args).toEqual([sendProof.command?.ticket]);
  expect(resultCalls[1].args).toEqual([sendProof.command?.ticket]);
  expect(log.filter((call) => call.fn === 'CompleteUICommand' && call.args[0] === sendProof.command?.ticket)).toHaveLength(0);
});
