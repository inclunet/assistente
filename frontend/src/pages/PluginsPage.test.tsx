import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from 'vitest-axe';
import PluginsPage from './PluginsPage';

const mocks = vi.hoisted(() => ({ createList: vi.fn(), repos: vi.fn(), catalogPreview: vi.fn(), lifecycle: vi.fn(), sync: vi.fn(), register: vi.fn(), remove: vi.fn(), confirm: vi.fn(), list: vi.fn(), components: vi.fn(), toggle: vi.fn(), preview: vi.fn(), install: vi.fn(), discard: vi.fn(), announce: vi.fn(), announceRequest: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Plugins', () => ({ CreateListFromTemplate: mocks.createList, ListRepositories: mocks.repos, PreviewRepositoryEntry: mocks.catalogPreview, ChangeInstallation: mocks.lifecycle, SyncMCP: mocks.sync, RegisterMCP: mocks.register, RemoveMCP: mocks.remove, InstalledComponents: mocks.components, SetSkillActive: mocks.toggle, DiscardPreview: mocks.discard, ListInstalled: mocks.list, PreviewLocal: mocks.preview, InstallReviewed: mocks.install }));
vi.mock('../hooks/useConfirm', () => ({ useConfirm: () => mocks.confirm }));
vi.mock('../store/authStore', async importOriginal => {
  const actual = await importOriginal<typeof import('../store/authStore')>();
  return { ...actual, useAuthStore: Object.assign(() => 'session', actual.useAuthStore) };
});
vi.mock('../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: mocks.announce, announceRequest: mocks.announceRequest }) }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, options?: {name?: string}) => ['plugins.registerServer','plugins.keepServer','plugins.removeServer'].includes(key) ? key + ' ' + options?.name : key }) }));

describe('PluginsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
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

  it('opens installed components with Enter and applies the selected revision', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', origin: 'folder', version: 'v1', digest: 'sha' }]);
    const note = { name: 'note', key: 'skills/note', kind: 'skill', active: false };
    mocks.components.mockResolvedValueOnce([note]).mockResolvedValueOnce([{ ...note, active: true, slug: 'note-local' }]);
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', { name: /Demo/ });
    cell.focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.activate' }));
    expect(mocks.toggle).toHaveBeenCalledWith('one', 'sha', 'skills/note', true);
    expect(await screen.findByText('/note-local')).toBeInTheDocument();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(cell).toHaveFocus());
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

  it('registers an MCP and preserves the native configuration only after confirmation', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', digest: 'sha' }]);
    const server = { kind: 'mcp', key: 'mcp/search', name: 'search', endpoint: 'https://example.org/mcp' };
    const other = { ...server, key: 'mcp/other', name: 'other' };
    mocks.components.mockResolvedValueOnce([server, other]).mockResolvedValue([{ ...server, active: true, slug: 'plugin-search' }, other]);
    mocks.confirm.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.registerServer search' }));
    expect(mocks.register).toHaveBeenCalledWith('one', 'sha', 'mcp/search');
    const keep = await screen.findByRole('button', { name: 'plugins.keepServer search' });
    await user.click(keep);
    expect(mocks.remove).not.toHaveBeenCalled();
    await user.click(keep);
    expect(mocks.remove).toHaveBeenCalledWith('one', 'sha', 'mcp/search', true);
  });

  it('recovers the persisted registration when both synchronization and the first refresh fail', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', digest: 'sha' }]);
    const server = { kind: 'mcp', key: 'mcp/search', name: 'search', endpoint: 'https://example.org/mcp' };
    mocks.components.mockResolvedValueOnce([server]).mockRejectedValueOnce(new Error('read')).mockResolvedValueOnce([{ ...server, active: true }]);
    mocks.register.mockRejectedValueOnce(new Error('PLUGIN_MCP_SYNC_PENDING'));
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.registerServer search' }));
    expect(await screen.findByText('plugins.syncPendingStale')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'plugins.retrySync' }));
    expect(await screen.findByRole('button', { name: 'plugins.keepServer search' })).toBeInTheDocument();
    expect(mocks.sync).toHaveBeenCalledOnce();
    expect(mocks.register).toHaveBeenCalledOnce();
  });

  it('removes the selected revision only after confirmation and refreshes the installed list', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValueOnce([{ id: 'one', name: 'Demo', digest: 'sha' }]).mockResolvedValue([]);
    mocks.components.mockResolvedValue([]);
    mocks.confirm.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    const remove = await screen.findByRole('button', { name: 'plugins.uninstall' });
    await user.click(remove);
    expect(mocks.lifecycle).not.toHaveBeenCalled();
    await user.click(remove);
    expect(mocks.lifecycle).toHaveBeenCalledWith('one','sha','uninstall');
    expect(await screen.findByText('plugins.empty')).toBeInTheDocument();
  });

  it('shows a failed catalog installation in the active catalog panel', async () => {
    const user = userEvent.setup();
    mocks.repos.mockResolvedValue([{ id: 's', origin: 'folder', revision: 'r', indices: [{ format: 'cursor', name: 'market', entries: [{ name: 'sample' }] }] }]);
    mocks.catalogPreview.mockResolvedValue({ token: 'catalog', name: 'sample', files: [], digest: 's' });
    mocks.install.mockRejectedValueOnce(new Error('failure'));
    render(<PluginsPage />);
    await user.click(await screen.findByRole('tab', { name: 'plugins.catalog' }));
    const cell = await screen.findByRole('gridcell', { name: /sample/ });
    cell.focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.install' }));
    expect(await screen.findByText('plugins.installError')).toBeVisible();
    expect(screen.getByRole('tab', { name: 'plugins.catalog' })).toHaveAttribute('aria-selected', 'true');
    await waitFor(() => expect(cell).toHaveFocus());
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
  it('reuses the request when retrying creation from a task list template', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{id: 'one', name: 'Demo', digest: 'sha'}]);
    mocks.components.mockResolvedValue([{name:'Release',key:'tasklists/release',kind:'tasklistTemplate'}]);
    mocks.createList.mockRejectedValueOnce(new Error('connection')).mockResolvedValueOnce('list-id');
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', {name:/Demo/});
    cell.focus();
    await user.keyboard('{Enter}');
    const button = await screen.findByRole('button', {name:'plugins.createList'});
    await user.click(button);
    await waitFor(() => expect(mocks.createList).toHaveBeenCalledTimes(1));
    await user.click(button);
    await screen.findByText('plugins.createdList');
    expect(mocks.createList.mock.calls[1]).toEqual(mocks.createList.mock.calls[0]);
    expect(mocks.createList.mock.calls[1]).toEqual(['one','sha','tasklists/release',expect.any(String)]);
  });
  it('distinguishes an invalid profile from a valid profile awaiting activation', async () => {
    mocks.list.mockResolvedValue([{id: 'one', name: 'Demo', digest: 'sha'}]);
    mocks.components.mockResolvedValue([
      {name: 'Writer', key: 'profiles/writer', kind: 'profile', reason: 'profileActivationPending'},
      {name: 'broken', key: 'profiles/broken', kind: 'profile', reason: 'invalidProfile'},
    ]);
    const user = userEvent.setup();
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', {name: /Demo/});
    cell.focus();
    await user.keyboard('{Enter}');
    await screen.findByText('plugins.profileActivationPending');
    expect(screen.getByText('plugins.invalidProfile')).toBeVisible();
    expect(screen.queryByRole('button', {name: 'plugins.activate'})).toBeNull();
  });

  it('reports a supported template in preview before installation', async () => {
    mocks.preview.mockResolvedValue({token:'reviewed',name:'planner',digest:'sha',files:[],diagnostics:[],components:[{kind:'tasklistTemplate',key:'tasklists/release',name:'Release'}]});
    const user = userEvent.setup();
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', {name:'plugins.choose'});
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await screen.findByText(/plugins.tasklistTemplate/);
    expect(screen.queryByText('plugins.unavailable')).toBeNull();
  });
});
