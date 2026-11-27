import { repositoryError } from './pluginRepositoryError';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { plugincatalog, plugins, wailsapi } from '@wailsjs/go/models';
import { AddGitRepository, AddIndexRepository, AddArchiveRepository, AddRemoteRepository, AddLocalRepository, ListRepositories, PreviewRepositoryEntry, RefreshRepository, RemoveRepository, DiscardPreview } from '@wailsjs/go/wailsapi/Plugins';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useConfirm } from '../hooks/useConfirm';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid, DATAGRID_ENTRY_SELECTOR } from '../components/ui/DataGrid';

import { Modal } from '../components/ui/Modal';
import { DialogActions } from '../components/ui/DialogActions';

import { pendingRepositoryWrites, trackRepositoryWrite } from './pluginRepositoryWrites';

import { useCatalogRefresh } from './useCatalogRefresh';

interface Props {
  session: string;
  parentBusy?: boolean;
  onFormOpenChange?: (open:boolean) => void;
  catalog: boolean;
  onPreview: (preview: wailsapi.PluginPreview, opener: HTMLElement | null) => void;
}

type CatalogVariant = plugincatalog.Entry & { format: string; store: string };

function catalogRows(repositories: plugins.Repository[]) {
  return repositories.flatMap(repo => {
    const groups = new Map<string, CatalogVariant[]>();
    for (const index of repo.indices) {
      for (const entry of index.entries) {
        // Only a resolved package path identifies the same source across indices.
        const id = JSON.stringify([repo.id, entry.name, entry.path ?? null, entry.path ? null : index.format]);
        const variant = { ...entry, format: index.format, store: index.name };
        const group = groups.get(id);
        if (group) group.push(variant);
        else groups.set(id, [variant]);
      }
    }
    return Array.from(groups, ([id, variants]) => ({ id, repo, name: variants[0].name, variants }));
  });
}

function variantValues(variants: CatalogVariant[], field: 'store' | 'format' | 'category' | 'authentication' | 'version' | 'author' | 'license') {
  return [...new Set(variants.map(variant => variant[field]).filter(Boolean))].join(', ');
}

function catalogStatus(diagnostic?: string) {
  if (diagnostic === 'policy') return 'plugins.catalogPolicy';
  if (diagnostic === 'configuration') return 'plugins.catalogConfig';
  return diagnostic ? 'plugins.catalogSource' : 'plugins.catalogAvailable';
}

