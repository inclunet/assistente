import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ChatGPTConnection } from './ChatGPTConnection';
const mocks = vi.hoisted(() => ({ create: vi.fn(), authorize: vi.fn(), cancel: vi.fn(), get: vi.fn(), disconnect: vi.fn(), announce: vi.fn(), userID: undefined as string | undefined, t: (key: string) => key }));
vi.mock('../../store/authStore', () => ({ useAuthStore: () => mocks.userID }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: mocks.t }) }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: mocks.announce }) }));
vi.mock('@wailsjs/go/wailsapi/LLMProviders', () => ({ CreateChatGPTConnection: mocks.create, AuthorizeChatGPT: mocks.authorize, CancelChatGPT: mocks.cancel, ChatGPTConnection: mocks.get, DisconnectChatGPT: mocks.disconnect }));
beforeEach(() => {
  vi.clearAllMocks();
  mocks.userID = undefined;
  mocks.create.mockResolvedValue({ id: 'authorization' });
  mocks.authorize.mockResolvedValue({ id: 'authorization', state: 'connected', email: 'user@example.test' });
  mocks.cancel.mockResolvedValue(undefined);
  mocks.get.mockResolvedValue({ id: 'authorization', state: 'connected', email: 'user@example.test' });
});
describe('ChatGPT connection', () => {
  it('announces disconnection failure assertively', async () => {
    mocks.disconnect.mockRejectedValue(new Error('network'));
    const user = userEvent.setup();
    render(<ChatGPTConnection id="authorization" onChanged={vi.fn()} onClose={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'chatgpt.disconnect' }));
    await waitFor(() => expect(mocks.announce).toHaveBeenCalledWith('chatgpt.connectionError', 'assertive'));
  });

  it('only authorizes after an explicit action and retains the issued registration on reconnect', async () => {
    const user = userEvent.setup();
    render(<ChatGPTConnection onChanged={vi.fn()} onClose={vi.fn()} />);
    expect(mocks.authorize).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'chatgpt.connect' })).toBeDisabled();
    await user.type(screen.getByLabelText('chatgpt.label'), 'Work account');
    await user.click(screen.getByRole('button', { name: 'chatgpt.connect' }));
    await screen.findByRole('button', { name: 'chatgpt.reconnect' });
    expect(mocks.create).toHaveBeenCalledWith('Work account');
    expect(mocks.authorize).toHaveBeenCalledWith('authorization', 'chatgpt.browserReturn');
    await user.click(screen.getByRole('button', { name: 'chatgpt.reconnect' }));
    await waitFor(() => expect(mocks.authorize).toHaveBeenCalledTimes(2));
    expect(mocks.create).toHaveBeenCalledTimes(1);
  });
  it('cancels a pending browser flow when unmounted', async () => {
    mocks.authorize.mockReturnValue(new Promise(() => undefined));
    const user = userEvent.setup();
    const view = render(<ChatGPTConnection id="authorization" onChanged={vi.fn()} onClose={vi.fn()} />);
    await waitFor(() => expect(mocks.get).toHaveBeenCalled());
    await user.click(await screen.findByRole('button', { name: 'chatgpt.reconnect' }));
    view.unmount();
    expect(mocks.cancel).toHaveBeenCalledWith('authorization');
  });
  it('warns when remote revocation is unconfirmed', async () => {
    mocks.disconnect.mockResolvedValue(false);
    const user = userEvent.setup();
    render(<ChatGPTConnection id="authorization" onChanged={vi.fn()} onClose={vi.fn()} />);
    await screen.findByRole('button', { name: 'chatgpt.reconnect' });
    await user.click(screen.getByRole('button', { name: 'chatgpt.disconnect' }));
    expect(await screen.findByText('chatgpt.revocationUnconfirmed')).toBeInTheDocument();
    expect(mocks.announce).toHaveBeenCalledWith('chatgpt.revocationUnconfirmed');
  });
  it('allows disconnecting a pending registration so it can be deleted', async () => {
    mocks.get.mockResolvedValue({ id: 'authorization', state: 'pending' });
    mocks.disconnect.mockResolvedValue(true);
    const user = userEvent.setup();
    render(<ChatGPTConnection id="authorization" onChanged={vi.fn()} onClose={vi.fn()} />);
    const disconnect = screen.getByRole('button', { name: 'chatgpt.disconnect' });
    expect(disconnect).toBeEnabled();
    await user.click(disconnect);
    await waitFor(() => expect(mocks.disconnect).toHaveBeenCalledWith('authorization'));
    expect(mocks.authorize).not.toHaveBeenCalled();
  });

  it('shows the plan welcome once for the local user', async () => {
    mocks.userID = 'welcome-test-user';
    localStorage.removeItem('chatgpt-plan-notice:welcome-test-user');
    const user = userEvent.setup();
    render(<ChatGPTConnection id="authorization" onChanged={vi.fn()} onClose={vi.fn()} />);
    await user.click(await screen.findByRole('button', { name: 'chatgpt.reconnect' }));
    const gotIt = await screen.findByRole('button', { name: 'chatgpt.gotIt' });
    expect(gotIt).toHaveFocus();
    await user.click(gotIt);
    expect(screen.getByRole('button', { name: 'chatgpt.reconnect' })).toHaveFocus();
    await user.click(screen.getByRole('button', { name: 'chatgpt.reconnect' }));
    await waitFor(() => expect(mocks.authorize).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole('button', { name: 'chatgpt.gotIt' })).not.toBeInTheDocument();
  });

});
