import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HistoryPicker } from './HistoryPicker';

const getConversationsMock = vi.hoisted(() => vi.fn());

vi.mock('@wailsjs/go/app/App', () => ({
  GetConversations: getConversationsMock,
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: () => () => {},
}));

describe('HistoryPicker', () => {
  beforeEach(() => {
    getConversationsMock.mockReset();
    getConversationsMock.mockResolvedValue([
      { id: '5', title: 'Conversa X', updatedAt: '2024-01-02', message_count: 3 },
    ]);
  });

  it('chama onChange ao selecionar uma conversa', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<HistoryPicker label="Hist" onChange={onChange} />);

    await user.click(screen.getByRole('button'));
    const option = await screen.findByRole('option', { name: /Conversa X/ });
    fireEvent.mouseDown(option);

    expect(onChange).toHaveBeenCalledWith('5', expect.objectContaining({ id: '5' }));
  });

  it('chama onSelectExtra ao selecionar um extraItem ("Nenhuma")', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const onSelectExtra = vi.fn();
    render(
      <HistoryPicker
        label="Hist"
        value="5"
        onChange={onChange}
        onSelectExtra={onSelectExtra}
        extraItems={[{ value: '__none__', label: 'Nenhuma' }]}
      />,
    );

    await user.click(screen.getByRole('button'));
    const noneOption = await screen.findByRole('option', { name: 'Nenhuma' });
    fireEvent.mouseDown(noneOption);

    expect(onSelectExtra).toHaveBeenCalledWith('__none__');
    expect(onChange).not.toHaveBeenCalled();
  });
});
