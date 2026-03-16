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
});
