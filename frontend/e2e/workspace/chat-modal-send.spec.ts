import { test, expect } from '../fixtures';

const now = new Date().toISOString();

test('chat modal do terminal cria conversa e envia mensagem', async ({ page, wails }) => {
  await wails.setResponse('GetActiveWorkspace', {
    id: 'ws-1',
    name: 'Workspace',
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
  await wails.setResponse('CreateConversation', {
    id: '01970a9e-0011-7000-8000-000000000011',
    title: 'Nova conversa',
    created_at: now,
    updated_at: now,
    messages: [],
    message_count: 0,
  });
  await wails.setResponse('GetConversationInfo', {
    id: '01970a9e-0011-7000-8000-000000000011',
    title: 'Nova conversa',
    created_at: now,
    updated_at: now,
    messages: [],
    message_count: 0,
  });
  await wails.setResponse('GetMessages', []);
  await wails.setResponse('UpdateWorkspaceTab', undefined);
  await wails.setResponse('SendMessage', '01970a9e-0101-7000-8000-000000000101');

  await wails.waitForApp();

  await page.getByRole('button', { name: /chat/i }).click();
  const chatModal = page.getByRole('dialog', { name: /chat/i });
  await expect(chatModal).toBeVisible();

  const textarea = chatModal.getByRole('combobox', { name: /message/i });
  await textarea.fill('teste terminal');
  await textarea.press('Enter');

  await page.waitForFunction(() => {
    return window.__wailsMock.getCallLog().some(
      (c: { fn: string; args: unknown[] }) => c.fn === 'SendMessage',
    );
  });

  const log = await wails.getCallLog();
  const createCalls = log.filter((c) => c.fn === 'CreateConversation');
  const sendCalls = log.filter((c) => c.fn === 'SendMessage');

  expect(createCalls.length).toBe(1);
  expect(sendCalls.length).toBe(1);
  expect(sendCalls[0].args[0]).toBe('01970a9e-0011-7000-8000-000000000011');
  expect(sendCalls[0].args[1]).toBe('teste terminal');
});