export default function PluginRepositories({ session, catalog, parentBusy, onFormOpenChange, onPreview }: Props) {
  const { t, i18n } = useTranslation();
  const { announce } = useAnnouncer();
  const confirm = useConfirm();
  const [repositories, setRepositories] = useState<plugins.Repository[]>([]);
  const [adding, setAdding] = useState(false);
  const [choice, setChoice] = useState<{ row: ReturnType<typeof catalogRows>[number]; opener: HTMLElement | null } | null>(null);
  const formOpen = adding || !!choice;
  useEffect(() => {
    onFormOpenChange?.(formOpen);
    return () => onFormOpenChange?.(false);
  }, [formOpen,onFormOpenChange]);
  const [source, setSource] = useState<'add' | 'index' | 'zip' | 'remote' | 'git'>('add');
  const [url, setUrl] = useState('');
  const [authenticated, setAuthenticated] = useState(false);
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
  const listGeneration = useRef(0);
  const refresh = useCallback(async () => {
    const generation = ++listGeneration.current;
    const result = await ListRepositories();
    if (mounted.current && generation === listGeneration.current) setRepositories(result ?? []);
  }, []);
  const offline = useCatalogRefresh(session,repositories,busy || !!parentBusy || formOpen,refresh);
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
    return () => { active = false; mounted.current = false; listGeneration.current++; };
  }, [session]);
  const offlineAnnounced = useRef(false);
  useEffect(() => {
    if (offline && !offlineAnnounced.current) announce(t('plugins.catalogOffline'));
    offlineAnnounced.current = offline;
  }, [offline,announce,t]);
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

  const allEntries = useMemo(() => catalogRows(repositories), [repositories]);
  const needle = search.toLocaleLowerCase();
  const entries = useMemo(() => allEntries.filter(row => row.variants.some(variant =>
    [variant.name, variant.description, variant.store, variant.format, variant.author, variant.license, row.repo.origin]
      .some(text => text?.toLocaleLowerCase().includes(needle)))), [allEntries, needle]);
  const preview = async (repo: plugins.Repository, entry: CatalogVariant, opener: HTMLElement | null) => {
    if (busy || parentBusy || entry.diagnostic) return;
    setBusy(true);
    setError('');
    try {
      const result = await PreviewRepositoryEntry(repo.id, repo.revision, entry.format, entry.name);
      if (mounted.current) { setChoice(null); onPreview(result, opener); }
      else await DiscardPreview(result.token);
    } catch (cause) { if (mounted.current) setError(repositoryError(cause, 'plugins.catalogPreviewError')); }
    finally { if (mounted.current) setBusy(false); }
  };
  const openRow = (row: typeof entries[number], opener: HTMLElement | null) => {
    if (busy || parentBusy) return;
    if (row.variants.length === 1) void preview(row.repo, row.variants[0], opener);
    else { setError(''); setChoice({ row, opener }); }
  };
  return <>
    <Toolbar ariaLabel={t(catalog ? 'plugins.catalog' : 'plugins.repositories')} left={<h2>{t(catalog ? 'plugins.catalog' : 'plugins.repositories')}</h2>}
      right={catalog ? <Input label={t('plugins.searchCatalog')} value={search} onChange={event => setSearch(event.target.value)} /> :
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
    {offline && <p>{t('plugins.catalogOffline')}</p>}
    {choice && <Modal isOpen initialFocusSelector={DATAGRID_ENTRY_SELECTOR} title={t('plugins.chooseCatalogFormat', { name: choice.row.name })} allowClose={!busy} onClose={() => {
      setChoice(null);
      setError('');
      const opener = choice.opener;
      requestAnimationFrame(() => opener?.focus());
    }}>
      <DataGrid items={choice.row.variants} getItemId={variant => variant.format}
        label={t('plugins.chooseCatalogFormat', { name: choice.row.name })}
        onActivate={variant => void preview(choice.row.repo, variant, choice.opener)}
        columns={[
          { key: 'format', label: t('plugins.format'), activatesRow: variant => !busy && !parentBusy && !variant.diagnostic },
          { key: 'diagnostic', label: t('common.status'), format: (_, variant) => t(catalogStatus(variant.diagnostic)) },
          { key: 'store', label: t('plugins.store') },
          { key: 'version', label: t('plugins.version') },
          { key: 'authentication', label: t('plugins.authentication') },
        ]} />
      {error && <p>{t(error)}</p>}
    </Modal>}
    {!choice && !adding && error && <p>{t(error)}</p>}
    {catalog ? <>
      {entries.length > 0 ? <DataGrid key={needle} virtualizeRows items={entries} getItemId={item => item.id} autoFocusOnMount={false} label={t('plugins.catalog')} onActivate={row => openRow(row, document.activeElement instanceof HTMLElement ? document.activeElement : null)}
        columns={[
          { key: 'name', label: t('plugins.name'), activatesRow: row => !busy && !parentBusy && (row.variants.length > 1 || !row.variants[0].diagnostic) },
          { key: 'store', label: t('plugins.store'), format: (_, row) => variantValues(row.variants, 'store') },
          { key: 'format', label: t('plugins.format'), format: (_, row) => variantValues(row.variants, 'format') }, { key: 'repo', label: t('plugins.origin'), format: (_, item) => item.repo.origin },
          { key: 'category', label: t('plugins.category'), format: (_, row) => variantValues(row.variants, 'category') }, { key: 'authentication', label: t('plugins.authentication'), format: (_, row) => variantValues(row.variants, 'authentication') },
          { key: 'version', label: t('plugins.version'), format: (_, row) => variantValues(row.variants, 'version') }, { key: 'author', label: t('plugins.author'), format: (_, row) => variantValues(row.variants, 'author') }, { key: 'license', label: t('plugins.license'), format: (_, row) => variantValues(row.variants, 'license') },
          { key: 'diagnostic', label: t('common.status'), format: (_, item) => [...new Set(item.variants.map(variant => t(catalogStatus(variant.diagnostic))))].join(' · ') },
        ]} /> : !busy && !error && <p>{t('plugins.catalogEmpty')}</p>}

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
