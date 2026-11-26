import { repositoryError } from './pluginRepositoryError';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { plugins, wailsapi } from '@wailsjs/go/models';
import { AddGitRepository, AddIndexRepository, AddArchiveRepository, AddRemoteRepository, AddLocalRepository, ListRepositories, PreviewRepositoryEntry, RefreshRepository, RemoveRepository, DiscardPreview } from '@wailsjs/go/wailsapi/Plugins';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useConfirm } from '../hooks/useConfirm';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid } from '../components/ui/DataGrid';

import { Modal } from '../components/ui/Modal';
import { DialogActions } from '../components/ui/DialogActions';

import { pendingRepositoryWrites, trackRepositoryWrite } from './pluginRepositoryWrites';

const pageSize = 50;

interface Props {
  session: string;
  parentBusy?: boolean;
  onFormOpenChange?: (open:boolean) => void;
  catalog: boolean;
  onPreview: (preview: wailsapi.PluginPreview, opener: HTMLElement | null) => void;
}

export default function PluginRepositories({ session, catalog, parentBusy, onFormOpenChange, onPreview }: Props) {
  const { t, i18n } = useTranslation();
  const { announce } = useAnnouncer();
  const confirm = useConfirm();
  const [repositories, setRepositories] = useState<plugins.Repository[]>([]);
  const [adding, setAdding] = useState(false);
  useEffect(() => {
    onFormOpenChange?.(adding);
    return () => onFormOpenChange?.(false);
  }, [adding,onFormOpenChange]);
  const [source, setSource] = useState<'add' | 'index' | 'zip' | 'remote' | 'git'>('add');
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
  const cancelAdd = () => { setAdding(false); requestAnimationFrame(() => { if (document.activeElement === document.body) addButton.current?.focus(); }); };
  const refresh = async () => {
    const result = await ListRepositories();
    if (mounted.current) setRepositories(result ?? []);
  };
  useEffect(() => {
    mounted.current = true;
    let active = true;
    async function load() {
      let failure = '';
      try { await pendingRepositoryWrites.get(session); } catch (cause) { failure = repositoryError(cause, 'plugins.repositoryReadError'); }
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

  const change = async (action: 'git' | 'index' | 'zip' | 'remote' | 'add' | 'refresh' | 'remove', repo?: plugins.Repository) => {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      if (action === 'remove' && repo) {
        const accepted = await confirm({ title: t('plugins.removeRepo'), message: t('plugins.removeRepoConfirm', { origin: repo.origin }), confirmText: t('plugins.removeRepo'), cancelText: t('common.cancel') });
        if (!accepted || !mounted.current) return;
      }
      const pending = action === 'add' ? AddLocalRepository()
        : action === 'index' ? AddIndexRepository()
        : action === 'zip' ? AddArchiveRepository()
        : action === 'remote' || action === 'git' ? (action === 'git' ? AddGitRepository : AddRemoteRepository)(url.trim(), authenticated).then(() => true)
        : repo ? (action === 'remove' ? RemoveRepository : RefreshRepository)(repo.id, repo.revision).then(() => true)
        : undefined;
      if (!pending) return;
      if (!await trackRepositoryWrite(session,pending) || !mounted.current) return;
      if (['add','index','zip','remote','git'].includes(action)) { setAdding(false); focusAfterRemoval.current = true; }
      if (action === 'remote' || action === 'git') { setUrl(''); setAuthenticated(false); }
      if (action === 'remove' && repo) {
        focusAfterRemoval.current = repositories.length === 1;
        setRepositories(rows => rows.filter(row => row.id !== repo.id));
      }
      await refresh();
      if (mounted.current) announce(t('plugins.repoUpdated'));
    } catch (cause) { if (mounted.current) setError(repositoryError(cause, action === 'git' ? 'plugins.gitError' : action === 'remote' ? 'plugins.archiveDownloadError' : 'plugins.repositoryReadError')); }
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
        <Button ref={addButton} disabled={busy} onClick={() => { setSource('add'); setUrl(''); setAuthenticated(false); setError(''); setAdding(true); }}>{t('plugins.addRepo')}</Button>} />
    {adding && <Modal isOpen title={t('plugins.addRepo')} allowClose={!busy} onClose={cancelAdd}>
      <form onSubmit={event => { event.preventDefault(); void change(source); }}>
      <label htmlFor="plugin-store-source">{t('plugins.storeSource')}</label>
      <select id="plugin-store-source" disabled={busy} value={source}
        onChange={event => { setSource(event.target.value as typeof source); setAuthenticated(false); setError(''); }}>
        <option value="add">{t('plugins.folderSource')}</option>
        <option value="index">{t('plugins.indexSource')}</option>
        <option value="zip">{t('plugins.zipSource')}</option>
        <option value="git">{t('plugins.gitURL')}</option>
        <option value="remote">{t('plugins.archiveURL')}</option>
      </select>
      {(source === 'remote' || source === 'git') && <>
      <Input type="url" required label={t(source === 'git' ? 'plugins.gitURL' : 'plugins.archiveURL')} value={url} disabled={busy} onChange={event => { setUrl(event.target.value); setAuthenticated(false); }} />
      <label htmlFor="plugin-use-host-credential"><input id="plugin-use-host-credential" type="checkbox" checked={authenticated} disabled={busy} onChange={event => setAuthenticated(event.target.checked)} />{t('plugins.archiveCredential')}</label>
      </>}
      {error && <p>{t(error)}</p>}
      <DialogActions primary={<Button type="submit" disabled={busy || ((source === 'remote' || source === 'git') && !url.trim())}>{t(source === 'git' ? 'plugins.addGit' : source === 'remote' ? 'plugins.addRemoteRepo' : source === 'index' ? 'plugins.indexSource' : source === 'zip' ? 'plugins.zipSource' : 'plugins.folderSource')}</Button>}
        secondary={<Button type="button" disabled={busy} onClick={cancelAdd}>{t('common.cancel')}</Button>} />
    </form></Modal>}
    {!adding && error && <p>{t(error)}</p>}
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
