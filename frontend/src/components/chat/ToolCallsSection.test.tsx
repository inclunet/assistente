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

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

describe('ToolCallsSection', () => {
  it('renderiza tool calls ativos', () => {
    render(
      <ToolCallsSection
        activeToolCalls={[{ name: 'Search', callId: '1', status: 'running' }]}
      />
    );

    expect(screen.getByText('Search')).toBeInTheDocument();
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
    fireEvent.click(screen.getByRole('button', { name: /search/i }));
    expect(screen.getByText('prévia')).toBeInTheDocument();
    const detailsButton = screen.getByRole('button', { name: 'chat.showAll' });
    expect(detailsButton).toHaveAttribute('tabindex', '0');
    fireEvent.click(detailsButton);

    await waitFor(() => expect(loadDetails).toHaveBeenCalledWith('user-a', ['inv-1']));
    expect(await screen.findByText('resultado integral')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'chat.showLess' })).toHaveAttribute('aria-expanded', 'true');
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
    fireEvent.click(screen.getByRole('button', { name: /search/i }));
    expect(await axe(container)).toHaveNoViolations();
  });

  it('renderiza origem, servidor e duração da projeção canônica', () => {
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
    expect(screen.getByText('chat.toolOriginMcpNative')).toBeInTheDocument();
    expect(screen.getByText('Atlassian')).toBeInTheDocument();
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

    expect(screen.getByText('chat.toolOriginAcpAgent')).toBeInTheDocument();
    expect(screen.queryByText('chat.toolOriginBuiltin')).not.toBeInTheDocument();
  });

});
