import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { ToolPicker } from './ToolPicker';

const mocks = vi.hoisted(() => ({
  fetchToolCatalog: vi.fn(),
  t: (key: string) => key,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: mocks.t,
  }),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: () => () => {},
}));

vi.mock('../../store/jobStore', () => ({
  useJobStore: (selector: (state: { fetchToolCatalog: typeof mocks.fetchToolCatalog }) => unknown) =>
    selector({ fetchToolCatalog: mocks.fetchToolCatalog }),
}));

vi.mock('./BasePicker', () => ({
  BasePicker: (props: {
    items: Array<{ value: string; label: string; sublabel?: string; disabled?: boolean }>;
    onSelect: (value: string) => void;
  }) => (
    <div>
      {props.items.map((item) => (
        <button
          key={item.value}
          type="button"
          disabled={item.disabled}
          data-testid={`tool-${item.value}`}
          data-sublabel={item.sublabel ?? ''}
          onClick={() => props.onSelect(item.value)}
        >
          {item.label}
        </button>
      ))}
    </div>
  ),
}));

describe('ToolPicker', () => {
  it('renders unavailable catalog entries as disabled and does not select them', async () => {
    mocks.fetchToolCatalog.mockResolvedValue([
      {
        name: 'mcp_jira__create_issue',
        description: 'server disconnected',
        source: 'mcp',
        availability_status: 'unavailable',
        availability_reason: 'server disconnected',
      },
    ]);
    const onChange = vi.fn();

    render(<ToolPicker value="" onChange={onChange} />);

    const item = await screen.findByTestId('tool-mcp_jira__create_issue');
    await waitFor(() => expect(mocks.fetchToolCatalog).toHaveBeenCalled());

    expect(item).toBeDisabled();
    expect(item).toHaveAttribute('data-sublabel', '[mcp] jobs.builder.toolUnavailableServerDisconnected');
    fireEvent.click(item);
    expect(onChange).not.toHaveBeenCalled();
  });
});
