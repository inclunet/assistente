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
import { logger } from '../utils/logger';
import { useAuthStore } from '../store/authStore';

// Cleanup can outlive the session that owned the preview. The backend still
// checks the owner and the exact token for every discard.
function discardAbandonedPreview(token: string) {
  void DiscardPreview(token).catch(() => logger.warn('Unable to discard an abandoned plugin preview'));
}

// A confirmed install continues across navigation. A new page for the same
// session waits for publication before reading the inventory.
const pendingInstalls = new Map<string, Promise<database.PluginInstallation>>();

export default function PluginsPage() {
  const session = useAuthStore(state => state.user?.sessionId);
  return session ? <InstalledPlugins key={session} session={session} /> : null;
}

function InstalledPlugins({ session }: { session: string }) {
  const { t } = useTranslation();
  useGridPageLandmarks({ pageClass: 'plugins-page' });
  const { announce } = useAnnouncer();
  const [items, setItems] = useState<database.PluginInstallation[] | null>(null);
  const [preview, setPreview] = useState<wailsapi.PluginPreview | null>(null);
  const [listFailed, setListFailed] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(true);
  useEffect(() => {
    if (error) announce(t(error), 'assertive');
  }, [error, announce, t]);
  useEffect(() => {
    if (listFailed) announce(t('plugins.listError'), 'assertive');
  }, [listFailed, announce, t]);
  const mounted = useRef(false);
  const openToken = useRef<string | null>(null);
  const chooseButton = useRef<HTMLButtonElement>(null);
  const restoreFocus = useRef(false);
  useEffect(() => {
    if (!preview && !busy && restoreFocus.current) {
      restoreFocus.current = false;
      chooseButton.current?.focus();
    }
  }, [preview, busy]);
  const refresh = useCallback(async () => {
    const pending = pendingInstalls.get(session);
    if (pending) {
      try {
        await pending;
      } catch {
        if (mounted.current) setError('plugins.installError');
      }
    }
    if (!mounted.current) return;
    try {
      const rows = await ListInstalled();
      if (mounted.current) { setItems(rows ?? []); setListFailed(false); }
    } catch {
      if (mounted.current) setListFailed(true);
    }
  }, [session]);
  useEffect(() => {
    mounted.current = true;
    void refresh()
      .finally(() => { if (mounted.current) setBusy(false); });
    return () => {
      mounted.current = false;
      if (openToken.current) discardAbandonedPreview(openToken.current);
      openToken.current = null;
    };
  }, [refresh]);
  const inspect = async () => {
    setBusy(true);
    setError('');
    try {
      const next = await PreviewLocal();
      if (!mounted.current) {
        if (next) discardAbandonedPreview(next.token);
        return;
      }
      if (next) {
        openToken.current = next.token;
        setPreview(next);
      } else restoreFocus.current = true;
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
      openToken.current = null;
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
    setError('');
    setBusy(true);
    try {
      openToken.current = null;
      const pending = InstallReviewed(preview.token);
      pendingInstalls.set(session, pending);
      try {
        await pending;
      } finally {
        if (pendingInstalls.get(session) === pending) pendingInstalls.delete(session);
      }
      if (!mounted.current) return;
      announce(t('plugins.installedNotice', { name: preview.name }));
      await refresh();
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
    {listFailed && <p>{t('plugins.listError')}</p>}
    {items && items.length > 0 ? <DataGrid items={items} getItemId={row => row.id} label={t('plugins.installed')}
      columns={[{ key: 'name', label: t('plugins.name') }, { key: 'version', label: t('plugins.version') }, { key: 'origin', label: t('plugins.origin') }]} />
      : items && !busy && !error && !listFailed && <p>{t('plugins.empty')}</p>}
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
      {(preview.diagnostics ?? []).length > 0 && <details>
        <summary>{t('plugins.warnings', { count: preview.diagnostics.length })}</summary>
        <ul>{preview.diagnostics.map(warning => {
          const prefix = 'unknown manifest field: ';
          const message = warning.startsWith(prefix)
            ? t('plugins.unknownField', { field: warning.slice(prefix.length) })
            : warning === 'invalid extensions object' ? t('plugins.invalidExtensions') : t('plugins.unsupportedMetadata');
          return <li key={warning}>{message}</li>;
        })}</ul>
      </details>}
      <details><summary>{t('plugins.files', { count: preview.files.length })}</summary><ul>{preview.files.map(path => <li key={path}>{path}</li>)}</ul></details>
      </> }]} />}
  </section>;
}
