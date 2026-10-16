import { test, expect, type WailsMock } from '../fixtures';

const now = new Date().toISOString();
const conversationId = '01926b90-0000-7000-8000-000000000001';

async function setMessagesResponse(wails: WailsMock, nodes: unknown[]): Promise<void> {
  await wails.setResponse('GetMessages', nodes);
  await wails.setResponse('GetConversationInfo', {
    id: conversationId,
    title: 'Conversa com ferramentas',
    created_at: now,
    updated_at: now,
  });
  await wails.setResponse('GetConversationMessageWindow', {
    scope: 'conversation',
    conversationId,
    nodes,
    totalCount: nodes.length,
    startIndex: 0,
    endIndex: Math.max(0, nodes.length - 1),
    hasBefore: false,
    hasAfter: false,
  });
}

const messagesWithToolCalls = [
  {
    message: {
      id: '01926b90-0000-7000-8000-000000000010',
      conversationId: '01926b90-0000-7000-8000-000000000001',
      role: 'user',
      content: 'Como está o clima?',
      createdAt: now,
    },
    children: [],
  },
  {
    message: {
      id: '01926b90-0000-7000-8000-000000000011',
      conversationId: '01926b90-0000-7000-8000-000000000001',
      role: 'assistant',
      content: 'O clima hoje está ensolarado, 25°C.',
      createdAt: now,
      turnSegments: [{
        type: 'tool_calls',
        toolInvocations: [{
          invocationId: 'inv-1',
          callId: 'tc-1',
          name: 'search_web',
          status: 'succeeded',
          inputPreview: '{"bytes":23,"fields":["query"]}',
          outputPreview: '{"bytes":35,"fields":["results"]}',
          inputBytes: 23,
          outputBytes: 35,
          hasDetails: true,
          resultAvailability: 'available',
        }],
      }],
    },
    children: [],
  },
];

test.describe('Chat — tool calls (histórico)', () => {
  test('exibe a seção de ferramentas pela projeção leve', async ({ page, wails }) => {
    await setMessagesResponse(wails, messagesWithToolCalls);
    await wails.waitForApp();

    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    const toolSection = page.locator('.tool-calls-section');
    await expect(toolSection).toBeVisible();
  });

  test('cada tool call aparece diretamente como card amigável', async ({ page, wails }) => {
    await setMessagesResponse(wails, messagesWithToolCalls);
    await wails.waitForApp();

    await page.waitForSelector('.tool-calls-section', { timeout: 5_000 });

    const card = page.locator('.tool-calls-section__item');
    await expect(card).toHaveCount(1);
    await expect(card.locator('.tool-calls-section__intent')).toContainText(/Buscou na web|Searched the web|Buscó en la web/);
    await expect(card).not.toContainText('search_web');
  });

  test('card fica visível sem expansão', async ({ page, wails }) => {
    await setMessagesResponse(wails, messagesWithToolCalls);
    await wails.waitForApp();

    await page.waitForSelector('.tool-calls-section', { timeout: 5_000 });

    await expect(page.locator('.tool-calls-section__item')).toBeVisible();
    await expect(page.locator('.tool-calls-section__header')).toHaveCount(0);
  });

  test('card mantém a saída técnica somente nos detalhes', async ({ page, wails }) => {
    await setMessagesResponse(wails, messagesWithToolCalls);
    await wails.waitForApp();

    await page.waitForSelector('.tool-calls-section', { timeout: 5_000 });

    await expect(page.locator('.tool-calls-section__result-summary')).toHaveCount(0);
    await expect(page.locator('.tool-calls-section__item')).not.toContainText('"bytes"');
    await expect(page.getByRole('button', { name: /Detalhes técnicos|Technical details|Detalles técnicos/i })).toBeVisible();
  });

  test('card comunica o estado terminal da ferramenta', async ({ page, wails }) => {
    await setMessagesResponse(wails, messagesWithToolCalls);
    await wails.waitForApp();

    await page.waitForSelector('.tool-calls-section', { timeout: 5_000 });

    await expect(page.locator('.tool-calls-section__state')).toContainText(/Concluída|Completed|Completada/);
  });

  test('carrega payload integral somente ao pedir detalhes', async ({ page, wails }) => {
    await setMessagesResponse(wails, messagesWithToolCalls);
    await wails.setResponse('GetToolInvocationDetails', [{
      invocationId: 'inv-1',
      callId: 'tc-1',
      name: 'search_web',
      status: 'succeeded',
      attempt: 1,
      dryRun: false,
      input: '{"query":"clima hoje"}',
      output: '{"content":"Ensolarado, 25°C"}',
      metadata: '{}',
      resultAvailability: 'available',
      retryable: false,
      retryabilityKnown: true,
      queuedAt: now,
    }]);
    await wails.waitForApp();
    await page.getByRole('button', { name: /Detalhes técnicos|Technical details|Detalles técnicos/i }).click();

    await expect(page.locator('.tool-calls-section__args')).toContainText('clima hoje');
    await expect(page.locator('.tool-calls-section__result-content')).toContainText('Ensolarado');
  });

  test('conjunto e cards usam semântica de lista', async ({ page, wails }) => {
    await setMessagesResponse(wails, messagesWithToolCalls);
    await wails.waitForApp();

    await page.waitForSelector('.tool-calls-section', { timeout: 5_000 });

    await expect(page.locator('.tool-calls-section')).toHaveAttribute('role', 'list');
    await expect(page.locator('.tool-calls-section__item')).toHaveAttribute('role', 'listitem');
  });
});

