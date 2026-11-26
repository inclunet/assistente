import { describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import PluginRepositories from './PluginRepositories';

const mocks = vi.hoisted(() => ({ add: vi.fn(), addRemote: vi.fn(), addGit: vi.fn(), list: vi.fn(), preview: vi.fn(), refresh: vi.fn(), remove: vi.fn(), confirm: vi.fn(), announce: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Plugins', () => ({ AddLocalRepository: mocks.add, AddGitRepository: mocks.addGit, AddRemoteRepository: mocks.addRemote, ListRepositories: mocks.list, RefreshRepository: mocks.refresh, PreviewRepositoryEntry: mocks.preview, RemoveRepository: mocks.remove }));
vi.mock('../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: mocks.announce }) }));
vi.mock('../hooks/useConfirm', () => ({ useConfirm: () => mocks.confirm }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));

const repo = { id: 'one', origin: 'folder', revision: 'reviewed', updated: new Date().toISOString(), indices: [{ format: 'cursor', name: 'sample-store', entries: [
  { name: 'first', version: '1', author: 'alice' }, { name: 'second', diagnostic: 'source' },
] }] };

describe('PluginRepositories', () => {
  it('searches the cached catalog and previews the selected source and revision from the keyboard', async () => {
    mocks.list.mockResolvedValue([repo]);
    mocks.preview.mockResolvedValue({ token: 'once' });
    const onPreview = vi.fn();
    const user = userEvent.setup();
    render(<PluginRepositories session="test" catalog onPreview={onPreview} />);
    expect(await screen.findByText('second')).toBeInTheDocument();
    await user.type(screen.getByLabelText('plugins.searchCatalog'), 'alice');
    expect(screen.queryByText('second')).toBeNull();
    const cell = screen.getByRole('gridcell', { name: /first/ });
    cell.focus();
    await user.keyboard('{Enter}');
    expect(mocks.preview).toHaveBeenCalledWith('one','reviewed','cursor','first');
    expect(onPreview).toHaveBeenCalledWith({ token: 'once' }, cell);
  });

  it('keeps the saved index visible after refresh failure and honors declined removal', async () => {
    mocks.list.mockResolvedValue([repo]);
    mocks.refresh.mockRejectedValue(new Error('offline'));
    mocks.confirm.mockResolvedValue(false);
    const user = userEvent.setup();
    render(<PluginRepositories session="test" catalog={false} onPreview={vi.fn()} />);
    await user.click(await screen.findByRole('button', { name: 'plugins.refreshRepo' }));
    expect(await screen.findByText('plugins.repositoryReadError')).toBeInTheDocument();
    expect(screen.getByText('folder')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'plugins.removeRepoNamed' }));
    await waitFor(() => expect(mocks.confirm).toHaveBeenCalled());
    expect(mocks.remove).not.toHaveBeenCalled();
  });
  it('keeps the cached catalog searchable when an automatic refresh fails', async () => {
    const stale = {...repo,updated:'2000-01-01T00:00:00Z'};
    mocks.list.mockResolvedValue([stale]);
    mocks.refresh.mockRejectedValueOnce(new Error('offline'));
    render(<PluginRepositories session="offline" catalog onPreview={vi.fn()} />);
    expect(await screen.findByText('first')).toBeVisible();
    expect(await screen.findByText('plugins.catalogOffline')).toBeVisible();
    expect(mocks.refresh).toHaveBeenCalledWith(stale.id,stale.revision);
  });

  it('discovers new entries after checking a stale store while the catalog is open', async () => {
    const stale = {...repo,updated:'2000-01-01T00:00:00Z'};
    const fresh = {...repo,updated:new Date().toISOString(),indices:[{...repo.indices[0],entries:[{name:'new arrival'}]}]};
    mocks.list.mockResolvedValueOnce([stale]).mockResolvedValue([fresh]);
    mocks.refresh.mockResolvedValueOnce(undefined);
    render(<PluginRepositories session="new entries" catalog onPreview={vi.fn()} />);
    expect(await screen.findByText('new arrival')).toBeVisible();
    expect(screen.queryByText('first')).toBeNull();
  });

  it('publishes a completed refresh after the store form closes', async () => {
    let finish!: () => void;
    mocks.refresh.mockImplementationOnce(() => new Promise<void>(resolve => {finish=resolve;}));
    const stale={...repo,updated:'2000-01-01T00:00:00Z'};
    mocks.list.mockResolvedValueOnce([stale]).mockResolvedValue([{...repo,origin:'updated-folder'}]);
    const user=userEvent.setup();
    render(<PluginRepositories session="modal-refresh" catalog={false} onPreview={vi.fn()} />);
    await waitFor(()=>expect(finish).toBeDefined());
    await user.click(screen.getByRole('button',{name:'plugins.addRepo'}));
    await act(async()=>finish());
    await user.keyboard('{Escape}');
    expect(await screen.findByText('updated-folder')).toBeVisible();
  });

  it('retries the listing after the store was refreshed successfully', async () => {
    const stale={...repo,updated:'2000-01-01T00:00:00Z'};
    mocks.list.mockResolvedValueOnce([stale]).mockRejectedValueOnce(new Error('unavailable')).mockResolvedValue([{...repo,origin:'retry-result'}]);
    mocks.refresh.mockClear().mockResolvedValueOnce(undefined);
    render(<PluginRepositories session="retry-list" catalog={false} onPreview={vi.fn()} />);
    expect(await screen.findByText('plugins.catalogOffline')).toBeVisible();
    await act(async()=>{window.dispatchEvent(new Event('focus'));});
    expect(await screen.findByText('retry-result')).toBeVisible();
    expect(mocks.refresh).toHaveBeenCalledTimes(1);
  });

  it('scopes the vault choice to the address in the form', async () => {
    mocks.list.mockResolvedValue([]);
    const user = userEvent.setup();
    render(<PluginRepositories session="test" catalog={false} onPreview={vi.fn()} />);
    mocks.addRemote.mockResolvedValue(undefined);
    const opener = await screen.findByRole('button',{name:'plugins.addRepo'});
    await waitFor(() => expect(opener).toBeEnabled());
    await user.click(opener);
    await user.selectOptions(screen.getByRole('combobox'), 'remote');
    const address = screen.getByRole('textbox', {name:'plugins.archiveURL'});
    await waitFor(() => expect(address).toBeEnabled());
    await user.type(address, 'https://first.example/store.zip');
    const credential = screen.getByLabelText('plugins.archiveCredential');
    await user.click(credential);
    await user.clear(address);
    await user.type(address, 'https://second.example/store.zip');
    expect(credential).not.toBeChecked();
    await user.click(credential);
    await user.click(screen.getByRole('button', {name: 'plugins.addRemoteRepo'}));
    expect(mocks.addRemote).toHaveBeenCalledWith('https://second.example/store.zip', true);
    await waitFor(() => expect(mocks.announce).toHaveBeenCalledWith('plugins.repoUpdated'));
    expect(address).not.toBeInTheDocument();
  });

  it('sends a Git clone URL to the Git reader', async () => {
    mocks.list.mockResolvedValue([]);
    mocks.addGit.mockResolvedValue(undefined);
    const user=userEvent.setup();
    render(<PluginRepositories session="git" catalog={false} onPreview={vi.fn()} />);
    const add=screen.getByRole('button',{name:'plugins.addRepo'});
    await waitFor(()=>expect(add).toBeEnabled());
    await user.click(add);
    await user.selectOptions(screen.getByRole('combobox'),'git');
    await user.type(screen.getByRole('textbox',{name:'plugins.gitURL'}),'https://git.example.com/team/store.git');
    await user.click(screen.getByRole('button',{name:'plugins.addGit'}));
    expect(mocks.addGit).toHaveBeenCalledWith('https://git.example.com/team/store.git',false);
  });

  it('does not announce a publication after the folder picker is cancelled', async () => {
    mocks.announce.mockClear();
    mocks.list.mockResolvedValue([]);
    mocks.add.mockResolvedValue(false);
    render(<PluginRepositories session="cancel" catalog={false} onPreview={vi.fn()} />);
    const add = screen.getByRole('button', { name: 'plugins.addRepo' });
    await waitFor(() => expect(add).toBeEnabled());
    await userEvent.click(add);
    await userEvent.click(screen.getByRole('button',{name:'plugins.folderSource'}));
    expect(mocks.add).toHaveBeenCalled();
    expect(mocks.announce).not.toHaveBeenCalled();
  });

  it('restores focus when the store form closes with Escape', async () => {
    mocks.list.mockResolvedValue([]);
    const user = userEvent.setup();
    render(<PluginRepositories session="escape" catalog={false} onPreview={vi.fn()} />);
    const add = screen.getByRole('button', {name:'plugins.addRepo'});
    await waitFor(() => expect(add).toBeEnabled());
    await user.click(add);
    await user.keyboard('{Escape}');
    await waitFor(() => expect(add).toHaveFocus());
  });

  it('waits for a repository update when reopening the catalog', async () => {
    let finish!: () => void;
    mocks.refresh.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve; }));
    mocks.list.mockReset().mockResolvedValueOnce([repo]).mockResolvedValue([]);
    const page = render(<PluginRepositories session="navigation" catalog={false} onPreview={vi.fn()} />);
    await userEvent.click(await screen.findByRole('button', { name: 'plugins.refreshRepo' }));
    page.unmount();
    render(<PluginRepositories session="navigation" catalog onPreview={vi.fn()} />);
    expect(mocks.list).toHaveBeenCalledTimes(1);
    await act(async () => finish());
    expect(await screen.findByText('plugins.catalogEmpty')).toBeVisible();
    expect(mocks.list).toHaveBeenCalledTimes(2);
  });

  it('navigates between repository rows and confirms removal for the focused row', async () => {
    mocks.list.mockResolvedValue([repo, { ...repo, id: 'two', origin: 'second folder' }]);
    mocks.confirm.mockResolvedValueOnce(true);
    mocks.remove.mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(<PluginRepositories session="keyboard" catalog={false} onPreview={vi.fn()} />);
    const first = await screen.findByRole('gridcell', { name: 'folder' });
    expect(screen.getByRole('toolbar', { name: 'plugins.repositories' })).toBeVisible();
    first.focus();
    await user.keyboard('{ArrowDown}{ArrowRight}{ArrowRight}{ArrowRight}{Enter}');
    expect(mocks.remove).toHaveBeenCalledWith('two', 'reviewed');
  });

  it.each([true, false])('restores focus after removing the last repository (reload succeeds: %s)', async success => {
    let removed!: () => void;
    let finishRead!: () => void;
    mocks.list.mockReset().mockResolvedValueOnce([repo]).mockImplementationOnce(() => new Promise((resolve, reject) => {
      finishRead = () => success ? resolve([]) : reject(new Error('offline'));
    }));
    mocks.remove.mockImplementationOnce(() => new Promise<void>(resolve => { removed = resolve; }));
    mocks.confirm.mockResolvedValueOnce(true);
    const user = userEvent.setup();
    render(<PluginRepositories session="last" catalog={false} onPreview={vi.fn()} />);
    (await screen.findByRole('gridcell', { name: 'folder' })).focus();
    await user.keyboard('{ArrowRight}{ArrowRight}{ArrowRight}{Enter}');
    await act(async () => removed());
    expect(screen.queryByRole('gridcell', { name: 'folder' })).toBeNull();
    await act(async () => finishRead());
    expect(screen.getByRole('button', { name: 'plugins.addRepo' })).toHaveFocus();
  });

  it('paginates the grid while searching all entries', async () => {
    const entries = Array.from({ length: 51 }, (_, index) => ({ name: 'sample-' + index }));
    mocks.list.mockResolvedValue([{ ...repo, indices: [{ ...repo.indices[0], entries }] }]);
    render(<PluginRepositories session="pages" catalog onPreview={vi.fn()} />);
    const next = await screen.findByRole('button', { name: 'plugins.nextPage' });
    expect(screen.queryByText('sample-50')).toBeNull();
    next.focus();
    await userEvent.keyboard('{Enter}');
    expect(next).toHaveFocus();
    expect(next).toHaveAttribute('aria-disabled', 'true');
    expect(mocks.announce).toHaveBeenCalledWith('plugins.pageOf');
    expect(screen.getByText('sample-50')).toBeVisible();
    await userEvent.type(screen.getByLabelText('plugins.searchCatalog'), 'sample-2');
    expect(screen.getByText('sample-2')).toBeVisible();
    expect(screen.queryByRole('button', { name: 'plugins.nextPage' })).toBeNull();
  });
});
