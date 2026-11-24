import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from 'vitest-axe';
import PluginsPage from './PluginsPage';

const mocks = vi.hoisted(() => ({ list: vi.fn(), preview: vi.fn(), install: vi.fn(), discard: vi.fn(), announce: vi.fn(), announceRequest: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Plugins', () => ({ DiscardPreview: mocks.discard, ListInstalled: mocks.list, PreviewLocal: mocks.preview, InstallReviewed: mocks.install }));
vi.mock('../store/authStore', async importOriginal => {
  const actual = await importOriginal<typeof import('../store/authStore')>();
  return { ...actual, useAuthStore: Object.assign(() => 'session', actual.useAuthStore) };
});
vi.mock('../hooks/useAnnouncer', () => ({ announce: mocks.announce, useAnnouncer: () => ({ announce: mocks.announce, announceRequest: mocks.announceRequest }) }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

describe('PluginsPage', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    mocks.list.mockResolvedValue([]);
    mocks.preview.mockResolvedValue({ token: 'reviewed', name: 'demo', origin: 'local', digest: '123', files: ['plugin.json'], diagnostics: [] });
    mocks.install.mockResolvedValue({});
    mocks.discard.mockResolvedValue(undefined);
  });

  it('opens a preview from the keyboard and installs only after confirmation', async () => {
    const user = userEvent.setup();
    const { container } = render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    expect(await axe(container)).toHaveNoViolations();
    choose.focus();
    await user.keyboard('{Enter}');
    const install = await screen.findByRole('button', { name: 'plugins.install' });
    const cancel = screen.getByRole('button', { name: 'common.cancel' });
    expect(install.compareDocumentPosition(cancel) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(mocks.install).not.toHaveBeenCalled();
    install.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(mocks.install).toHaveBeenCalledWith('reviewed'));
    expect(mocks.announce).toHaveBeenCalled();
  });

  it('keeps the preview open during refresh and restores focus after publication', async () => {
    const user = userEvent.setup();
    let finish!: () => void;
    mocks.list.mockResolvedValueOnce([]).mockImplementationOnce(() => new Promise(resolve => { finish = () => resolve([]); }));
    mocks.preview.mockResolvedValueOnce({ token: 'revision', name: 'demo', digest: 'new', previousDigest: 'old', files: [], diagnostics: [] });
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    expect(await screen.findByText('old')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'plugins.update' }));
    expect(choose).toBeDisabled();
    expect(screen.getByRole('button', { name: 'plugins.update' })).toBeDisabled();
    finish();
    await waitFor(() => expect(choose).toHaveFocus());
  });

  it('disables publication of the installed revision', async () => {
    const user = userEvent.setup();
    mocks.preview.mockResolvedValueOnce({ token: 'revision', name: 'demo', digest: 'same', previousDigest: 'same', files: [], diagnostics: [] });
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    expect(await screen.findByText('plugins.current')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'plugins.update' })).toBeDisabled();
  });

  it('shows discard errors inside the modal and allows retry', async () => {
    const user = userEvent.setup();
    mocks.discard.mockRejectedValueOnce(new Error('offline'));
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    const cancel = await screen.findByRole('button', { name: 'common.cancel' });
    await user.click(cancel);
    expect(screen.getByRole('alertdialog')).toContainElement(await screen.findByText('plugins.discardError'));
    expect(mocks.announce).toHaveBeenCalledWith('plugins.discardError', 'assertive');
    await waitFor(async () => {
      await user.click(cancel);
      expect(choose).toHaveFocus();
    }, { timeout: 2500 });
    expect(mocks.install).not.toHaveBeenCalled();
  });

  it.each(['cancel', 'readError'])('restores keyboard focus after picker result %s', async result => {
    const user = userEvent.setup();
    if (result === 'cancel') mocks.preview.mockResolvedValueOnce(null);
    else mocks.preview.mockRejectedValueOnce(new Error('read'));
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    choose.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(mocks.preview).toHaveBeenCalledOnce());
    await waitFor(() => expect(choose).toHaveFocus());
    expect(screen.queryByRole('alertdialog')).toBeNull();
  });

  it('closes the preview without installing', async () => {
    const user = userEvent.setup();
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(await screen.findByRole('button', { name: 'common.cancel' }));
    expect(mocks.install).not.toHaveBeenCalled();
    expect(mocks.discard).toHaveBeenCalledWith('reviewed');
    expect(choose).toHaveFocus();
    expect(screen.queryByRole('button', { name: 'plugins.install' })).toBeNull();
  });

  it.each(['open', 'pending', 'installing'])('discards only an abandoned preview on unmount (%s)', async phase => {
    const user = userEvent.setup();
    const result = { token: 'old-page', name: 'demo', digest: 'new', files: [], diagnostics: [] };
    let finishPreview!: (value: typeof result) => void;
    mocks.preview.mockImplementationOnce(() => new Promise(resolve => { finishPreview = resolve; }));
    let finishInstall!: () => void;
    if (phase === 'installing') mocks.install.mockImplementationOnce(() => new Promise<void>(resolve => { finishInstall = resolve; }));
    const { unmount } = render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    if (phase !== 'pending') await act(async () => finishPreview(result));
    if (phase === 'installing') await user.click(await screen.findByRole('button', { name: 'plugins.install' }));
    const listsBeforeUnmount = mocks.list.mock.calls.length;
    if (phase === 'installing') expect(mocks.install).toHaveBeenCalledWith('old-page');
    unmount();
    if (phase === 'pending') await act(async () => finishPreview(result));
    if (phase === 'installing') {
      mocks.list.mockResolvedValueOnce([{ id: 'installed', name: 'done', version: '1', origin: 'local' }]);
      render(<PluginsPage />);
      expect(mocks.list).toHaveBeenCalledTimes(listsBeforeUnmount);
      await act(async () => finishInstall());
      expect(await screen.findByText('done')).toBeInTheDocument();
      expect(mocks.discard).not.toHaveBeenCalled();
    } else {
      expect(mocks.discard.mock.calls).toEqual([['old-page']]);
      expect(mocks.install).not.toHaveBeenCalled();
    }
  });

  it('clears a discard error after installation and explains distinct manifest warnings', async () => {
    const user = userEvent.setup();
    mocks.preview.mockResolvedValueOnce({ token: 'warning', name: 'demo', digest: 'new', files: [], diagnostics: ['unknown manifest field: other', 'invalid extensions object'] });
    mocks.discard.mockRejectedValueOnce(new Error('offline'));
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(await screen.findByText('plugins.warnings'));
    expect(screen.getByText('plugins.unknownField')).toBeVisible();
    expect(screen.getByText('plugins.invalidExtensions')).toBeVisible();
    await user.click(screen.getByRole('button', {name:'common.cancel'}));
    expect(await screen.findByText('plugins.discardError')).toBeVisible();
    await waitFor(async () => {
      await user.click(screen.getByRole('button', {name:'plugins.install'}));
      expect(mocks.install).toHaveBeenCalledTimes(1);
    }, {timeout: 2500});
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(screen.queryByText('plugins.discardError')).toBeNull();
  });

  it('keeps list failures distinct from an empty inventory after cancelling the picker', async () => {
    mocks.list.mockRejectedValue(new Error('offline'));
    mocks.preview.mockResolvedValueOnce(null);
    const user = userEvent.setup();
    render(<PluginsPage />);
    await screen.findByText('plugins.listError');
    await user.click(screen.getByRole('button', { name: 'plugins.choose' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'plugins.choose' })).toBeEnabled());
    expect(screen.getByText('plugins.listError')).toBeVisible();
    expect(screen.queryByText('plugins.empty')).toBeNull();
  });

  it('reports failure to a new page that waited for the installation', async () => {
    const user = userEvent.setup();
    let fail!: () => void;
    mocks.install.mockImplementationOnce(() => new Promise((_, reject) => { fail = () => reject(new Error('offline')); }));
    const { unmount } = render(<PluginsPage />);
    const choose = screen.getByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(await screen.findByRole('button', { name: 'plugins.install' }));
    unmount();
    render(<PluginsPage />);
    await act(async () => fail());
    expect(await screen.findByText('plugins.installError')).toBeVisible();
    expect(screen.getByRole('button', { name: 'plugins.choose' })).toBeEnabled();
  });
});
