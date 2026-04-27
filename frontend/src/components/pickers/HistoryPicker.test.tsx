import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { HistoryPicker } from './HistoryPicker';

const getConversationsSpy = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  GetConversations: () => getConversationsSpy(),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: () => () => {},
}));

vi.mock('./BasePicker', () => ({
  BasePicker: (props: { items: Array<{ value: string }>; onSelect: (value: string) => void }) => (
    <div>
      <button onClick={() => props.onSelect(props.items[0]?.value || '')}>Selecionar</button>
      <div data-testid="base-picker" data-items={props.items.length} />
    </div>
  ),
}));

describe('HistoryPicker', () => {
  it('carrega conversas e dispara onChange', async () => {
    getConversationsSpy.mockResolvedValueOnce([
      {
        id: '1',
        title: 'Conversa',
        message_count: 2,
        updated_at: new Date().toISOString(),
      },
    ]);

    const onChange = vi.fn();
    render(<HistoryPicker onChange={onChange} />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '1');
    });

    fireEvent.click(screen.getByRole('button', { name: 'Selecionar' }));

    expect(onChange).toHaveBeenCalledWith('1', expect.objectContaining({ id: '1' }));
  });
});
