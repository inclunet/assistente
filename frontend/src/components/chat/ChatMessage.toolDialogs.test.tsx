import { invocationFixture, messageFixture } from '../../test/chatPresentationFixture';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { ChatMessage } from './ChatMessage';
import { ToolCallsSection } from './ToolCallsSection';
import { ToolInvocationDialogsProvider } from './ToolInvocationDialogs';

const currentUser = vi.hoisted(() => ({ id: 'user-a' }));
const loadDetails = vi.hoisted(() => vi.fn());
const translate = vi.hoisted(() => (key: string) => key);

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: translate }) }));
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
  messageFixture({
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
  it('preserva o parcial e apresenta falha canônica sem inventar ferramenta sem ledger', () => {
    render(<ChatMessage message={messageFixture({ ...baseMessage(),
      content: 'parcial preservado', isStreaming: false,
      timelineItem: { state: 'failed' },
    })} />);
    expect(screen.getByText('chat.timelineFailed')).toBeInTheDocument();
    expect(screen.getByText('parcial preservado')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'chat.technicalDetails' })).not.toBeInTheDocument();
    expect(loadDetails).not.toHaveBeenCalled();
  });

  it.each([0, 1])('seleciona a invocação %i sem fundir call IDs reutilizados', async (index) => {
    const calls = ['first-execution', 'second-execution'].map(invocationId => invocationFixture({
      invocationId, callId: 'reused-provider-id', name: 'search', status: 'succeeded',
      hasDetails: true, resultAvailability: 'available', outputPreview: invocationId,
    }));
    loadDetails.mockImplementation((_: string, ids: string[]) => Promise.resolve(new Map(ids.map(id => [id, {
      invocationId: id, input: '{}', output: JSON.stringify({ content: `detalhes de ${id}` }), metadata: '{}',
    }]))));
    const frame = (invocations = calls) => messageFixture({ ...baseMessage(),
      turnSegments: [{ type: 'tool_calls', toolInvocations: invocations }],
    });
    const { rerender } = render(<ChatMessage message={frame()} isReading />);
    const buttons = screen.getAllByRole('button', { name: 'chat.technicalDetails' });
    fireEvent.click(buttons[index]);
    expect(await screen.findByText(`detalhes de ${calls[index].invocationId}`)).toBeInTheDocument();
    expect(loadDetails).toHaveBeenCalledWith('user-a', [calls[index].invocationId]);
    rerender(<ChatMessage message={frame([...calls].reverse())} isReading />);
    expect(screen.getByRole('dialog')).toHaveTextContent(`detalhes de ${calls[index].invocationId}`);
    fireEvent.click(screen.getByRole('button', { name: 'ui.modal.close' }));
    await waitFor(() => expect(document.activeElement).toBe(buttons[index]));
    expect(screen.getAllByRole('button', { name: 'chat.technicalDetails' })[1 - index]).toBe(buttons[index]);
  });

  it('mantém o detalhe selecionado quando outra execução reutiliza o call ID', () => {
    const selected = invocationFixture({ invocationId: 'previous', callId: 'reused', outputPreview: 'prévia anterior' });
    const replacement = invocationFixture({ invocationId: 'next', callId: 'reused', status: 'succeeded', outputPreview: 'outra execução' });
    const { rerender } = render(<ToolCallsSection toolInvocations={[selected]} tabNavigationEnabled />);
    fireEvent.click(screen.getByRole('button', { name: 'chat.technicalDetails' }));
    rerender(<ToolCallsSection toolInvocations={[replacement]} tabNavigationEnabled />);
    expect(screen.getByRole('dialog')).toHaveTextContent('prévia anterior');
    expect(screen.getByRole('dialog')).toHaveTextContent('chat.toolStatusRunning');
    expect(screen.getByRole('dialog')).not.toHaveTextContent('outra execução');
    expect(loadDetails).not.toHaveBeenCalled();
  });
  it.each([false, true])('preserva a conclusão após retirada da janela, com detalhes=%s', async (hasDetails) => {
    const initial = invocationFixture({ invocationId: 'previous', callId: 'reused', outputPreview: 'prévia parcial', hasDetails });
    const completed = { ...initial, status: 'succeeded', outputPreview: 'prévia final' };
    const replacement = invocationFixture({ invocationId: 'next', callId: 'reused', outputPreview: 'outra execução' });
    loadDetails.mockResolvedValue(new Map([['previous', { invocationId: 'previous', output: 'resultado completo', metadata: '{}' }]]));
    const { rerender } = render(<ToolCallsSection toolInvocations={[initial]} tabNavigationEnabled />);
    fireEvent.click(screen.getByRole('button', { name: 'chat.technicalDetails' }));
    expect(screen.getByRole('dialog')).toHaveTextContent('chat.toolStatusRunning');
    rerender(<ToolCallsSection toolInvocations={[completed]} tabNavigationEnabled />);
    const expectedOutput = hasDetails ? 'resultado completo' : 'prévia final';
    await waitFor(() => expect(screen.getByRole('dialog')).toHaveTextContent(expectedOutput));
    const dialog = screen.getByRole('dialog');
    rerender(<ToolCallsSection toolInvocations={[replacement]} tabNavigationEnabled />);
    expect(screen.getByRole('dialog')).toBe(dialog);
    expect(dialog).toHaveTextContent('chat.toolStatusSucceeded');
    expect(dialog).toHaveTextContent(expectedOutput);
    expect(dialog).not.toHaveTextContent('prévia parcial');
    expect(dialog).not.toHaveTextContent('outra execução');
    expect(loadDetails).toHaveBeenCalledTimes(hasDetails ? 1 : 0);
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
    const frame = (status: string, outputPreview: string) => messageFixture({
      ...baseMessage(), turnSegments: [{ type: 'tool_calls', toolInvocations: [{
        invocationId: 'inv-1', callId: 'call-1', name: 'search', status, outputPreview,
        hasDetails: false, resultAvailability: 'available',
      }] }],
    });
    const { rerender } = render(<ChatMessage message={frame('running', 'parcial')} />);
    fireEvent.click(screen.getByRole('button', { name: 'chat.technicalDetails' }));
    expect(screen.getByRole('dialog')).toHaveTextContent('chat.toolStatusRunning');
    rerender(<ChatMessage message={frame('succeeded', 'segment result')} />);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByRole('dialog')).toHaveTextContent('chat.toolStatusSucceeded');
    expect(screen.getByRole('dialog')).toHaveTextContent('segment result');

    rerender(
      <ChatMessage
        message={
          messageFixture({
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
