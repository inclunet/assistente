import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ChatGPTProviderSettings } from './ChatGPTProviderSettings';
const mocks = vi.hoisted(() => ({ get: vi.fn(), models: vi.fn(), update: vi.fn(), setDefault: vi.fn(), announce: vi.fn(), t: (key: string) => key }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: mocks.t }) }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: mocks.announce }) }));
vi.mock('@wailsjs/go/wailsapi/LLMProviders', () => ({ GetLLMProvider: mocks.get, UpdateLLMProvider: mocks.update, SetDefaultProvider: mocks.setDefault }));
vi.mock('@wailsjs/go/wailsapi/LLMModels', () => ({ GetModelsByProvider: mocks.models }));
const props = () => ({ providerId: 'account-1', connected: true, disabled: false, onBusyChange: vi.fn(), onChanged: vi.fn() });
beforeEach(() => {
  vi.clearAllMocks();
  mocks.t = (key: string) => key;
  mocks.get.mockResolvedValue({ name: 'Personal', default_model: 'model-a', is_default: false });
  mocks.models.mockResolvedValue(['model-a', 'model-b']);
  mocks.update.mockResolvedValue({}); mocks.setDefault.mockResolvedValue(undefined);
});
describe('ChatGPT provider preferences', () => {
  it('saves the account model and name without connection fields', async () => {
    const user = userEvent.setup(), p = props();
    render(<ChatGPTProviderSettings {...p} />);
    await screen.findByDisplayValue('Personal');
    await user.selectOptions(screen.getByLabelText('providerForm.defaultModel'), 'model-b');
    await user.clear(screen.getByLabelText('chatgpt.providerName'));
    await user.type(screen.getByLabelText('chatgpt.providerName'), 'Work');
    await user.click(screen.getByRole('button', { name: 'chatgpt.savePreferences' }));
    expect(mocks.update).toHaveBeenCalledWith('account-1', { name: 'Work', default_model: 'model-b' });
    expect(p.onChanged).toHaveBeenCalledOnce();
    expect(p.onBusyChange.mock.calls.map(call => call[0])).toEqual([true, false]);
    expect(mocks.announce).toHaveBeenCalledWith('chatgpt.preferencesSaved');
  });
  it('makes the account default through the existing endpoint', async () => {
    const p = props(); render(<ChatGPTProviderSettings {...p} />);
    await screen.findByDisplayValue('Personal');
    await userEvent.click(screen.getByRole('button', { name: 'providers.actions.setDefault' }));
    expect(mocks.setDefault).toHaveBeenCalledWith('account-1');
    expect(await screen.findByText('providers.badge.default')).toBeInTheDocument();
    expect(mocks.update).not.toHaveBeenCalled();
  });
  it('retains selection and releases controls after save failure', async () => {
    mocks.update.mockRejectedValue(new Error('private backend detail'));
    const p = props(); render(<ChatGPTProviderSettings {...p} />);
    await screen.findByDisplayValue('Personal');
    await userEvent.click(screen.getByRole('button', { name: 'chatgpt.savePreferences' }));
    expect(await screen.findByText('chatgpt.preferencesSaveError')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(mocks.announce).toHaveBeenCalledWith('chatgpt.preferencesSaveError', 'assertive');
    expect(screen.getByLabelText('providerForm.defaultModel')).toHaveValue('model-a');
    expect(p.onChanged).not.toHaveBeenCalled();
    expect(p.onBusyChange).toHaveBeenLastCalledWith(false);
    expect(screen.queryByText('private backend detail')).not.toBeInTheDocument();
  });
  it('does not list models for a disconnected account', async () => {
    render(<ChatGPTProviderSettings {...props()} connected={false} />);
    await screen.findByDisplayValue('Personal');
    expect(mocks.models).not.toHaveBeenCalled();
    expect(screen.getByLabelText('providerForm.defaultModel')).toBeDisabled();
  });
  it('blocks changes if provider metadata cannot be loaded', async () => {
    mocks.get.mockRejectedValueOnce(new Error('private'));
    render(<ChatGPTProviderSettings {...props()} />);
    await screen.findByText('chatgpt.preferencesLoadError');
    expect(screen.getByRole('button', { name: 'providers.actions.setDefault' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'chatgpt.savePreferences' })).toBeDisabled();
  });
  it('does not overwrite preferences with late data from another account', async () => {
    let complete!: (value: unknown) => void;
    mocks.get.mockReturnValueOnce(new Promise(resolve => { complete = resolve; }));
    const p = props(), view = render(<ChatGPTProviderSettings {...p} />);
    view.rerender(<ChatGPTProviderSettings {...p} providerId="account-2" />);
    await screen.findByDisplayValue('Personal');
    await act(async () => complete({ name: 'Old account', default_model: 'old-model' }));
    expect(screen.queryByDisplayValue('Old account')).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByLabelText('providerForm.defaultModel')).toHaveValue('model-a'));
  });
  it('preserves the draft and releases saving when the language changes', async () => {
    let finish!: () => void;
    mocks.update.mockReturnValueOnce(new Promise<void>(resolve => { finish = resolve; }));
    const p = props(), view = render(<ChatGPTProviderSettings {...p} />);
    await screen.findByDisplayValue('Personal');
    await userEvent.clear(screen.getByLabelText('chatgpt.providerName'));
    await userEvent.type(screen.getByLabelText('chatgpt.providerName'), 'Draft');
    await userEvent.click(screen.getByRole('button', { name: 'chatgpt.savePreferences' }));
    mocks.t = (key: string) => `translated:${key}`;
    view.rerender(<ChatGPTProviderSettings {...p} />);
    expect(screen.getByDisplayValue('Draft')).toBeInTheDocument();
    expect(mocks.get).toHaveBeenCalledOnce();
    await act(async () => finish());
    expect(p.onBusyChange).toHaveBeenLastCalledWith(false);
    expect(screen.getByRole('button', { name: 'translated:chatgpt.savePreferences' })).toBeEnabled();
  });

});
