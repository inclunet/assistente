import { useState } from 'react';
import { Modal } from '../ui/Modal';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ProviderForm } from './ProviderForm';
import { CreateLLMProvider, ListModelsRaw } from '@wailsjs/go/wailsapi/LLMProviders';
import { GetCredentialForURL, UpsertCredential } from '@wailsjs/go/wailsapi/Credentials';
vi.mock('react-i18next', () => ({ initReactI18next: { type: '3rdParty', init: () => {} }, useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
const { announce } = vi.hoisted(() => ({ announce: vi.fn() }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce }) }));
vi.mock('@wailsjs/go/wailsapi/LLMProviders', () => ({ ListModelsRaw: vi.fn(), CreateLLMProvider: vi.fn(), UpdateLLMProvider: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Credentials', () => ({ GetCredentialForURL: vi.fn(), ListCredentials: vi.fn(async () => []), ListExternalSources: vi.fn(async () => []), UpsertCredential: vi.fn() }));
beforeEach(() => {
 vi.clearAllMocks();
 vi.mocked(GetCredentialForURL).mockResolvedValue(null!);
 vi.mocked(ListModelsRaw).mockResolvedValue(['model']);
 vi.mocked(CreateLLMProvider).mockResolvedValue({});
});
const configure = async () => {
 const button = screen.getByRole('button', { name: 'credentials.mcp.configure' });
 await waitFor(() => expect(button).toBeEnabled());
 await userEvent.click(button);
};
const set = async (label: string, value: string) => { await act(async () => { fireEvent.change(screen.getByLabelText(new RegExp(label)), { target: { value } }); }); };
describe('ProviderForm shared credentials', () => {
 it.each(['static', 'env', 'keyring', 'command'])('tests and saves a %s draft through the provider transaction only', async source => {
  const saved = vi.fn();
  render(<ProviderForm onSave={saved} onCancel={() => {}} />);
  expect(ListModelsRaw).not.toHaveBeenCalled();
  await set('providerForm.name', 'Gateway');
  await configure();
  await set('credentials.sourceFields.source', source);
  if (source === 'static') await set('credentials.labels.token', 'private-token');
  if (source === 'env') await set('credentials.sourceFields.envName', 'API_TOKEN');
  if (source === 'keyring') await set('credentials.sourceFields.keyringName', 'api-target');
  if (source === 'command') await set('credentials.sourceFields.commandName', 'token-command');
  expect(ListModelsRaw).not.toHaveBeenCalled();
  await userEvent.click(screen.getByRole('button', { name: 'providerForm.loadModels' }));
  await screen.findByText('providerForm.connected');
  const preview = vi.mocked(ListModelsRaw).mock.calls[0][0];
  expect(preview.credential).toEqual(expect.objectContaining({ source, pattern: 'api.openai.com', type: 'bearer' }));
  expect(preview.api_key).toBeUndefined();
  await userEvent.click(screen.getByRole('button', { name: 'common.create' }));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(vi.mocked(CreateLLMProvider).mock.calls[0][0].credential).toEqual(preview.credential);
  expect(UpsertCredential).not.toHaveBeenCalled();
 });
 it('does not submit when configuring or keeping a credential', async () => {
  const submitted = vi.fn();
  const { container } = render(<ProviderForm onSave={() => {}} onCancel={() => {}} />);
  container.querySelector('form')!.addEventListener('submit', submitted);
  await configure();
  await userEvent.click(screen.getByRole('button', { name: 'credentials.mcp.keepExisting' }));
  expect(submitted).not.toHaveBeenCalled();
  expect(CreateLLMProvider).not.toHaveBeenCalled();
 });
 it('keeps a valid preview when closed-editor metadata arrives late', async () => {
  let metadata!: (value: Awaited<ReturnType<typeof GetCredentialForURL>>) => void;
  vi.mocked(GetCredentialForURL).mockReturnValue(new Promise(resolve => { metadata = resolve; }));
  render(<ProviderForm onSave={() => {}} onCancel={() => {}} />);
  await set('providerForm.name', 'API');
  await userEvent.click(screen.getByRole('button', { name: 'providerForm.loadModels' }));
  await screen.findByText('providerForm.connected');
  await act(async () => metadata({ pattern: 'api.openai.com', type: 'bearer', source: 'env', sourceConfig: { env: 'SAVED_TOKEN' } } as Awaited<ReturnType<typeof GetCredentialForURL>>));
  expect(screen.getByText('providerForm.connected')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'common.create' })).toBeEnabled();
 });
 it.each(['env', 'keyring'])('keeps a valid preview when returning focus to %s', async source => {
  render(<ProviderForm onSave={() => {}} onCancel={() => {}} />);
  await set('providerForm.name', 'API');
  await configure();
  await set('credentials.sourceFields.source', source);
  await set('credentials.sourceFields.' + source + 'Name', 'SAVED_TOKEN');
  await userEvent.click(screen.getByRole('button', { name: 'providerForm.loadModels' }));
  await screen.findByText('providerForm.connected');
  await userEvent.click(screen.getByLabelText('credentials.sourceFields.' + source + 'Name'));
  expect(screen.getByText('providerForm.connected')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'common.create' })).toBeEnabled();
 });
 it('announces validation once and associates the invalid credential field', async () => {
  render(<ProviderForm onSave={() => {}} onCancel={() => {}} />);
  await configure();
  const token = screen.getByLabelText('credentials.labels.token');
  expect(token).toHaveAttribute('aria-invalid', 'true');
  expect(document.getElementById(token.getAttribute('aria-describedby')!)).toHaveTextContent('credentials.sourceFields.required');
  expect(announce.mock.calls.filter(call => call[0] === 'credentials.sourceFields.required')).toHaveLength(1);
  await set('credentials.labels.token', 'valid');
  expect(token).not.toHaveAttribute('aria-invalid');
  await set('credentials.sourceFields.source', 'command');
  await set('credentials.sourceFields.commandName', 'token-command');
  await set('credentials.sourceFields.args', '{invalid');
  const args = screen.getByLabelText('credentials.sourceFields.args');
  expect(args).toHaveAttribute('aria-invalid', 'true');
  expect(document.getElementById(args.getAttribute('aria-describedby')!)).toHaveTextContent('credentials.sourceFields.invalidArgs');
  await userEvent.click(args);
  expect(announce.mock.calls.filter(call => call[0] === 'credentials.sourceFields.invalidArgs')).toHaveLength(1);
 });
 it('waits for the backend preview beyond fifteen seconds', async () => {
  vi.useFakeTimers();
  try {
   let complete!: (value: string[]) => void;
   vi.mocked(ListModelsRaw).mockReturnValue(new Promise(resolve => { complete = resolve; }));
   render(<ProviderForm onSave={() => {}} onCancel={() => {}} />);
   await act(async () => {});
   fireEvent.click(screen.getByRole('button', { name: 'providerForm.loadModels' }));
   await act(async () => { await vi.advanceTimersByTimeAsync(20000); });
   expect(screen.getByRole('button', { name: 'providerForm.loadModels' })).toBeDisabled();
   expect(screen.getByRole('button', { name: 'providerForm.loadModels' })).toHaveTextContent('providerForm.loadingModels');
   await act(async () => complete(['model']));
   expect(screen.getByText('providerForm.connected')).toBeInTheDocument();
  } finally { vi.useRealTimers(); }
 });
 it('invalidates testing on source changes and discards a late result after URL change', async () => {
  let resolve!: (models: string[]) => void;
  vi.mocked(ListModelsRaw).mockReturnValue(new Promise(done => { resolve = done; }));
  render(<ProviderForm onSave={() => {}} onCancel={() => {}} />);
  await set('providerForm.providerType', 'custom');
  await set('Base URL', 'https://one.example/v1');
  await userEvent.click(screen.getByRole('button', { name: 'providerForm.loadModels' }));
  await set('Base URL', 'https://two.example/v1');
  await act(async () => resolve(['old-model']));
  expect(screen.queryByText('providerForm.connected')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'common.create' })).toBeDisabled();
  vi.mocked(ListModelsRaw).mockResolvedValue(['current-model']);
  await userEvent.click(screen.getByRole('button', { name: 'providerForm.loadModels' }));
  await screen.findByText('providerForm.connected');
  await configure();
  await set('credentials.sourceFields.source', 'env');
  await set('credentials.sourceFields.envName', 'OTHER_TOKEN');
  expect(screen.getByRole('button', { name: 'common.create' })).toBeDisabled();
 });
 it('keeps editing and cancellation blocked until the save resolves', async () => {
  let resolve!: (value: Record<string, never>) => void;
  vi.mocked(CreateLLMProvider).mockReturnValue(new Promise(done => { resolve = done; }));
  const busy = vi.fn(); const saved = vi.fn();
  render(<ProviderForm onSave={saved} onCancel={() => {}} onBusyChange={busy} />);
  await set('providerForm.name', 'Gateway');
  await userEvent.click(screen.getByRole('button', { name: 'providerForm.loadModels' }));
  await screen.findByText('providerForm.connected');
  await userEvent.click(screen.getByRole('button', { name: 'common.create' }));
  expect(screen.getByLabelText(/providerForm.name/)).toBeDisabled();
  expect(screen.getByRole('button', { name: 'common.cancel' })).toBeDisabled();
  expect(busy).toHaveBeenLastCalledWith(true);
  await act(async () => resolve({}));
  expect(saved).toHaveBeenCalledOnce();
  expect(busy).toHaveBeenLastCalledWith(false);
 });
});

