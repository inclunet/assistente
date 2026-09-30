import { act, render, screen, waitFor } from '@testing-library/react';
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
  it.each(['oauth_vault_persistence_required', 'oauth_vault_unavailable'])('explica cofre indisponível ao desconectar: %s', async code => {
    mocks.disconnect.mockRejectedValueOnce(code === 'oauth_vault_unavailable' ? new Error(code) : code);
    render(<ChatGPTConnection id="authorization" onChanged={vi.fn()} onClose={vi.fn()} />);
    await screen.findByRole('button', { name: 'chatgpt.reconnect' });
    await userEvent.click(screen.getByRole('button', { name: 'chatgpt.disconnect' }));
    expect(await screen.findByText('chatgpt.vaultUnavailable')).toBeInTheDocument();
    expect(mocks.announce).toHaveBeenCalledWith('chatgpt.vaultUnavailable', 'assertive');
    expect(screen.getByRole('button', { name: 'chatgpt.reconnect' })).toBeInTheDocument();
  });

  it.each(['unconfirmed', 'failed'])('keeps disconnection results visible until completion: %s', async outcome => {
    let resolve!: (value: boolean) => void, reject!: (reason: Error) => void;
    mocks.disconnect.mockReturnValueOnce(new Promise((yes, no) => { resolve = yes; reject = no; }));
    const user = userEvent.setup(), close = vi.fn(), blocked = vi.fn(), changed = vi.fn();
    render(<ChatGPTConnection id="authorization" onChanged={changed} onClose={close} onCloseBlockedChange={blocked} />);
    await screen.findByRole('button', { name: 'chatgpt.reconnect' });
    await user.click(screen.getByRole('button', { name: 'chatgpt.disconnect' }));
    expect(blocked).toHaveBeenLastCalledWith(true);
    expect(screen.getByRole('button', { name: 'common.cancel' })).toBeDisabled();
    expect(screen.getByText('chatgpt.operationPending')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'common.cancel' }));
    expect(close).not.toHaveBeenCalled(); expect(changed).not.toHaveBeenCalled();
    await act(async () => { if (outcome === 'unconfirmed') resolve(false); else reject(new Error('failure')); });
    expect(blocked).toHaveBeenLastCalledWith(false);
    expect(changed).toHaveBeenCalledOnce();
    expect(screen.getByRole('button', { name: 'common.close' })).toBeEnabled();
    const message = outcome === 'unconfirmed' ? 'chatgpt.revocationUnconfirmed' : 'chatgpt.connectionError';
    expect(screen.getByText(message)).toBeInTheDocument();
    expect(mocks.announce.mock.calls.some(call => call[0] === message)).toBe(true);
  });

  it.each(['resolve', 'reject'])('ignores an obsolete initial query after disconnect: %s', async outcome => {
    let resolve!: (value: unknown) => void, reject!: (reason: Error) => void;
    mocks.get.mockReturnValueOnce(new Promise((yes, no) => { resolve = yes; reject = no; }));
    mocks.disconnect.mockResolvedValueOnce(true);
    const user = userEvent.setup();
    render(<ChatGPTConnection id="provider-id" onChanged={vi.fn()} onClose={vi.fn()} />);
    await user.click(screen.getByRole('button', { name: 'chatgpt.disconnect' }));
    await waitFor(() => expect(mocks.announce).toHaveBeenCalledWith('chatgpt.states.disconnected'));
    mocks.announce.mockClear();
    await act(async () => {
      if (outcome === 'resolve') resolve({ id: 'authorization-id', state: 'connected', email: 'old@example.test' });
      else reject(new Error('obsolete'));
    });
    expect(screen.getByRole('button', { name: 'chatgpt.disconnect' })).toBeDisabled();
    expect(screen.queryByRole('button', { name: 'chatgpt.reconnect' })).not.toBeInTheDocument();
    expect(screen.queryByText('chatgpt.connectionError')).not.toBeInTheDocument();
    expect(mocks.announce).not.toHaveBeenCalled();
    expect(screen.getByText('chatgpt.providerID')).toBeInTheDocument();
  });

  it('blocks closing only while the initial record is being created', async () => {
    let resolveCreate!: (value: {id: string}) => void;
    mocks.create.mockReturnValueOnce(new Promise(resolve => { resolveCreate = resolve; }));
    mocks.authorize.mockReturnValueOnce(new Promise(() => undefined));
    const close = vi.fn(), creation = vi.fn();
    const user = userEvent.setup();
    render(<ChatGPTConnection onChanged={vi.fn()} onClose={close} onCloseBlockedChange={creation} />);
    await user.type(screen.getByLabelText('chatgpt.label'), 'Account');
    await user.click(screen.getByRole('button', { name: 'chatgpt.connect' }));
    expect(creation).toHaveBeenLastCalledWith(true);
    const cancel = screen.getByRole('button', { name: 'common.cancel' });
    expect(cancel).toBeDisabled();
    await user.click(cancel);
    expect(close).not.toHaveBeenCalled();
    await act(async () => resolveCreate({ id: 'issued' }));
    expect(creation).toHaveBeenLastCalledWith(false);
    expect(cancel).toBeEnabled();
    await user.click(cancel);
    expect(close).toHaveBeenCalledOnce();
    expect(mocks.cancel).toHaveBeenCalledWith('issued');
  });

  it('announces the asynchronously loaded state', async () => {
    render(<ChatGPTConnection id="authorization" onChanged={vi.fn()} onClose={vi.fn()} />);
    await waitFor(() => expect(mocks.announce).toHaveBeenCalledWith('chatgpt.states.connected'));
  });
  it('announces an initial load failure assertively', async () => {
    mocks.get.mockRejectedValueOnce(new Error('load'));
    render(<ChatGPTConnection id="authorization" onChanged={vi.fn()} onClose={vi.fn()} />);
    await waitFor(() => expect(mocks.announce).toHaveBeenCalledWith('chatgpt.connectionError', 'assertive'));
  });

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
