import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { plugins, wailsapi } from '@wailsjs/go/models';
import { AddRemoteRepository, AddLocalRepository, ListRepositories, PreviewRepositoryEntry, RefreshRepository, RemoveRepository, DiscardPreview } from '@wailsjs/go/wailsapi/Plugins';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useConfirm } from '../hooks/useConfirm';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid } from '../components/ui/DataGrid';

interface Props {
  catalog: boolean;
  onPreview: (preview: wailsapi.PluginPreview, opener: HTMLElement | null) => void;
}

export default function PluginRepositories({ catalog, onPreview }: Props) {
  const { t, i18n } = useTranslation();
  const { announce } = useAnnouncer();
  const confirm = useConfirm();
  const [repositories, setRepositories] = useState<plugins.Repository[]>([]);
  const [url, setUrl] = useState('');
  const [authenticated, setAuthenticated] = useState(false);
  const [search, setSearch] = useState('');
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState('');
  const mounted = useRef(false);
  const refresh = async () => {
    const result = await ListRepositories();
    if (mounted.current) setRepositories(result ?? []);
  };
  useEffect(() => {
    mounted.current = true;
    void refresh().catch(() => { if (mounted.current) setError('plugins.repositoryReadError'); })
      .finally(() => { if (mounted.current) setBusy(false); });
    return () => { mounted.current = false; };
  }, []);
  useEffect(() => { if (error) announce(t(error), 'assertive'); }, [error, t, announce]);

  const change = async (action: 'remote' | 'add' | 'refresh' | 'remove', repo?: plugins.Repository) => {
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      if (action === 'add') await AddLocalRepository();
      else if (action === 'remote') {
        await AddRemoteRepository(url.trim(), authenticated);
        if (mounted.current) { setUrl(''); setAuthenticated(false); }
      } else if (repo && action === 'refresh') await RefreshRepository(repo.id, repo.revision);
      else if (repo && action === 'remove') {
        const accepted = await confirm({ title: t('plugins.removeRepo'), message: t('plugins.removeRepoConfirm', { origin: repo.origin }), confirmText: t('plugins.removeRepo'), cancelText: t('common.cancel') });
        if (!accepted || !mounted.current) return;
        await RemoveRepository(repo.id, repo.revision);
      }
      await refresh();
      if (mounted.current) announce(t('plugins.repoUpdated'));
    } catch { if (mounted.current) setError('plugins.repositoryReadError'); }
    finally { if (mounted.current) setBusy(false); }
  };

  const entries = repositories.flatMap(repo => repo.indices.flatMap(index => index.entries.map(entry => ({
    ...entry, repo, format: index.format, store: index.name,
    id: JSON.stringify([repo.id, index.format, entry.name]),
  })))).filter(entry => [entry.name, entry.description, entry.store, entry.author, entry.license, entry.repo.origin].some(text => text?.toLocaleLowerCase().includes(search.toLocaleLowerCase())));
  const preview = async (entry: typeof entries[number]) => {
    if (busy || entry.diagnostic) return;
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setBusy(true);
    setError('');
    try {
      const result = await PreviewRepositoryEntry(entry.repo.id, entry.repo.revision, entry.format, entry.name);
      if (mounted.current) onPreview(result, opener);
      else await DiscardPreview(result.token);
    } catch (cause) { if (mounted.current) setError(String(cause).includes('PLUGIN_AUTH_ON_INSTALL') ? 'plugins.catalogAuthError' : 'plugins.catalogPreviewError'); }
    finally { if (mounted.current) setBusy(false); }
  };
  return <>
    <Toolbar left={<h2>{t(catalog ? 'plugins.catalog' : 'plugins.repositories')}</h2>}
      right={catalog ? <Input label={t('plugins.searchCatalog')} value={search} onChange={event => setSearch(event.target.value)} /> :
        <Button disabled={busy} onClick={() => void change('add')}>{t('plugins.addRepo')}</Button>} />
    {!catalog && <form onSubmit={event => { event.preventDefault(); void change('remote'); }}>
      <Input type="url" required label={t('plugins.archiveURL')} value={url} disabled={busy} onChange={event => { setUrl(event.target.value); setAuthenticated(false); }} />
      <label htmlFor="plugin-use-host-credential"><input id="plugin-use-host-credential" type="checkbox" checked={authenticated} disabled={busy} onChange={event => setAuthenticated(event.target.checked)} />{t('plugins.archiveCredential')}</label>
      <Button type="submit" disabled={busy || !url.trim()}>{t('plugins.addRemoteRepo')}</Button>
    </form>}
    {error && <p>{t(error)}</p>}
    {catalog ? <>
      {entries.length > 0 ? <DataGrid items={entries} getItemId={item => item.id} autoFocusOnMount={false} label={t('plugins.catalog')} onActivate={entry => void preview(entry)}
        columns={[
          { key: 'name', label: t('plugins.name') }, { key: 'store', label: t('plugins.store') },
          { key: 'format', label: t('plugins.format') }, { key: 'repo', label: t('plugins.origin'), format: (_, item) => item.repo.origin },
          { key: 'category', label: t('plugins.category') }, { key: 'authentication', label: t('plugins.authentication') },
          { key: 'version', label: t('plugins.version') }, { key: 'author', label: t('plugins.author') }, { key: 'license', label: t('plugins.license') },
          { key: 'diagnostic', label: t('common.status'), format: (_, item) => t(item.diagnostic === 'policy' ? 'plugins.catalogPolicy' : item.diagnostic === 'configuration' ? 'plugins.catalogConfig' : item.diagnostic ? 'plugins.catalogSource' : 'plugins.catalogAvailable') },
        ]} /> : !busy && <p>{t('plugins.catalogEmpty')}</p>}
    </> : <ul>{repositories.map(repo => <li key={repo.id}>
      <strong>{repo.origin}</strong>
      <p>{t('plugins.lastIndex', { date: new Date(repo.updated).toLocaleString(i18n.language) })}</p>
      <Button disabled={busy} onClick={() => void change('refresh', repo)}>{t('plugins.refreshRepo', { origin: repo.origin })}</Button>
      <Button disabled={busy} onClick={() => void change('remove', repo)}>{t('plugins.removeRepoNamed', { origin: repo.origin })}</Button>
    </li>)}</ul>}
  </>;
}
