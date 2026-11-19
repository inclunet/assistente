import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import PluginRepositories from './PluginRepositories';

const mocks = vi.hoisted(() => ({ list: vi.fn(), preview: vi.fn(), refresh: vi.fn(), remove: vi.fn(), confirm: vi.fn(), announce: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Plugins', () => ({ ListRepositories: mocks.list, RefreshRepository: mocks.refresh, PreviewRepositoryEntry: mocks.preview, RemoveRepository: mocks.remove }));
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
    render(<PluginRepositories catalog onPreview={onPreview} />);
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
    render(<PluginRepositories catalog={false} onPreview={vi.fn()} />);
    await user.click(await screen.findByRole('button', { name: 'plugins.refreshRepo' }));
    expect(await screen.findByText('plugins.repositoryReadError')).toBeInTheDocument();
    expect(screen.getByText('folder')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'plugins.removeRepoNamed' }));
    await waitFor(() => expect(mocks.confirm).toHaveBeenCalled());
    expect(mocks.remove).not.toHaveBeenCalled();
  });
});