it('preserves a typed credential across required/optional and clears it for none', async () => {
 render(<ProviderForm onSave={() => {}} onCancel={() => {}} />);
 await configure();
 await set('credentials.labels.token', 'unsaved-token');
 for (const mode of ['optional', 'required']) {
  await userEvent.click(screen.getByRole('button', { name: 'providerForm.loadModels' }));
  await screen.findByText('providerForm.connected');
  await set('providerForm.authMode', mode);
  expect(screen.getByLabelText('credentials.labels.token')).toHaveValue('unsaved-token');
  expect(screen.queryByText('providerForm.connected')).not.toBeInTheDocument();
 }
 await set('providerForm.authMode', 'none');
 expect(screen.queryByLabelText('credentials.labels.token')).not.toBeInTheDocument();
 await set('providerForm.authMode', 'required');
 await configure();
 expect(screen.getByLabelText('credentials.labels.token')).toHaveValue('');
});

it('contains focus in the real modal during save and restores it on failure', async () => {
 const visibility = vi.spyOn(HTMLElement.prototype, 'offsetParent', 'get').mockReturnValue(document.body);
 let reject!: (reason: Error) => void;
 vi.mocked(CreateLLMProvider).mockReturnValue(new Promise((_, fail) => { reject = fail; }));
 const close = vi.fn();
 function Harness() {
  const [busy, setBusy] = useState(false);
  return <Modal isOpen title="Provider" onClose={close} allowClose={!busy}><ProviderForm onSave={() => {}} onCancel={close} onBusyChange={setBusy} /></Modal>;
 }
 try {
  render(<Harness />);
  await set('providerForm.name', 'API');
  await userEvent.click(screen.getByRole('button', { name: 'providerForm.loadModels' }));
  await screen.findByText('providerForm.connected');
  const save = screen.getByRole('button', { name: 'common.create' });
  await userEvent.click(save);
  const status = screen.getAllByText('common.saving').find(el => el.tagName === 'P')!;
  await waitFor(() => expect(status).toHaveFocus());
  await userEvent.tab();
  expect(status).toHaveFocus();
  await userEvent.tab({ shift: true });
  expect(status).toHaveFocus();
  await act(async () => reject(new Error('save failed')));
  await waitFor(() => expect(save).toHaveFocus());
  expect(screen.getByLabelText(/providerForm.name/)).toBeEnabled();
  expect(close).not.toHaveBeenCalled();
 } finally { visibility.mockRestore(); }
});
