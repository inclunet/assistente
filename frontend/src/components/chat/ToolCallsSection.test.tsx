import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ToolCallsSection } from './ToolCallsSection';
import { axe } from '../../test/a11yAxe';

const loadDetails = vi.fn();
const openToolNavigationTarget = vi.fn();
const announce = vi.fn();

vi.mock('../../lib/toolTargetNavigation', () => ({
  openToolNavigationTarget: (...args: unknown[]) => openToolNavigationTarget(...args),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  announce: (...args: unknown[]) => announce(...args),
}));

vi.mock('../../services/toolInvocationDetailsCache', () => ({
  loadToolInvocationDetails: (...args: unknown[]) => loadDetails(...args),
}));

vi.mock('../../store/authStore', () => ({
  useAuthStore: (selector: (state: { user: { userId: string } }) => unknown) =>
    selector({ user: { userId: 'user-a' } }),
}));

vi.mock('react-i18next', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react-i18next')>()),
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}));

describe('ToolCallsSection', () => {
  beforeEach(() => {
    openToolNavigationTarget.mockReset();
    openToolNavigationTarget.mockResolvedValue(undefined);
    announce.mockReset();
  });

  it('renderiza tool calls ativos', () => {
    render(
      <ToolCallsSection
        activeToolCalls={[{ name: 'Search', callId: '1', status: 'running' }]}
      />
    );

    expect(screen.getByText('chat.toolGeneric')).toBeInTheDocument();
    expect(screen.queryByText('Search')).not.toBeInTheDocument();
  });

  it('torna os controles focáveis somente durante a leitura', () => {
    const props = {
      activeToolCalls: [{ name: 'Search', callId: '1', status: 'running' as const }],
    };
    const { rerender } = render(<ToolCallsSection {...props} />);
    expect(screen.getByRole('button')).toHaveAttribute('tabindex', '-1');

    rerender(<ToolCallsSection {...props} tabNavigationEnabled />);
    expect(screen.getByRole('button')).toHaveAttribute('tabindex', '0');
  });

  it('carrega detalhes somente ao expandir a invocação persistida', async () => {
    loadDetails.mockResolvedValue(new Map([['inv-1', {
      invocationId: 'inv-1',
      input: '{"q":"integral"}',
      output: '{"content":"resultado integral"}',
      metadata: '{}',
    }]]));
    render(
      <ToolCallsSection
        tabNavigationEnabled
        toolInvocations={[{
          invocationId: 'inv-1',
          callId: 'call-1',
          name: 'search',
          status: 'succeeded',
          inputPreview: '{"q":"[redacted]"}',
          outputPreview: 'prévia',
          hasDetails: true,
          resultAvailability: 'available',
        }]}
      />,
    );

    expect(loadDetails).not.toHaveBeenCalled();
    expect(screen.queryByText('prévia')).not.toBeInTheDocument();
    const detailsButton = screen.getByRole('button', { name: 'chat.technicalDetails' });
    expect(detailsButton).toHaveAttribute('tabindex', '0');
    fireEvent.click(detailsButton);

    await waitFor(() => expect(loadDetails).toHaveBeenCalledWith('user-a', ['inv-1']));
    expect(await screen.findByText('resultado integral')).toBeInTheDocument();
  });

  it('abre resultados estruturados de uma busca sem interpretar a saída textual', async () => {
    loadDetails.mockResolvedValue(new Map([['inv-search', {
      invocationId: 'inv-search', input: '{}', output: '{"content":"texto arbitrário"}',
      metadata: JSON.stringify({ search_result_presentation: {
        version: 1, total: 1, truncated: false,
        items: [{ kind: 'file', title: 'arquivo.ts', snippet: '10: match', target: { kind: 'file', path: 'C:/repo/arquivo.ts' } }],
      } }),
    }]]));
    render(<ToolCallsSection tabNavigationEnabled toolInvocations={[{
      invocationId: 'inv-search', callId: 'call-search', name: 'search_files', status: 'succeeded',
      hasDetails: true, resultAvailability: 'available', hasSearchResults: true, searchResultCount: 1,
    }]} />);

    fireEvent.click(screen.getByRole('button', { name: 'chat.viewSearchResults' }));

    expect(await screen.findByText('arquivo.ts')).toBeInTheDocument();
    expect(screen.getByText('10: match')).toBeInTheDocument();
    expect(screen.queryByText('texto arbitrário')).not.toBeInTheDocument();
  });

  it('anuncia quando não consegue abrir um destino de ferramenta', async () => {
    openToolNavigationTarget.mockRejectedValueOnce(new Error('falha'));
    render(<ToolCallsSection tabNavigationEnabled activeToolCalls={[{
      name: 'read_file', callId: 'read-1', status: 'running', args: '{"path":"C:/repo/arquivo.ts"}',
    }]} />);

    fireEvent.click(screen.getByRole('button', { name: /chat\.toolReadFile/ }));

    await waitFor(() => expect(announce).toHaveBeenCalledWith('chat.toolTargetOpenFailed', 'assertive'));
  });

  it('anuncia quando não consegue abrir um resultado de busca', async () => {
    loadDetails.mockResolvedValue(new Map([['inv-open-failure', {
      invocationId: 'inv-open-failure', metadata: JSON.stringify({ search_result_presentation: {
        version: 1, total: 1, items: [{ kind: 'url', title: 'Resultado', target: { kind: 'url', url: 'https://example.com' } }],
      } }),
    }]]));
    openToolNavigationTarget.mockRejectedValueOnce(new Error('falha'));
    render(<ToolCallsSection tabNavigationEnabled toolInvocations={[{
      invocationId: 'inv-open-failure', callId: 'search-open-failure', name: 'search_web', status: 'succeeded',
      hasDetails: true, resultAvailability: 'available', hasSearchResults: true, searchResultCount: 1,
    }]} />);

    fireEvent.click(screen.getByRole('button', { name: 'chat.viewSearchResults' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Resultado' }));

    await waitFor(() => expect(announce).toHaveBeenCalledWith('chat.toolTargetOpenFailed', 'assertive'));
  });

  it('pagina resultados estruturados sem carregar detalhes adicionais', async () => {
    const items = Array.from({ length: 21 }, (_, index) => ({
      kind: 'file', title: `arquivo-${index + 1}.ts`, target: { kind: 'file', path: `C:/repo/arquivo-${index + 1}.ts` },
    }));
    loadDetails.mockResolvedValue(new Map([['inv-paged', {
      invocationId: 'inv-paged', metadata: JSON.stringify({ search_result_presentation: { version: 1, total: 21, items } }),
    }]]));
    render(<ToolCallsSection tabNavigationEnabled toolInvocations={[{
      invocationId: 'inv-paged', callId: 'call-paged', name: 'search_files', status: 'succeeded',
      hasDetails: true, resultAvailability: 'available', hasSearchResults: true, searchResultCount: 21,
    }]} />);

    fireEvent.click(screen.getByRole('button', { name: 'chat.viewSearchResults' }));
    expect(await screen.findByText('arquivo-20.ts')).toBeInTheDocument();
    expect(screen.queryByText('arquivo-21.ts')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'chat.nextPage' }));
    expect(screen.getByText('arquivo-21.ts')).toBeInTheDocument();
    expect(screen.queryByText('arquivo-1.ts')).not.toBeInTheDocument();
    expect(loadDetails).toHaveBeenCalledTimes(1);
  });

  it('não introduz violações axe no resumo persistido expandido', async () => {
    const { container } = render(
      <ToolCallsSection
        tabNavigationEnabled
        toolInvocations={[{
          invocationId: 'inv-axe',
          callId: 'call-axe',
          name: 'search',
          status: 'succeeded',
          inputPreview: '{"fields":["q"]}',
          outputPreview: '{"bytes":42}',
          hasDetails: true,
          resultAvailability: 'available',
        }]}
      />,
    );
    expect(await axe(container)).toHaveNoViolations();
  });

  it('usa fallback plug-and-play do provedor MCP e duração da projeção canônica', () => {
    render(
      <ToolCallsSection
        toolInvocations={[{
          invocationId: 'inv-mcp',
          callId: 'call-mcp',
          name: 'jira_search',
          origin: 'mcp_native',
          serverLabel: 'Atlassian',
          status: 'succeeded',
          durationMs: 1500,
          hasDetails: false,
          resultAvailability: 'unavailable',
        }]}
      />,
    );
    expect(screen.getByText('chat.toolMcpProvider')).toBeInTheDocument();
    expect(screen.getByText('1.5s')).toBeInTheDocument();
  });

  it('marca a ferramenta do agente externo enquanto ela roda', () => {
    render(
      <ToolCallsSection
        activeToolCalls={[
          { name: 'execute', callId: '1', status: 'running', origin: 'acp_agent' },
          { name: 'read_file', callId: '2', status: 'running', origin: 'builtin' },
        ]}
      />
    );

    expect(screen.getAllByText('chat.toolGeneric')).toHaveLength(1);
    expect(screen.getByText('chat.toolReadFile')).toBeInTheDocument();
    expect(screen.getAllByRole('listitem')).toHaveLength(2);
  });

  it('mostra ação concluída e mantém prévias técnicas apenas nos detalhes', () => {
    render(<ToolCallsSection tabNavigationEnabled toolInvocations={[{
      invocationId: 'inv-read', callId: 'read', name: 'read_file', origin: 'builtin',
      status: 'succeeded', inputPreview: '{"fields":["path"]}', outputPreview: '{"bytes":42}',
      hasDetails: false, resultAvailability: 'available',
    }]} />);
    expect(screen.getByText('chat.toolReadFileDone')).toBeInTheDocument();
    expect(screen.queryByText('{"bytes":42}')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'chat.technicalDetails' }));
    expect(screen.getByRole('dialog')).toHaveTextContent('{"bytes":42}');
  });

  it('mantém estados persistidos canônicos perceptíveis sem prometer sucesso', () => {
    render(
      <ToolCallsSection
        toolInvocations={[
          { invocationId: 'queued', callId: 'queued', name: 'search_files', status: 'queued', hasDetails: false, resultAvailability: 'pending', outputPreview: 'parcial' },
          { invocationId: 'failed', callId: 'failed', name: 'search_files', status: 'failed', hasDetails: false, resultAvailability: 'available' },
          { invocationId: 'cancelled', callId: 'cancelled', name: 'search_files', status: 'cancelled', hasDetails: false, resultAvailability: 'available' },
          { invocationId: 'unknown', callId: 'unknown', name: 'search_files', status: 'unexpected', hasDetails: false, resultAvailability: 'available' },
        ]}
      />,
    );

    expect(screen.getAllByText('chat.toolStatusRunning')).toHaveLength(1);
    expect(screen.getByText('chat.toolStatusFailed')).toBeInTheDocument();
    expect(screen.getByText('chat.toolStatusCancelled')).toBeInTheDocument();
    expect(screen.getByText('chat.toolStatusUnknown')).toBeInTheDocument();
    expect(screen.getByText('chat.partialOutputAvailable')).toBeInTheDocument();
    expect(screen.queryByText('parcial')).not.toBeInTheDocument();
  });

  it('trata done do streaming como concluído', () => {
    render(<ToolCallsSection activeToolCalls={[{ name: 'search', callId: 'done', status: 'done', args: '{"token":"secret"}' }]} tabNavigationEnabled />);
    expect(screen.getByText('chat.toolStatusSucceeded')).toBeInTheDocument();
  });

  it('não mostra a prévia técnica recebida durante o streaming', () => {
    const technicalPreview = '{"bytes":42,"fields":["content"]}';
    render(<ToolCallsSection tabNavigationEnabled activeToolCalls={[{
      name: 'read_file', callId: 'streaming-preview', status: 'running',
      args: '{"path":"C:/repo/arquivo.txt"}', summary: technicalPreview,
    }]} />);

    expect(screen.getByText('chat.partialOutputAvailable')).toBeInTheDocument();
    expect(screen.queryByText(technicalPreview)).not.toBeInTheDocument();
  });

  it('apresenta timeout persistido como falha terminal', () => {
    render(<ToolCallsSection toolInvocations={[
      {
        invocationId: 'inv-timed-out', callId: 'call-timed-out', name: 'search',
        status: 'timed_out', hasDetails: true, resultAvailability: 'available',
      },
      {
        invocationId: 'inv-timeout', callId: 'call-timeout', name: 'search',
        status: 'timeout', hasDetails: true, resultAvailability: 'available',
      },
    ]} />);

    expect(screen.getAllByText('chat.toolStatusFailed')).toHaveLength(2);
    expect(screen.queryByText('chat.toolStatusUnknown')).not.toBeInTheDocument();
  });

  it('sanitiza o fallback de argumentos no modal e acompanha a transição do ativo', async () => {
    const props = { activeToolCalls: [{ name: 'search', callId: 'same', status: 'running' as const, args: '{"password":"secret"}', summary: 'parcial' }] };
    const { rerender } = render(<ToolCallsSection {...props} tabNavigationEnabled />);
    fireEvent.click(screen.getByRole('button', { name: 'chat.technicalDetails' }));
    expect(screen.getByRole('dialog').querySelector('.tool-calls-section__args')).toHaveTextContent('[redacted]');
    expect(screen.getByRole('dialog')).toHaveTextContent('chat.toolStatusRunning');

    loadDetails.mockResolvedValue(new Map([['inv-transition', {
      invocationId: 'inv-transition', input: '{"password":"secret"}', output: '{"content":"integral"}', metadata: '{}',
    }]]));
    rerender(<ToolCallsSection tabNavigationEnabled toolInvocations={[{
      invocationId: 'inv-transition', callId: 'same', name: 'search', status: 'succeeded',
      inputPreview: '{"password":"[redacted]"}', outputPreview: 'preview', hasDetails: true, resultAvailability: 'available',
    }]} />);
    expect(screen.getByRole('dialog')).toHaveTextContent('chat.toolStatusSucceeded');
    expect(await screen.findByText('integral')).toBeInTheDocument();
  });

  it('mantém o modal acessível durante loading e restaura foco após fechar', async () => {
    loadDetails.mockResolvedValue(new Map([['inv-modal', {
      invocationId: 'inv-modal', input: '{}', output: '{"content":"ok"}', metadata: '{}',
    }]]));
    const { container } = render(<ToolCallsSection tabNavigationEnabled toolInvocations={[{
      invocationId: 'inv-modal', callId: 'modal', name: 'search', status: 'succeeded', hasDetails: true, resultAvailability: 'available',
    }]} />);
    const details = screen.getByRole('button', { name: 'chat.technicalDetails' });
    details.focus();
    fireEvent.click(details);
    expect(await screen.findByText('ok')).toBeInTheDocument();
    expect(await axe(container)).toHaveNoViolations();
    fireEvent.click(screen.getByRole('button', { name: 'ui.modal.close' }));
    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(document.activeElement).toBe(details);
  });

  it('não aplica resposta obsoleta ao fechar e abrir outra invocação', async () => {
    let resolveFirst!: (value: Map<string, unknown>) => void;
    const first = new Promise<Map<string, unknown>>((resolve) => { resolveFirst = resolve; });
    loadDetails.mockImplementation((_: string, ids: string[]) => ids[0] === 'first' ? first : Promise.resolve(new Map([['second', {
      invocationId: 'second', input: '{}', output: '{"content":"segundo"}', metadata: '{}',
    }]])));
    const { rerender } = render(<ToolCallsSection tabNavigationEnabled toolInvocations={[{
      invocationId: 'first', callId: 'first', name: 'search', status: 'succeeded', hasDetails: true, resultAvailability: 'available',
    }]} />);
    fireEvent.click(screen.getByRole('button', { name: 'chat.technicalDetails' }));
    fireEvent.click(screen.getByRole('button', { name: 'ui.modal.close' }));
    rerender(<ToolCallsSection tabNavigationEnabled toolInvocations={[{
      invocationId: 'second', callId: 'second', name: 'search', status: 'succeeded', hasDetails: true, resultAvailability: 'available',
    }]} />);
    fireEvent.click(screen.getByRole('button', { name: 'chat.technicalDetails' }));
    resolveFirst(new Map([['first', { invocationId: 'first', input: '{}', output: '{"content":"primeiro"}', metadata: '{}' }]]));
    expect(await screen.findByText('segundo')).toBeInTheDocument();
    expect(screen.queryByText('primeiro')).not.toBeInTheDocument();
  });

  it('avisa limite da busca e pagina os 100 itens preservados', async () => {
    const items = Array.from({ length: 150 }, (_, index) => ({ kind: 'text', title: `resultado-${index + 1}` }));
    loadDetails.mockResolvedValue(new Map([['inv-limit', { invocationId: 'inv-limit', metadata: JSON.stringify({ search_result_presentation: { version: 1, total: 150, items } }) }]]));
    render(<ToolCallsSection tabNavigationEnabled toolInvocations={[{
      invocationId: 'inv-limit', callId: 'limit', name: 'search_files', status: 'succeeded', hasDetails: true, hasSearchResults: true, searchResultCount: 150, resultAvailability: 'available',
    }]} />);
    fireEvent.click(screen.getByRole('button', { name: 'chat.viewSearchResults' }));
    expect(await screen.findByText(/resultado-20/)).toBeInTheDocument();
    expect(screen.queryByText('resultado-101')).not.toBeInTheDocument();
    expect(screen.getByText(/chat.searchResultsTruncated/)).toBeInTheDocument();
    for (let page = 0; page < 4; page += 1) fireEvent.click(screen.getByRole('button', { name: 'chat.nextPage' }));
    expect(screen.getByText('resultado-100')).toBeInTheDocument();
    expect(screen.queryByText('resultado-101')).not.toBeInTheDocument();
  });

});
