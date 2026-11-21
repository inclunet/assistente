import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DiscardPreview, InstallReviewed, ListInstalled, PreviewLocal } from '@wailsjs/go/wailsapi/Plugins';
import type { database, wailsapi } from '@wailsjs/go/models';
import { Button } from '../components/ui/Button';
import { DataGrid } from '../components/ui/DataGrid';
import { DecisionDialog } from '../components/ui/DecisionDialog';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { Toolbar } from '../components/ui/Toolbar';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useAuthStore } from '../store/authStore';

export default function PluginsPage() {
  const session = useAuthStore(state => state.user?.sessionId);
  return session ? <InstalledPlugins key={session} /> : null;
}

function InstalledPlugins() {
  const { t } = useTranslation();
  useGridPageLandmarks({ pageClass: 'plugins-page' });
  const { announce } = useAnnouncer();
  const [items, setItems] = useState<database.PluginInstallation[]>([]);
  const [preview, setPreview] = useState<wailsapi.PluginPreview | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(true);
  useEffect(() => {
    if (error) announce(t(error), 'assertive');
  }, [error, announce, t]);
  const mounted = useRef(false);
  const chooseButton = useRef<HTMLButtonElement>(null);
  const restoreFocus = useRef(false);
  useEffect(() => {
    if (!preview && !busy && restoreFocus.current) {
      restoreFocus.current = false;
      chooseButton.current?.focus();
    }
  }, [preview, busy]);
  const refresh = useCallback(async () => {
    const rows = await ListInstalled();
    if (mounted.current) setItems(rows ?? []);
  }, []);
  useEffect(() => {
    mounted.current = true;
    void refresh().catch(() => { if (mounted.current) setError('plugins.listError'); })
      .finally(() => { if (mounted.current) setBusy(false); });
    return () => { mounted.current = false; };
  }, [refresh]);
  const inspect = async () => {
    setBusy(true);
    setError('');
    try {
      const next = await PreviewLocal();
      if (mounted.current && next) setPreview(next);
      else restoreFocus.current = true;
    } catch {
      restoreFocus.current = true;
      if (mounted.current) setError('plugins.inspectError');
    } finally {
      if (mounted.current) setBusy(false);
    }
  };
  const discard = async () => {
    if (!preview || busy) return;
    setError('');
    setBusy(true);
    try {
      await DiscardPreview(preview.token);
    } catch {
      if (mounted.current) setError('plugins.discardError');
      return;
    } finally {
      if (mounted.current) setBusy(false);
    }
    if (mounted.current) {
      restoreFocus.current = true;
      setPreview(null);
    }
  };
  const sameRevision = Boolean(preview && preview.previousDigest === preview.digest);
  const install = async () => {
    if (!preview || busy || sameRevision) return;
    setBusy(true);
    try {
      await InstallReviewed(preview.token);
      if (!mounted.current) return;
      announce(t('plugins.installedNotice', { name: preview.name }));
      try { await refresh(); } catch { if (mounted.current) setError('plugins.listError'); }
    } catch {
      if (mounted.current) setError('plugins.installError');
    } finally {
      if (mounted.current) { restoreFocus.current = true; setBusy(false); setPreview(null); }
    }
  };
  return <section className="plugins-page" aria-label={t('menu.plugins')}>
    <Toolbar left={<h1 className="page-toolbar__title">{t('menu.plugins')}</h1>}
      right={<Button ref={chooseButton} onClick={() => void inspect()} disabled={busy}>{t('plugins.choose')}</Button>} />
    <h2>{t('plugins.installed')}</h2>
    {!preview && error && <p>{t(error)}</p>}
    {items.length > 0 ? <DataGrid items={items} getItemId={row => row.id} label={t('plugins.installed')}
      columns={[{ key: 'name', label: t('plugins.name') }, { key: 'version', label: t('plugins.version') }, { key: 'origin', label: t('plugins.origin') }]} />
      : !busy && !error && <p>{t('plugins.empty')}</p>}
    {preview && <DecisionDialog isOpen title={t('plugins.preview', { name: preview.name })}
      description={t('plugins.inactive')} allowClose={!busy} size="lg"
      actions={[
        { id: 'install', label: t(preview.previousDigest ? 'plugins.update' : 'plugins.install'), primary: true, polarity: 'affirmative', disabled: busy || sameRevision },
        { id: 'cancel', label: t('common.cancel'), disabled: busy },
      ]}
      onAction={id => { if (id === 'install') void install(); else void discard(); }}
      onCancel={() => void discard()}
      readingRegions={[{ id: 'package', label: t('plugins.preview', { name: preview.name }), content: <>
      {error && <p>{t(error)}</p>}
      <dl>
        <dt>{t('plugins.origin')}</dt><dd>{preview.origin}</dd>
        {preview.version && <><dt>{t('plugins.version')}</dt><dd>{preview.version}</dd></>}
        {preview.previousDigest && <><dt>{t('plugins.previous')}</dt><dd>{preview.previousDigest}</dd></>}
        <dt>SHA-256</dt><dd>{preview.digest}</dd>
      </dl>
      {preview.previousDigest && <p>{t(sameRevision ? 'plugins.current' : 'plugins.updateNotice')}</p>}
      {(preview.diagnostics ?? []).length > 0 && <p>{t('plugins.warnings', { count: preview.diagnostics.length })}</p>}
      <details><summary>{t('plugins.files', { count: preview.files.length })}</summary><ul>{preview.files.map(path => <li key={path}>{path}</li>)}</ul></details>
      </> }]} />}
  </section>;
}
