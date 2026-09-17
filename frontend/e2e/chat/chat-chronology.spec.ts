import { test, expect, type Locator, type Page, type WailsMock } from '../fixtures';

const now = '2026-09-17T15:00:00.000Z';
const conversationId = '01926b90-0000-7000-8000-000000000001';
const historyUserMessageId = '01926b90-0000-7000-8000-000000000202';
const userMessageId = '01926b90-0000-7000-8000-000000000204';
const assistantMessageId = '01926b90-0000-7000-8000-000000000203';
const turnId = userMessageId;
const finalContent = 'resposta final canônica';

const userNode = {
  message: {
    id: historyUserMessageId,
    conversationId,
    role: 'user',
    content: 'Mensagem histórica anterior.',
    createdAt: now,
    timestamp: Date.parse(now),
  },
  children: [],
  childCount: 0,
};

const canonicalAssistantMessage = {
  id: assistantMessageId,
  conversationId,
  role: 'assistant',
  turnId,
  content: finalContent,
  createdAt: now,
  timestamp: Date.parse(now) + 1,
  turnSegments: [
    { type: 'text', content: 'texto intermediário um' },
    {
      type: 'tool_calls',
      toolInvocations: [{
        invocationId: 'inv-one',
        callId: 'call-one',
        name: 'buscar-primeira-fonte',
        status: 'succeeded',
        hasDetails: false,
        resultAvailability: 'available',
      }],
    },
    { type: 'text', content: 'texto intermediário dois' },
    {
      type: 'tool_calls',
      toolInvocations: [{
        invocationId: 'inv-two',
        callId: 'call-two',
        name: 'refinar-segunda-fonte',
        status: 'succeeded',
        hasDetails: false,
        resultAvailability: 'available',
      }],
    },
    { type: 'text', content: finalContent },
  ],
};

const canonicalNode = {
  message: canonicalAssistantMessage,
  children: [],
  childCount: 0,
};

const conversation = {
  id: conversationId,
  title: 'Cronologia de tools',
  created_at: now,
  updated_at: now,
  messages: [],
  message_count: 1,
};

const messageWindow = (nodes: unknown[]) => ({
  scope: 'conversation',
  conversationId,
  threadParentId: '',
  nodes,
  totalCount: nodes.length,
  startIndex: 0,
  endIndex: Math.max(0, nodes.length - 1),
  hasBefore: false,
  hasAfter: false,
});

async function configureHistory(wails: WailsMock, nodes: unknown[]) {
  await wails.setResponse('GetMessages', nodes);
  await wails.setResponse('GetConversationMessageWindow', messageWindow(nodes));
  await wails.setResponse('GetConversationInfo', conversation);
}

async function readChronology(assistant: Locator) {
  return assistant.evaluate((element, conclusion) => {
    const region = element.querySelector('.chat-message__segments-log');
    const final = element.querySelector('.chat-message__text--conclusion-preview');
    const toggle = element.querySelector('.chat-message__chain-toggle');
    const children = region
      ? Array.from(region.children).map((child) => {
        if (child.classList.contains('tool-calls-section')) {
          return `tool:${child.querySelector('.tool-calls-section__title')?.textContent?.trim()}`;
        }
        return `text:${child.textContent?.trim()}`;
      })
      : [];
    return {
      children,
      finalCount: Array.from(element.querySelectorAll('.chat-message__text--conclusion-preview'))
        .filter((node) => node.textContent?.trim() === conclusion).length,
      finalOutsideRegion: !!region && !!final && !region.contains(final),
      finalAfterRegion: !!region && !!final
        && !!(region.compareDocumentPosition(final) & Node.DOCUMENT_POSITION_FOLLOWING),
      controlsId: toggle?.getAttribute('aria-controls') || '',
      localLiveRegions: element.querySelectorAll('[aria-live]').length,
    };
  }, finalContent);
}

async function installReloadHistory(page: Page) {
  await page.addInitScript(({ nodes, info, windowResponse }) => {
    const apply = () => {
      window.__wailsMock.setResponse('GetMessages', nodes);
      window.__wailsMock.setResponse('GetConversationMessageWindow', windowResponse);
      window.__wailsMock.setResponse('GetConversationInfo', info);
    };
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', apply, { once: true });
    } else {
      apply();
    }
  }, {
    nodes: [userNode, canonicalNode],
    info: conversation,
    windowResponse: messageWindow([userNode, canonicalNode]),
  });
}

