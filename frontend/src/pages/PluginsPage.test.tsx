import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from 'vitest-axe';
import PluginsPage from './PluginsPage';
import * as navigation from '../lib/commandLandmarkNavigation';

const mocks = vi.hoisted(() => ({ createList: vi.fn(), repos: vi.fn(), catalogPreview: vi.fn(), lifecycle: vi.fn(), sync: vi.fn(), register: vi.fn(), remove: vi.fn(), confirm: vi.fn(), list: vi.fn(), components: vi.fn(), toggle: vi.fn(), preview: vi.fn(), install: vi.fn(), discard: vi.fn(), announce: vi.fn(), announceRequest: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Tasklist', () => ({GetAllTaskLists: async () => []}));
vi.mock('@wailsjs/go/wailsapi/Plugins', () => ({ BuilderSkillNames: async () => [], CreateListFromTemplate: mocks.createList, ListRepositories: mocks.repos, PreviewRepositoryEntry: mocks.catalogPreview, ChangeInstallation: mocks.lifecycle, SyncMCP: mocks.sync, RegisterMCP: mocks.register, RemoveMCP: mocks.remove, InstalledComponents: mocks.components, SetSkillActive: mocks.toggle, DiscardPreview: mocks.discard, ListInstalled: mocks.list, PreviewLocal: mocks.preview, InstallReviewed: mocks.install }));
vi.mock('../hooks/useConfirm', () => ({ useConfirm: () => mocks.confirm }));
vi.mock('../store/authStore', async importOriginal => {
  const actual = await importOriginal<typeof import('../store/authStore')>();
  return { ...actual, useAuthStore: Object.assign(() => 'session', actual.useAuthStore) };
});
vi.mock('../hooks/useAnnouncer', () => ({ announce: mocks.announce, useAnnouncer: () => ({ announce: mocks.announce, announceRequest: mocks.announceRequest }) }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, options?: {name?: string}) => ['plugins.registerServer','plugins.keepServer','plugins.removeServer'].includes(key) ? key + ' ' + options?.name : key }) }));

