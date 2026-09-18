import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ToolCallsSection } from './ToolCallsSection';
import { axe } from '../../test/a11yAxe';

const loadDetails = vi.fn();

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
  it('renderiza tool calls ativos', () => {
    render(
      <ToolCallsSection
        activeToolCalls={[{ name: 'Search', callId: '1', status: 'running' }]}
      />
    );

    fireEvent.click(screen.getByRole('button'));
    expect(screen.getByText('chat.toolGeneric')).toBeInTheDocument();
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
    fireEvent.click(screen.getByRole('button'));
    expect(screen.getByText('prévia')).toBeInTheDocument();
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

    fireEvent.click(screen.getByRole('button'));
    fireEvent.click(screen.getByRole('button', { name: 'chat.viewSearchResults' }));

    expect(await screen.findByText('arquivo.ts')).toBeInTheDocument();
    expect(screen.getByText('10: match')).toBeInTheDocument();
    expect(screen.queryByText('texto arbitrário')).not.toBeInTheDocument();
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
    fireEvent.click(screen.getByRole('button'));
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
    fireEvent.click(screen.getByRole('button'));
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

    fireEvent.click(screen.getByRole('button'));

    expect(screen.getAllByText('chat.toolGeneric')).toHaveLength(1);
    expect(screen.getByText('chat.toolReadFile')).toBeInTheDocument();
  });

});