test.describe('Chat — cronologia canônica de tools', () => {
  test('preserva ordem no streaming, patch terminal, reload, colapso e modo leitura', async ({ page, wails }) => {
    await configureHistory(wails, [userNode]);
    await wails.setResponse('EnsureConversation', conversation);
    await wails.setResponse('SendMessage', assistantMessageId);
    await wails.waitForApp();

    const input = page.locator('.chat-input__textarea');
    await expect(input).toBeEditable();
    await input.fill('Organize as fontes em ordem cronológica.');
    await input.press('Enter');
    await page.waitForFunction(() => window.__wailsMock.getCallLog().some(
      (call: { fn: string }) => call.fn === 'SendMessage',
    ));
    await wails.emit('chat:messages_ready', {
      conversationId,
      userMessageId,
      userContent: 'Organize as fontes em ordem cronológica.',
      turnId,
    });

    const identity = { conversationId, turnId, assistantMessageId };
    await wails.emit('chat:thinking', {
      ...identity,
      started: true,
      content: 'Preparando a resposta.',
    });
    const assistant = page.locator('.message-node:has(.chat-message--assistant)').last();
    await expect(assistant).toBeVisible();
    await wails.emit('chat:thinking', {
      ...identity,
      done: true,
      content: 'Preparando a resposta.',
    });
    await wails.emit('chat:stream', {
      ...identity,
      messageId: assistantMessageId,
      delta: 'texto intermediário um',
      reset: true,
      sequence: 0,
    });
    await expect(assistant.getByText('texto intermediário um', { exact: true })).toBeVisible();
    await wails.emit('chat:tool_start', {
      ...identity,
      name: 'buscar-primeira-fonte',
      callId: 'call-one',
    });
    await wails.emit('chat:tool_end', {
      ...identity,
      name: 'buscar-primeira-fonte',
      callId: 'call-one',
      status: 'ok',
    });
    await wails.emit('chat:segment_done', {
      ...identity,
      content: 'texto intermediário um',
      hasMore: true,
    });
    await wails.emit('chat:stream', {
      ...identity,
      messageId: assistantMessageId,
      delta: 'texto intermediário dois',
      reset: true,
      sequence: 0,
    });
    await wails.emit('chat:tool_start', {
      ...identity,
      name: 'refinar-segunda-fonte',
      callId: 'call-two',
    });
    await wails.emit('chat:tool_end', {
      ...identity,
      name: 'refinar-segunda-fonte',
      callId: 'call-two',
      status: 'ok',
    });
    await wails.emit('chat:segment_done', {
      ...identity,
      content: 'texto intermediário dois',
      hasMore: true,
    });
    await wails.emit('chat:stream', {
      ...identity,
      messageId: assistantMessageId,
      delta: finalContent,
      reset: true,
      sequence: 0,
    });

    await expect(assistant.getByText('texto intermediário dois', { exact: true })).toBeVisible();
    await expect(assistant.locator('.tool-calls-section')).toHaveCount(2);

    await wails.emit('chat:done', {
      ...identity,
      hadToolCalls: true,
      reason: 'completed',
      turnPatch: { message: canonicalAssistantMessage },
    });

    await expect(assistant.locator('.chat-message__chain-toggle')).toBeVisible();
    await expect.poll(() => readChronology(assistant)).toMatchObject({
      children: [
        'text:texto intermediário um',
        'tool:buscar-primeira-fonte',
        'text:texto intermediário dois',
        'tool:refinar-segunda-fonte',
      ],
      finalCount: 1,
      finalOutsideRegion: true,
      finalAfterRegion: true,
      localLiveRegions: 0,
    });

    await installReloadHistory(page);
    await page.reload();
    const reloadedAssistant = page.locator('.message-node:has(.chat-message--assistant)').last();
    await expect(reloadedAssistant.getByText(finalContent, { exact: true })).toBeVisible();
    await expect.poll(() => readChronology(reloadedAssistant)).toMatchObject({
      children: [
        'text:texto intermediário um',
        'tool:buscar-primeira-fonte',
        'text:texto intermediário dois',
        'tool:refinar-segunda-fonte',
      ],
      finalCount: 1,
      finalOutsideRegion: true,
      finalAfterRegion: true,
    });

    const toggle = reloadedAssistant.locator('.chat-message__chain-toggle');
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
    await expect(toggle).toHaveAttribute('aria-controls', /^[^\s]+$/);
    const controlsId = await toggle.getAttribute('aria-controls');
    expect(controlsId).toBeTruthy();
    await expect(reloadedAssistant.locator(`[id="${controlsId}"]`)).toBeVisible();

    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');
    await expect(reloadedAssistant.getByText(finalContent, { exact: true })).toHaveCount(1);
    await expect.poll(() => readChronology(reloadedAssistant)).toMatchObject({
      children: [],
      finalCount: 1,
      finalOutsideRegion: true,
    });

    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
    await expect(reloadedAssistant.getByText(finalContent, { exact: true })).toHaveCount(1);

    await reloadedAssistant.focus();
    await reloadedAssistant.press('Enter');
    await expect(reloadedAssistant).toHaveAttribute('role', 'dialog');
    await expect(reloadedAssistant.locator('.chat-message__chain-toggle')).toHaveAttribute('tabindex', '0');
    await expect.poll(() => reloadedAssistant.evaluate(
      (element) => element.contains(document.activeElement),
    )).toBe(true);
  });
});
