import { describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import PluginRepositories from './PluginRepositories';

const mocks = vi.hoisted(() => ({ add: vi.fn(), list: vi.fn(), preview: vi.fn(), refresh: vi.fn(), remove: vi.fn(), confirm: vi.fn(), announce: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Plugins', () => ({ AddLocalRepository: mocks.add, ListRepositories: mocks.list, RefreshRepository: mocks.refresh, PreviewRepositoryEntry: mocks.preview, RemoveRepository: mocks.remove }));
vi.mock('../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: mocks.announce }) }));
vi.mock('../hooks/useConfirm', () => ({ useConfirm: () => mocks.confirm }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));

const repo = { id: 'one', origin: 'folder', revision: 'reviewed', updated: '2026-11-19T00:00:00Z', indices: [{ format: 'cursor', name: 'sample-store', entries: [
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

  it('does not announce a publication after the folder picker is cancelled', async () => {
    mocks.announce.mockClear();
    mocks.list.mockResolvedValue([]);
    mocks.add.mockResolvedValue(false);
    render(<PluginRepositories session="cancel" catalog={false} onPreview={vi.fn()} />);
    const add = screen.getByRole('button', { name: 'plugins.addRepo' });
    await waitFor(() => expect(add).toBeEnabled());
    await userEvent.click(add);
    expect(mocks.add).toHaveBeenCalled();
    expect(mocks.announce).not.toHaveBeenCalled();
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
