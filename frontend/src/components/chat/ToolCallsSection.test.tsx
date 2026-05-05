import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ToolCallsSection } from './ToolCallsSection';

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

  it('renderiza tool calls historicos e alterna resultado', () => {
    const longResult = 'a'.repeat(350);
    const toolCallsJson = JSON.stringify([
      {
        id: '1',
        type: 'function',
        function: { name: 'fetch', arguments: '{"q":1}' },
        result: longResult,
      },
    ]);

    render(<ToolCallsSection toolCallsJson={toolCallsJson} />);

    fireEvent.click(screen.getByRole('button'));
    expect(screen.getAllByText('fetch')).toHaveLength(2);
    expect(screen.getByRole('button', { name: /chat.showAll/i })).toBeInTheDocument();
  });

  it('renderiza badges de origem e duração (AEP-0039 Fase 5)', () => {
    const toolCallsJson = JSON.stringify([
      {
        id: '1',
        type: 'function',
        function: { name: 'read_file', arguments: '{"path":"main.go"}' },
        result: 'conteúdo do arquivo',
        origin: 'builtin',
        duration_ms: 250,
      },
      {
        id: '2',
        type: 'function',
        function: { name: 'jira_search', arguments: '{"q":"bug"}' },
        result: '3 issues found',
        origin: 'mcp_native',
        server_label: 'Atlassian',
        iteration: 1,
        duration_ms: 1500,
      },
    ]);

    render(<ToolCallsSection toolCallsJson={toolCallsJson} />);

    // Expande a seção
    fireEvent.click(screen.getByRole('button'));

    // Verifica badges de origem
    expect(screen.getByText('chat.toolOriginBuiltin')).toBeInTheDocument();
    expect(screen.getByText('chat.toolOriginMcpNative')).toBeInTheDocument();

    // Verifica server label
    expect(screen.getByText('Atlassian')).toBeInTheDocument();

    // Verifica duração formatada
    expect(screen.getByText('250ms')).toBeInTheDocument();
    expect(screen.getByText('1.5s')).toBeInTheDocument();
  });

  it('não renderiza badges quando metadata ausente (retrocompatível)', () => {
    const toolCallsJson = JSON.stringify([
      {
        id: '1',
        type: 'function',
        function: { name: 'old_tool', arguments: '{}' },
        result: 'ok',
      },
    ]);

    render(<ToolCallsSection toolCallsJson={toolCallsJson} />);
    fireEvent.click(screen.getByRole('button'));

    // Não deve haver badges
    expect(screen.queryByText('chat.toolOriginBuiltin')).not.toBeInTheDocument();
    expect(screen.queryByText('chat.toolOriginMcpNative')).not.toBeInTheDocument();
    expect(screen.queryByText('chat.toolOriginMcpBridge')).not.toBeInTheDocument();
  });

  it('conta tool calls historicos grandes sem parse inicial completo', () => {
    const toolCallsJson = JSON.stringify([
      {
        id: '1',
        type: 'function',
        function: { name: 'first_tool', arguments: '{}' },
        result: 'a'.repeat(5_000),
      },
      {
        id: '2',
        type: 'function',
        function: { name: 'second_tool', arguments: '{}' },
        result: 'b'.repeat(5_000),
      },
      {
        id: '3',
        type: 'function',
        function: { name: 'third_tool', arguments: '{}' },
        result: 'c'.repeat(5_000),
      },
    ]);

    render(<ToolCallsSection toolCallsJson={toolCallsJson} />);

    expect(screen.getByText('3 chat.toolsUsed')).toBeInTheDocument();
    expect(screen.getByText('chat.toolDetails')).toBeInTheDocument();
    expect(screen.queryByText('first_tool')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button'));

    expect(screen.getByText('first_tool')).toBeInTheDocument();
    expect(screen.getByText('second_tool')).toBeInTheDocument();
    expect(screen.getByText('third_tool')).toBeInTheDocument();
  });
});