describe('PluginsPage', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    mocks.lifecycle.mockResolvedValue(undefined);
    mocks.list.mockResolvedValue([]);
    mocks.preview.mockResolvedValue({ token: 'reviewed', name: 'demo', origin: 'local', digest: '123', files: ['plugin.json'], diagnostics: [] });
    mocks.install.mockResolvedValue({});
    mocks.discard.mockResolvedValue(undefined);
  });

  it('registers a focus target only while the builder form is available', async () => {
    const register = vi.spyOn(navigation, 'registerLandmarkNavigationSurface');
    try {
      const user = userEvent.setup();
      render(<PluginsPage />);
      const target = register.mock.calls.at(-1)?.[0].read().landmarks.find(l => l.id === 'plugin-builder');
      expect(target?.isAvailable?.()).toBe(false);
      await user.click(await screen.findByRole('tab', {name:'plugins.builder'}));
      const name = await screen.findByRole('textbox', {name:'plugins.packageName'});
      await waitFor(() => expect(name).toBeEnabled());
      expect(target?.focus()).toBe(true);
      expect(name).toHaveFocus();
      expect(target?.contains()).toBe(true);
      await user.click(screen.getByRole('tab', {name:'plugins.installed'}));
      expect(target?.focus()).toBe(false);
    } finally { register.mockRestore(); }
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

  it('informa que a alteração foi salva quando a atualização dos detalhes falha', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', origin: 'folder', version: 'v1', digest: 'sha' }]);
    const note = { name: 'note', key: 'skills/note', kind: 'skill', active: false };
    mocks.components.mockResolvedValueOnce([note]).mockRejectedValueOnce(new Error('offline'));
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', { name: /Demo/ });
    cell.focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.activate' }));
    expect(await screen.findByText('plugins.componentReloadError')).toBeVisible();
    expect(screen.queryByRole('button', { name: 'plugins.activate' })).toBeNull();
    expect(mocks.toggle).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(cell).toHaveFocus());
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

  it('mostra o endereço disponível na prévia e nos detalhes de um MCP que exige configuração', async () => {
    const user = userEvent.setup();
    const endpoint = 'https://example.org/search';
    const server = { kind: 'mcp', key: 'mcp/search', name: 'search', reason: 'configuration', transport: 'sse', endpoint };
    mocks.preview.mockResolvedValueOnce({ token: 'reviewed', name: 'demo', files: [], components: [server] });
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', digest: 'sha' }]);
    mocks.components.mockResolvedValue([server]);
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', { name: /Demo/ });
    await user.click(screen.getByRole('button', { name: 'plugins.choose' }));
    expect(await screen.findByText('sse · ' + endpoint)).toBeVisible();
    expect(screen.getByText('plugins.mcpConfigRequired')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'common.cancel' }));
    cell.focus();
    await user.keyboard('{Enter}');
    expect(await screen.findByText('sse · ' + endpoint)).toBeVisible();
    expect(screen.getByText('plugins.mcpConfigRequired')).toBeVisible();
    expect(screen.queryByRole('button', { name: /plugins.registerServer/ })).toBeNull();
    expect(mocks.register).not.toHaveBeenCalled();
  });

  it.each([undefined, 'frontmatter'])('informa a política na prévia e nos detalhes com requisito %s', async reason => {
    const user = userEvent.setup();
    const skill = { kind: 'skill', key: 'skills/build', name: 'build', requestsShell: true, reason };
    mocks.preview.mockResolvedValueOnce({ token: 'reviewed', name: 'demo', files: [], components: [skill] });
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', digest: 'sha' }]);
    mocks.components.mockResolvedValue([skill]);
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', { name: /Demo/ });
    await user.click(screen.getByRole('button', { name: 'plugins.choose' }));
    expect(await screen.findByText(/plugins\.commandPolicy/)).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'common.cancel' }));
    cell.focus();
    await user.keyboard('{Enter}');
    expect(await screen.findByText('plugins.commandPolicy')).toBeVisible();
    if (reason) {
      expect(screen.getByText('plugins.fieldsPending')).toBeVisible();
      expect(screen.queryByRole('button', { name: /plugins.activate/ })).toBeNull();
    }
  });

  it('identifica os servidores afetados e explica a desconexão antes de confirmar a atualização', async () => {
    const user = userEvent.setup();
    mocks.preview.mockResolvedValueOnce({ token: 'reviewed', name: 'demo', digest: 'new', previousDigest: 'old', files: [], affectedMcpServers: ['build', 'docs'] });
    render(<PluginsPage />);
    const choose = screen.getByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    expect(await screen.findByText('plugins.affectedServersNotice')).toBeVisible();
    expect(mocks.announceRequest).toHaveBeenCalledWith(expect.objectContaining({
      message: expect.stringContaining('plugins.disconnectUpdate'), announcePriority: 'assertive',
    }));
    expect(screen.getByText('build')).toBeVisible();
    expect(screen.getByText('docs')).toBeVisible();
    expect(mocks.install).not.toHaveBeenCalled();
  });

  it('impede atualizar enquanto a prévia indica um servidor editado e permite reabrir a pasta', async () => {
    const user = userEvent.setup();
    const preview = { token: 'reviewed', name: 'demo', digest: 'new', previousDigest: 'old', files: [] };
    mocks.preview.mockResolvedValueOnce({ ...preview, editedMcpServers: ['custom'] }).mockResolvedValueOnce(preview);
    render(<PluginsPage />);
    const choose = screen.getByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    expect(await screen.findByText('plugins.editedServersNotice')).toBeVisible();
    expect(mocks.announceRequest).toHaveBeenCalledWith(expect.objectContaining({
      message: expect.stringContaining('plugins.editedUpdate'), announcePriority: 'assertive',
    }));
    expect(screen.getByText('custom')).toBeVisible();
    const update = screen.getByRole('button', { name: 'plugins.update' });
    expect(update).toBeDisabled();
    await user.click(update);
    expect(mocks.install).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'common.cancel' }));
    await waitFor(() => expect(choose).toHaveFocus());
    await user.click(choose);
    expect(await screen.findByRole('button', { name: 'plugins.update' })).toBeEnabled();
  });

  it('anuncia uma vez a sincronização confirmada com falha na recarga da lista', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValueOnce([]).mockRejectedValueOnce(new Error('read')).mockResolvedValueOnce([]);
    render(<PluginsPage />);
    const sync = screen.getByRole('button', { name: 'plugins.retrySync' });
    await waitFor(() => expect(sync).toBeEnabled());
    mocks.announce.mockClear();
    await user.click(sync);
    expect(await screen.findByText('plugins.syncReloadError')).toBeVisible();
    expect(screen.queryByText('plugins.empty')).toBeNull();
    expect(mocks.announce.mock.calls).toEqual([['plugins.syncReloadError', 'assertive']]);
    mocks.sync.mockRejectedValueOnce(new Error('offline'));
    mocks.announce.mockClear();
    await user.click(sync);
    expect(await screen.findByText('plugins.syncFailedStale')).toBeVisible();
    expect(screen.queryByText('plugins.syncReloadError')).toBeNull();
    expect(screen.queryByText('plugins.empty')).toBeNull();
    expect(mocks.announce.mock.calls).toEqual([['plugins.syncFailedStale', 'assertive']]);
    expect(mocks.list).toHaveBeenCalledTimes(2);
    await user.click(sync);
    await waitFor(() => expect(mocks.announce).toHaveBeenCalledWith('plugins.synchronized'));
    expect(screen.queryByText('plugins.syncReloadError')).toBeNull();
    expect(screen.getByText('plugins.empty')).toBeVisible();
    expect(screen.queryByText('plugins.syncFailedStale')).toBeNull();
    expect(mocks.sync).toHaveBeenCalledTimes(3);
    expect(mocks.list).toHaveBeenCalledTimes(3);
  });

  it('permite sincronizar após sair da página com um registro já salvo', async () => {
    const user = userEvent.setup();
    const row = { id: 'one', name: 'Demo', digest: 'sha' };
    const component = { kind: 'mcp', key: 'mcp/search', name: 'search', endpoint: 'https://example.org' };
    mocks.list.mockResolvedValue([row]);
    mocks.components.mockResolvedValueOnce([component]).mockResolvedValue([{ ...component, active: true }]);
    mocks.register.mockRejectedValueOnce(new Error('PLUGIN_MCP_SYNC_PENDING'));
    const page = render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.registerServer search' }));
    expect(await screen.findByText('plugins.syncPending')).toBeVisible();
    page.unmount();
    render(<PluginsPage />);
    const sync = screen.getByRole('button', { name: 'plugins.retrySync' });
    await waitFor(() => expect(sync).toBeEnabled());
    await user.click(sync);
    await waitFor(() => expect(mocks.announce).toHaveBeenCalledWith('plugins.synchronized'));
    expect(mocks.register).toHaveBeenCalledOnce();
    expect(mocks.sync).toHaveBeenCalledOnce();
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
    const retry = screen.getByRole('button', { name: 'plugins.retrySync' });
    mocks.sync.mockRejectedValueOnce(new Error('offline'));
    await user.click(retry);
    await waitFor(() => expect(retry).toBeEnabled());
    expect(screen.getByRole('button', { name: 'plugins.registerServer search' })).toBeDisabled();
    await user.click(retry);
    expect(await screen.findByRole('button', { name: 'plugins.keepServer search' })).toBeInTheDocument();
    expect(mocks.sync).toHaveBeenCalledTimes(2);
    expect(mocks.register).toHaveBeenCalledOnce();
  });

  it('removes the selected revision only after confirmation and refreshes the installed list', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValueOnce([{ id: 'one', name: 'Demo', digest: 'sha', origin: '/selected' }]).mockResolvedValue([]);
    mocks.components.mockResolvedValue([]);
    mocks.confirm.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    const remove = await screen.findByRole('button', { name: 'plugins.uninstall' });
    await user.click(remove);
    expect(mocks.lifecycle).not.toHaveBeenCalled();
    expect(remove).toHaveFocus();
    expect(mocks.confirm.mock.calls[0][0].message).toContain('/selected');
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

  it('keeps the committed state and the selected cell when the list reload fails', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValueOnce([{ id: 'one', name: 'Demo', digest: 'sha', origin: '/demo' }])
      .mockRejectedValueOnce(new Error('offline'));
    mocks.components.mockResolvedValue([]);
    mocks.confirm.mockResolvedValue(true);
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', { name: /Demo/ });
    cell.focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.disable' }));
    await screen.findByText('plugins.listError');
    await waitFor(() => expect(cell).toHaveFocus());
    await user.keyboard('{Enter}');
    expect(await screen.findByRole('button', { name: 'plugins.enable' })).toBeEnabled();
  });

  it('waits for a removal across navigation and retains its sync warning until reconciliation', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValueOnce([{ id: 'one', name: 'Demo', digest: 'sha' }]).mockResolvedValue([]);
    mocks.components.mockResolvedValue([]);
    mocks.confirm.mockResolvedValue(true);
    let fail!: () => void;
    mocks.lifecycle.mockImplementationOnce(() => new Promise((_, reject) => {
      fail = () => reject(new Error('PLUGIN_MCP_SYNC_PENDING'));
    }));
    const first = render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.uninstall' }));
    first.unmount();
    const second = render(<PluginsPage />);
    expect(mocks.list).toHaveBeenCalledTimes(1);
    await act(async () => fail());
    expect(await screen.findByText('plugins.syncPending')).toBeVisible();
    second.unmount();
    const third = render(<PluginsPage />);
    expect(await screen.findByText('plugins.syncPending')).toBeVisible();
    const retry = screen.getByRole('button', { name: 'plugins.retrySync' });
    await waitFor(() => expect(retry).toBeEnabled());
    let synchronized!: () => void;
    mocks.sync.mockImplementationOnce(() => new Promise<void>(resolve => { synchronized = resolve; }));
    await user.click(retry);
    third.unmount();
    render(<PluginsPage />);
    expect(await screen.findByText('plugins.syncPending')).toBeVisible();
    await act(async () => synchronized());
    await waitFor(() => expect(screen.queryByText('plugins.syncPending')).toBeNull());
  });

  it('completes a pending removal after a later inventory write succeeds', async () => {
    const user = userEvent.setup();
    const plugin = { id: 'one', name: 'Demo', digest: 'sha', origin: '/demo' };
    mocks.list.mockResolvedValueOnce([plugin]).mockResolvedValue([{ ...plugin, disabled: true }]);
    mocks.components.mockResolvedValue([]);
    mocks.confirm.mockResolvedValue(true);
    mocks.lifecycle.mockRejectedValueOnce(new Error('PLUGIN_MCP_SYNC_PENDING'));
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.disable' }));
    await screen.findByText('plugins.syncPending');
    await waitFor(() => expect(screen.getByRole('button', { name: 'plugins.choose' })).toBeEnabled());
    screen.getByRole('gridcell', { name: /Demo/ }).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.enable' }));
    await waitFor(() => expect(mocks.sync).toHaveBeenCalledWith('session'));
    await waitFor(() => expect(screen.queryByText('plugins.syncPending')).toBeNull());
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
    mocks.createList.mockRejectedValueOnce(new Error('connection')).mockResolvedValueOnce('list-id').mockRejectedValueOnce(new Error('PLUGIN_LIST_CAPACITY'));
    const view = render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', {name:/Demo/});
    cell.focus();
    await user.keyboard('{Enter}');
    const button = await screen.findByRole('button', {name:'plugins.createList'});
    await user.click(button);
    await screen.findByText('plugins.componentError');
    view.unmount();
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', {name:/Demo/})).focus();
    await user.keyboard('{Enter}');
    const retry = await screen.findByRole('button', {name:'plugins.createList'});
    await user.click(retry);
    await screen.findByText('plugins.createdList');
    expect(mocks.createList.mock.calls[1]).toEqual(mocks.createList.mock.calls[0]);
    expect(mocks.createList.mock.calls[1]).toEqual(['one','sha','tasklists/release',expect.any(String)]);
    await user.click(retry);
    await screen.findByText('plugins.listCapacity');
    expect(screen.queryByText('plugins.createdList')).toBeNull();
    expect(mocks.createList.mock.calls[2][3]).not.toBe(mocks.createList.mock.calls[1][3]);
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

  it.each([
    ['offline', 'plugins.installError'],
    ['PLUGIN_REPOSITORY_ARCHIVE_CHANGED', 'plugins.archiveChanged'],
    ['PLUGIN_MCP_SYNC_PENDING', 'plugins.syncPending'],
  ])('reports %s to a new page that waited for the installation', async (cause, message) => {
    const user = userEvent.setup();
    let fail!: () => void;
    mocks.install.mockImplementationOnce(() => new Promise((_, reject) => { fail = () => reject(new Error(cause)); }));
    const { unmount } = render(<PluginsPage />);
    const choose = screen.getByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(await screen.findByRole('button', { name: 'plugins.install' }));
    unmount();
    render(<PluginsPage />);
    await act(async () => fail());
    expect(await screen.findByText(message)).toBeVisible();
    if (cause === 'PLUGIN_MCP_SYNC_PENDING') expect(screen.getByRole('button', { name: 'plugins.retrySync' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'plugins.choose' })).toBeEnabled();
  });
});
