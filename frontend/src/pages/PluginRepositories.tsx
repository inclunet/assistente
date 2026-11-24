import { repositoryError } from './pluginRepositoryError';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { plugins, wailsapi } from '@wailsjs/go/models';
import { AddRemoteRepository, AddLocalRepository, ListRepositories, PreviewRepositoryEntry, RefreshRepository, RemoveRepository, DiscardPreview } from '@wailsjs/go/wailsapi/Plugins';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useConfirm } from '../hooks/useConfirm';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid } from '../components/ui/DataGrid';

const pageSize = 50;
const pendingWrites = new Map<string, Promise<boolean>>();

interface Props {
  session: string;
  parentBusy?: boolean;
  catalog: boolean;
  onPreview: (preview: wailsapi.PluginPreview, opener: HTMLElement | null) => void;
}

export default function PluginRepositories({ session, catalog, parentBusy, onPreview }: Props) {
  const { t, i18n } = useTranslation();
  const { announce } = useAnnouncer();
  const confirm = useConfirm();
  const [repositories, setRepositories] = useState<plugins.Repository[]>([]);
  const [url, setUrl] = useState('');
  const [authenticated, setAuthenticated] = useState(false);
  const [page, setPage] = useState(0);
  const [search, setSearch] = useState('');
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState('');
  const mounted = useRef(false);
  const addButton = useRef<HTMLButtonElement>(null);
  const focusAfterRemoval = useRef(false);
  useEffect(() => {
    if (busy || !focusAfterRemoval.current) return;
    focusAfterRemoval.current = false;
    if (document.activeElement === document.body) addButton.current?.focus();
  }, [busy]);
  const refresh = async () => {
    const result = await ListRepositories();
    if (mounted.current) setRepositories(result ?? []);
  };
  useEffect(() => {
    mounted.current = true;
    let active = true;
    async function load() {
      let failure = '';
      try { await pendingWrites.get(session); } catch (cause) { failure = repositoryError(cause, 'plugins.repositoryReadError'); }
      if (!active) return;
      try {
        const rows = await ListRepositories();
        if (active) setRepositories(rows ?? []);
      } catch { failure = 'plugins.repositoryReadError'; }
      if (active) { setError(failure); setBusy(false); }
    }
    void load();
    return () => { active = false; mounted.current = false; };
  }, [session]);
  useEffect(() => { if (error) announce(t(error), 'assertive'); }, [error, t, announce]);

  const change = async (action: 'remote' | 'add' | 'refresh' | 'remove', repo?: plugins.Repository) => {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      if (action === 'remove' && repo) {
        const accepted = await confirm({ title: t('plugins.removeRepo'), message: t('plugins.removeRepoConfirm', { origin: repo.origin }), confirmText: t('plugins.removeRepo'), cancelText: t('common.cancel') });
        if (!accepted || !mounted.current) return;
      }
      const pending = action === 'add' ? AddLocalRepository()
        : action === 'remote' ? AddRemoteRepository(url.trim(), authenticated).then(() => true)
        : repo ? (action === 'remove' ? RemoveRepository : RefreshRepository)(repo.id, repo.revision).then(() => true)
        : undefined;
      if (!pending) return;
      pendingWrites.set(session, pending);
      try {
        if (!await pending || !mounted.current) return;
      } finally {
        if (pendingWrites.get(session) === pending) pendingWrites.delete(session);
      }
      if (action === 'remote') { setUrl(''); setAuthenticated(false); }
      if (action === 'remove' && repo) {
        focusAfterRemoval.current = repositories.length === 1;
        setRepositories(rows => rows.filter(row => row.id !== repo.id));
      }
      await refresh();
      if (mounted.current) announce(t('plugins.repoUpdated'));
    } catch (cause) { if (mounted.current) setError(repositoryError(cause, action === 'remote' ? 'plugins.archiveDownloadError' : 'plugins.repositoryReadError')); }
    finally { if (mounted.current) setBusy(false); }
  };

  const allEntries = useMemo(() => repositories.flatMap(repo => repo.indices.flatMap(index => index.entries.map(entry => ({
    ...entry, repo, format: index.format, store: index.name,
    id: JSON.stringify([repo.id, index.format, entry.name]),
  })))), [repositories]);
  const needle = search.toLocaleLowerCase();
  const entries = useMemo(() => allEntries.filter(entry => [entry.name, entry.description, entry.store, entry.author, entry.license, entry.repo.origin].some(text => text?.toLocaleLowerCase().includes(needle))), [allEntries, needle]);
  const lastPage = Math.max(0, Math.ceil(entries.length / pageSize) - 1);
  const currentPage = Math.min(page, lastPage);
  const selectPage = (next: number) => {
    if (next < 0 || next > lastPage) return;
    setPage(next);
    announce(t('plugins.pageOf', { page: next + 1, total: lastPage + 1 }));
  };
  const preview = async (entry: typeof entries[number]) => {
    if (busy || parentBusy || entry.diagnostic) return;
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setBusy(true);
    setError('');
    try {
      const result = await PreviewRepositoryEntry(entry.repo.id, entry.repo.revision, entry.format, entry.name);
      if (mounted.current) onPreview(result, opener);
      else await DiscardPreview(result.token);
    } catch (cause) { if (mounted.current) setError(repositoryError(cause, 'plugins.catalogPreviewError')); }
    finally { if (mounted.current) setBusy(false); }
  };
  return <>
    <Toolbar ariaLabel={t(catalog ? 'plugins.catalog' : 'plugins.repositories')} left={<h2>{t(catalog ? 'plugins.catalog' : 'plugins.repositories')}</h2>}
      right={catalog ? <Input label={t('plugins.searchCatalog')} value={search} onChange={event => { setSearch(event.target.value); setPage(0); }} /> :
        <Button ref={addButton} disabled={busy} onClick={() => void change('add')}>{t('plugins.addRepo')}</Button>} />
    {!catalog && <form onSubmit={event => { event.preventDefault(); void change('remote'); }}>
      <Input type="url" required label={t('plugins.archiveURL')} value={url} disabled={busy} onChange={event => { setUrl(event.target.value); setAuthenticated(false); }} />
      <label htmlFor="plugin-use-host-credential"><input id="plugin-use-host-credential" type="checkbox" checked={authenticated} disabled={busy} onChange={event => setAuthenticated(event.target.checked)} />{t('plugins.archiveCredential')}</label>
      <Button type="submit" disabled={busy || !url.trim()}>{t('plugins.addRemoteRepo')}</Button>
    </form>}
    {error && <p>{t(error)}</p>}
    {catalog ? <>
      {entries.length > 0 ? <DataGrid items={entries.slice(currentPage * pageSize, (currentPage + 1) * pageSize)} getItemId={item => item.id} autoFocusOnMount={false} label={t('plugins.catalog')} onActivate={entry => void preview(entry)}
        columns={[
          { key: 'name', label: t('plugins.name') }, { key: 'store', label: t('plugins.store') },
          { key: 'format', label: t('plugins.format') }, { key: 'repo', label: t('plugins.origin'), format: (_, item) => item.repo.origin },
          { key: 'category', label: t('plugins.category') }, { key: 'authentication', label: t('plugins.authentication') },
          { key: 'version', label: t('plugins.version') }, { key: 'author', label: t('plugins.author') }, { key: 'license', label: t('plugins.license') },
          { key: 'diagnostic', label: t('common.status'), format: (_, item) => t(item.diagnostic === 'policy' ? 'plugins.catalogPolicy' : item.diagnostic === 'configuration' ? 'plugins.catalogConfig' : item.diagnostic ? 'plugins.catalogSource' : 'plugins.catalogAvailable') },
        ]} /> : !busy && !error && <p>{t('plugins.catalogEmpty')}</p>}
      {lastPage > 0 && <nav aria-label={t('plugins.catalog')}>
        <Button aria-disabled={currentPage === 0} onClick={() => selectPage(currentPage - 1)}>{t('plugins.prevPage')}</Button>
        <span>{t('plugins.pageOf', { page: currentPage + 1, total: lastPage + 1 })}</span>
        <Button aria-disabled={currentPage === lastPage} onClick={() => selectPage(currentPage + 1)}>{t('plugins.nextPage')}</Button>
      </nav>}
    </> : repositories.length === 0 && (busy || error) ? null : <DataGrid
      items={repositories} getItemId={repo => repo.id} label={t('plugins.repositories')} autoFocusOnMount={false}
      onEmptyFocus={() => { addButton.current?.focus(); }}
      onCellAction={(repo, column) => {
        if (column.key === 'refresh' || column.key === 'remove') void change(column.key, repo);
      }}
      columns={[
        { key: 'origin', label: t('plugins.origin') },
        { key: 'updated', label: t('plugins.lastIndexLabel'), format: (_, repo) => new Date(repo.updated).toLocaleString(i18n.language) },
        ...(['refresh', 'remove'] as const).map(action => ({
          key: action, label: t(action === 'refresh' ? 'plugins.refreshLabel' : 'plugins.removeRepo'), keyboardAction: true,
          format: (_: unknown, repo: plugins.Repository) => <Button tabIndex={-1} disabled={busy} onClick={() => void change(action, repo)}>
            {t(action === 'refresh' ? 'plugins.refreshRepo' : 'plugins.removeRepoNamed', { origin: repo.origin })}
          </Button>,
        })),
      ]}
    />}
  </>;
}
