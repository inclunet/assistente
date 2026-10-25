import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { chat } from '../../../wailsjs/go/models';
import { ChatMessage } from './ChatMessage';
import { ToolCallsSection } from './ToolCallsSection';
import { ToolInvocationDialogsProvider } from './ToolInvocationDialogs';

const currentUser = vi.hoisted(() => ({ id: 'user-a' }));
const loadDetails = vi.hoisted(() => vi.fn());

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('./ChatSessionContext', () => ({
  useChatMessageLiveState: () => ({
    liveContent: null,
    liveIsStreaming: null,
    liveReasoning: null,
    liveSegments: [],
    liveToolCalls: [],
  }),
}));
vi.mock('../../store/authStore', () => ({
  useAuthStore: (selector: (state: { user: { userId: string } }) => unknown) =>
    selector({ user: { userId: currentUser.id } }),
}));
vi.mock('../../services/toolInvocationDetailsCache', () => ({
  loadToolInvocationDetails: loadDetails,
}));
vi.mock('../../hooks/useAnnouncer', () => ({
  announce: vi.fn(),
  useAnnouncer: () => ({ announceRequest: vi.fn() }),
}));
vi.mock('../../lib/dateUtils', () => ({ formatRelativeTime: () => 'agora' }));
vi.mock('../../lib/chatUtils', () => ({ isAgentMessage: () => false }));
vi.mock('../../lib/chatMessageAriaLabel', () => ({ buildChatMessageAriaLabel: () => 'aria' }));
vi.mock('../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}));

const baseMessage = () =>
  new chat.EnrichedMessage({
    id: 'dialog-host',
    conversationId: 'conversation-1',
    role: 'assistant',
    content: '',
    createdAt: new Date().toISOString(),
    timestamp: Date.now(),
    isStreaming: true,
    internal: false,
  });

describe('ChatMessage tool dialog host', () => {
  beforeEach(() => {
    currentUser.id = 'user-a';
    loadDetails.mockReset();
  });
  it('traduz a falha local pelo código sem exibir a mensagem técnica', async () => {
    const call = { callId: 'blocked', name: 'echo', status: 'error' as const,
      errorCode: 'invocation_ledger_unavailable', summary: 'backend diagnostic' };
    render(<ToolCallsSection tabNavigationEnabled activeToolCalls={[call]} />);
    fireEvent.click(screen.getByRole('button', { name: 'chat.technicalDetails' }));
    expect(await screen.findByText('chat.invocationLedgerUnavailable')).toBeInTheDocument();
    expect(screen.queryByText('backend diagnostic')).not.toBeInTheDocument();
    expect(loadDetails).not.toHaveBeenCalled();
  });
  it.each(['', 'saída original preservada'])(
    'mostra diagnóstico separado da saída MCP %j',
    async (content) => {
      const call = {
        invocationId: 'inv-error',
        callId: 'call-error',
        name: 'tool_catalog',
        status: 'failed' as const,
        hasDetails: true,
        resultAvailability: 'available' as const,
      };
      loadDetails.mockResolvedValue(
        new Map([
          [
            'inv-error',
            {
              invocationId: 'inv-error',
              input: '{}',
              output: JSON.stringify({ content, is_error: true }),
              errorMessage: 'Contrato MCP incompatível',
              metadata: '{}',
            },
          ],
        ])
      );
      render(
        <ToolInvocationDialogsProvider currentCalls={[call]}>
          <ToolCallsSection tabNavigationEnabled toolInvocations={[call]} />
        </ToolInvocationDialogsProvider>
      );
      fireEvent.click(screen.getByRole('button', { name: 'chat.technicalDetails' }));
      expect(await screen.findByText('Contrato MCP incompatível')).toBeInTheDocument();
      const errorSection = screen.getByRole('heading', { name: 'common.error' }).closest('section');
      const outputSection = screen
        .getByRole('heading', { name: 'chat.response' })
        .closest('section');
      expect(errorSection?.querySelector('pre')?.textContent).toBe('Contrato MCP incompatível');
      expect(outputSection?.querySelector('pre')?.textContent).toBe(content);
    }
  );
  it('mantém o diálogo aberto ao trocar ativo por segmento e patch canônico', () => {
    const { rerender } = render(
      <ChatMessage
        message={baseMessage()}
        activeToolCalls={[
          { callId: 'call-1', name: 'search', status: 'running', summary: 'parcial' },
        ]}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'chat.technicalDetails' }));
    expect(screen.getByRole('dialog')).toHaveTextContent('chat.toolStatusRunning');

    rerender(
      <ChatMessage
        message={baseMessage()}
        activeToolCalls={[]}
        completedSegments={[
          {
            type: 'tool_calls',
            toolCalls: [
              {
                id: 'call-1',
                type: 'function',
                function: { name: 'search', arguments: '{}' },
                status: 'done',
                result: 'segment result',
              },
            ],
          },
        ]}
      />
    );
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toHaveTextContent('chat.toolStatusSucceeded');
    expect(screen.getByRole('dialog')).toHaveTextContent('segment result');

    rerender(
      <ChatMessage
        message={
          new chat.EnrichedMessage({
            ...baseMessage(),
            isStreaming: false,
            turnSegments: [
              {
                type: 'tool_calls',
                toolInvocations: [
                  {
                    invocationId: 'inv-1',
                    callId: 'call-1',
                    name: 'search',
                    status: 'succeeded',
                    outputPreview: 'canonical result',
                    hasDetails: false,
                    resultAvailability: 'available',
                  },
                ],
              },
            ],
          })
        }
        activeToolCalls={[]}
      />
    );
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toHaveTextContent('canonical result');
    expect(screen.getByRole('dialog')).not.toHaveTextContent('stale segment result');
  });

  it('fecha detalhe carregado quando o usuário muda', async () => {
    loadDetails.mockResolvedValue(
      new Map([
        [
          'inv-1',
          { invocationId: 'inv-1', input: '{}', output: '{"content":"segredo"}', metadata: '{}' },
        ],
      ])
    );
    const view = render(
      <ToolInvocationDialogsProvider
        currentCalls={[
          {
            invocationId: 'inv-1',
            callId: 'call-1',
            name: 'search',
            status: 'succeeded',
            hasDetails: true,
            resultAvailability: 'available',
          },
        ]}
      >
        <ToolCallsSection
          tabNavigationEnabled
          toolInvocations={[
            {
              invocationId: 'inv-1',
              callId: 'call-1',
              name: 'search',
              status: 'succeeded',
              hasDetails: true,
              resultAvailability: 'available',
            },
          ]}
        />
      </ToolInvocationDialogsProvider>
    );
    fireEvent.click(screen.getByRole('button', { name: 'chat.technicalDetails' }));
    expect(await screen.findByText('segredo')).toBeInTheDocument();
    currentUser.id = 'user-b';
    await act(async () => {
      view.rerender(
        <ToolInvocationDialogsProvider
          currentCalls={[
            {
              invocationId: 'inv-1',
              callId: 'call-1',
              name: 'search',
              status: 'succeeded',
              hasDetails: true,
              resultAvailability: 'available',
            },
          ]}
        >
          <ToolCallsSection
            tabNavigationEnabled
            toolInvocations={[
              {
                invocationId: 'inv-1',
                callId: 'call-1',
                name: 'search',
                status: 'succeeded',
                hasDetails: true,
                resultAvailability: 'available',
              },
            ]}
          />
        </ToolInvocationDialogsProvider>
      );
    });
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  });

  it('descarta busca pendente após troca de usuário', async () => {
    let resolveDetails!: (value: Map<string, unknown>) => void;
    loadDetails.mockReturnValue(
      new Promise<Map<string, unknown>>((resolve) => {
        resolveDetails = resolve;
      })
    );
    const view = render(
      <ToolInvocationDialogsProvider
        currentCalls={[
          {
            invocationId: 'inv-search',
            callId: 'call-search',
            name: 'search',
            status: 'succeeded',
            hasDetails: true,
            hasSearchResults: true,
            searchResultCount: 1,
            resultAvailability: 'available',
          },
        ]}
      >
        <ToolCallsSection
          tabNavigationEnabled
          toolInvocations={[
            {
              invocationId: 'inv-search',
              callId: 'call-search',
              name: 'search',
              status: 'succeeded',
              hasDetails: true,
              hasSearchResults: true,
              searchResultCount: 1,
              resultAvailability: 'available',
            },
          ]}
        />
      </ToolInvocationDialogsProvider>
    );
    fireEvent.click(screen.getByRole('button', { name: 'chat.viewSearchResults' }));
    currentUser.id = 'user-c';
    await act(async () => {
      view.rerender(
        <ToolInvocationDialogsProvider
          currentCalls={[
            {
              invocationId: 'inv-search',
              callId: 'call-search',
              name: 'search',
              status: 'succeeded',
              hasDetails: true,
              hasSearchResults: true,
              searchResultCount: 1,
              resultAvailability: 'available',
            },
          ]}
        >
          <ToolCallsSection
            tabNavigationEnabled
            toolInvocations={[
              {
                invocationId: 'inv-search',
                callId: 'call-search',
                name: 'search',
                status: 'succeeded',
                hasDetails: true,
                hasSearchResults: true,
                searchResultCount: 1,
                resultAvailability: 'available',
              },
            ]}
          />
        </ToolInvocationDialogsProvider>
      );
      resolveDetails(
        new Map([
          [
            'inv-search',
            {
              invocationId: 'inv-search',
              metadata: JSON.stringify({
                search_result_presentation: {
                  version: 1,
                  total: 1,
                  items: [{ kind: 'text', title: 'segredo antigo' }],
                },
              }),
            },
          ],
        ])
      );
    });
    await waitFor(() => expect(screen.queryByText('segredo antigo')).not.toBeInTheDocument());
  });
});