test.describe('Chat — tool calls (streaming)', () => {
  test('exibe tool call em execução durante streaming', async ({ page, wails }) => {
    await setMessagesResponse(wails, [
      {
        message: {
          id: '01926b90-0000-7000-8000-000000000010',
          conversationId: '01926b90-0000-7000-8000-000000000001',
          role: 'user',
          content: 'Pesquise algo',
          createdAt: now,
        },
        children: [],
      },
    ]);
    await wails.setResponse('SendMessage', '01926b90-0000-7000-8000-100000000002');

    await wails.waitForApp();
    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    // Envia mensagem para iniciar o fluxo
    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeEditable({ timeout: 5_000 });
    await textarea.click();
    await textarea.fill('Pesquise o clima');
    await expect.poll(async () => textarea.inputValue()).toBe('Pesquise o clima');
    await textarea.press('Enter');

    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some(
        (c: { fn: string }) => c.fn === 'SendMessage',
      );
    }, { timeout: 5_000 });

    // Backend confirma a mensagem do usuário
    await wails.emit('chat:messages_ready', {
      conversationId: '01926b90-0000-7000-8000-000000000001',
      userMessageId: '01926b90-0000-7000-8000-100000000100',
      userContent: 'Pesquise o clima',
    });

    await page.waitForFunction(() => {
      return document.querySelectorAll('.message-node').length >= 2;
    }, { timeout: 5_000 });

    // Simula início de streaming
    await wails.emit('chat:stream', {
      conversationId: '01926b90-0000-7000-8000-000000000001',
      messageId: '01926b90-0000-7000-8000-100000000002',
      content: 'Buscando...',
      done: false,
    });

    await page.waitForFunction(() => {
      return document.querySelectorAll('.message-node').length >= 3;
    }, { timeout: 5_000 });

    // Simula início de tool call
    await wails.emit('chat:tool_start', {
      conversationId: '01926b90-0000-7000-8000-000000000001',
      name: 'search_web',
      callId: 'tc-live-1',
      args: '{"query":"clima"}',
    });

    // Aguarda a seção de tool calls aparecer
    const toolSection = page.locator('.tool-calls-section');
    await expect(toolSection).toBeVisible({ timeout: 5_000 });
  });

  test('tool call muda de running para done durante streaming', async ({ page, wails }) => {
    await setMessagesResponse(wails, [
      {
        message: {
          id: '01926b90-0000-7000-8000-000000000010',
          conversationId: '01926b90-0000-7000-8000-000000000001',
          role: 'user',
          content: 'Pesquise algo',
          createdAt: now,
        },
        children: [],
      },
    ]);
    await wails.setResponse('SendMessage', '01926b90-0000-7000-8000-100000000002');

    await wails.waitForApp();
    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeEditable({ timeout: 5_000 });
    await textarea.click();
    await textarea.fill('Pesquise o clima');
    await expect.poll(async () => textarea.inputValue()).toBe('Pesquise o clima');
    await textarea.press('Enter');

    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some(
        (c: { fn: string }) => c.fn === 'SendMessage',
      );
    }, { timeout: 5_000 });

    // Backend confirma a mensagem do usuário
    await wails.emit('chat:messages_ready', {
      conversationId: '01926b90-0000-7000-8000-000000000001',
      userMessageId: '01926b90-0000-7000-8000-100000000100',
      userContent: 'Pesquise o clima',
    });

    await page.waitForFunction(() => {
      return document.querySelectorAll('.message-node').length >= 2;
    }, { timeout: 5_000 });

    await wails.emit('chat:stream', {
      conversationId: '01926b90-0000-7000-8000-000000000001',
      messageId: '01926b90-0000-7000-8000-100000000002',
      content: 'Buscando...',
      done: false,
    });

    await page.waitForFunction(() => {
      return document.querySelectorAll('.message-node').length >= 3;
    }, { timeout: 5_000 });

    await wails.emit('chat:tool_start', {
      conversationId: '01926b90-0000-7000-8000-000000000001',
      name: 'search_web',
      callId: 'tc-live-2',
      args: '{"query":"clima"}',
    });

    await page.waitForSelector('.tool-calls-section', { timeout: 5_000 });

    // Verifica que a seção mostra estado de running
    const runningSection = page.locator('.tool-calls-section__item--running');
    await expect(runningSection).toBeVisible({ timeout: 3_000 });

    // Finaliza o tool call (muda de running para done)
    await wails.emit('chat:tool_end', {
      conversationId: '01926b90-0000-7000-8000-000000000001',
      callId: 'tc-live-2',
      name: 'search_web',
      status: 'success',
      summary: 'Encontrado: Ensolarado',
    });

    // Após tool_end, a seção deve ainda estar visível (tool call done, streaming continua)
    await expect(page.locator('.tool-calls-section')).toBeVisible({ timeout: 3_000 });
  });
});
