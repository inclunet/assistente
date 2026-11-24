import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DiscardPreview, InstalledComponents, InstallReviewed, ListInstalled, PreviewLocal, RegisterMCP, RemoveMCP, SetSkillActive, SyncMCP } from '@wailsjs/go/wailsapi/Plugins';
import type { database, plugins, wailsapi } from '@wailsjs/go/models';
import { Button } from '../components/ui/Button';
import { DataGrid } from '../components/ui/DataGrid';
import { Modal } from '../components/ui/Modal';
import { DecisionDialog } from '../components/ui/DecisionDialog';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { Toolbar } from '../components/ui/Toolbar';
import { useConfirm } from '../hooks/useConfirm';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { logger } from '../utils/logger';
import { useAuthStore } from '../store/authStore';

function mcpConnectionLabel(component: plugins.Component) {
  return [component.transport, component.endpoint].filter(Boolean).join(' · ');
}

function componentRequirement(component: plugins.Component) {
  if (component.reason === 'shellCommands') return 'plugins.shellPending';
  if (component.reason === 'supportingFiles') return 'plugins.filesPending';
  if (component.reason === 'frontmatter') return 'plugins.fieldsPending';
  if (component.reason === 'configuration') return 'plugins.mcpConfigRequired';
  if (component.kind === 'mcp' && !component.reason) return undefined;
  return component.kind === 'skill' ? undefined : 'plugins.unavailable';
}

function installationError(cause: unknown) {
  const message = String(cause);
  if (message.includes('PLUGIN_MCP_SYNC_PENDING')) return 'plugins.syncPending';
  if (message.includes('PLUGIN_COMPONENT_EDITED')) return 'plugins.edited';
  return 'plugins.installError';
}

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
  const confirm = useConfirm();
  const [items, setItems] = useState<database.PluginInstallation[] | null>(null);
  const [preview, setPreview] = useState<wailsapi.PluginPreview | null>(null);
  const [details, setDetails] = useState<{ installation: database.PluginInstallation; components: plugins.Component[] } | null>(null);
  const [listFailed, setListFailed] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(true);
  useEffect(() => {
    if (error) announce(t(error), 'assertive');
  }, [error, announce, t]);
  const detailsOpener = useRef<HTMLElement | null>(null);
  useEffect(() => {
    if (listFailed) announce(t('plugins.listError'), 'assertive');
  }, [listFailed, announce, t]);
  const mounted = useRef(false);
  const openToken = useRef<string | null>(null);
  const chooseButton = useRef<HTMLButtonElement>(null);
  const restoreFocus = useRef(false);
  useEffect(() => {
    if (!details && !preview && !busy && detailsOpener.current) {
      const opener = detailsOpener.current;
      detailsOpener.current = null;
      if (opener.isConnected) opener.focus(); else chooseButton.current?.focus();
    }
    if (!preview && !busy && restoreFocus.current) {
      restoreFocus.current = false;
      chooseButton.current?.focus();
    }
  }, [preview, details, busy]);
  const refresh = useCallback(async () => {
    const pending = pendingInstalls.get(session);
    if (pending) {
      try {
        await pending;
      } catch (cause) {
        if (mounted.current) setError(installationError(cause));
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
  const openDetails = async (installation: database.PluginInstallation) => {
    if (busy) return;
    detailsOpener.current = document.activeElement instanceof HTMLElement ? document.activeElement.closest<HTMLElement>('[role="gridcell"]') : null;
    setBusy(true);
    setError('');
    try {
      const components = await InstalledComponents(installation.id, installation.digest);
      if (mounted.current) setDetails({ installation, components: components ?? [] });
    } catch { if (mounted.current) setError('plugins.listError'); }
    finally { if (mounted.current) setBusy(false); }
  };
  const refreshAfterSync = async () => {
    if (details) {
      const { installation } = details;
      const components = await InstalledComponents(installation.id, installation.digest);
      if (mounted.current) setDetails({ installation, components: components ?? [] });
    } else {
      await refresh();
    }
  };
  const showPendingSync = async () => {
    try {
      await refreshAfterSync();
      if (mounted.current) setError('plugins.syncPending');
    } catch { if (mounted.current) setError('plugins.syncPendingStale'); }
  };
  const changeComponent = async (component: plugins.Component, preserve = false) => {
    if (!details || busy || error === 'plugins.syncPendingStale') return;
    setBusy(true);
    setError('');
    const { installation } = details;
    try {
      if (component.kind === 'mcp') {
        if (component.active) {
          const accepted = await confirm({ title: t(preserve ? 'plugins.keepServer' : 'plugins.removeServer', { name: component.name }), message: t(preserve ? 'plugins.keepConfirm' : 'plugins.removeConfirm', { name: component.name }), confirmText: t(preserve ? 'plugins.keepServer' : 'plugins.removeServer', { name: component.name }), cancelText: t('common.cancel') });
          if (!accepted || !mounted.current) return;
          await RemoveMCP(installation.id, installation.digest, component.key, preserve);
        } else {
          await RegisterMCP(installation.id, installation.digest, component.key);
        }
      } else {
        await SetSkillActive(installation.id, installation.digest, component.key, !component.active);
      }
      if (!mounted.current) return;
      try {
        const components = await InstalledComponents(installation.id, installation.digest);
        if (mounted.current) {
          setDetails({ installation, components: components ?? [] });
          announce(t(component.kind === 'mcp' ? 'plugins.serverChanged' : component.active ? 'plugins.deactivated' : 'plugins.activated', { name: component.name }));
        }
      } catch {
        if (mounted.current) {
          setDetails(null);
          setError('plugins.componentReloadError');
        }
      }
    } catch (cause) {
      if (!mounted.current) return;
      if (String(cause).includes('PLUGIN_MCP_SYNC_PENDING')) {
        await showPendingSync();
      } else setError(String(cause).includes('PLUGIN_COMPONENT_EDITED') ? 'plugins.edited' : 'plugins.componentError');
    }
    finally { if (mounted.current) setBusy(false); }
  };
  const syncMCP = async () => {
    if (busy) return;
    setBusy(true);
    try {
      await SyncMCP();
      try { await refreshAfterSync(); } catch { if (mounted.current) setError('plugins.syncPendingStale'); return; }
      if (mounted.current) { setError(''); announce(t('plugins.synchronized')); }
    } catch { if (mounted.current && error !== 'plugins.syncPendingStale') setError('plugins.syncFailed'); }
    finally { if (mounted.current) setBusy(false); }
  };
  const syncAction = <Button disabled={busy} onClick={() => void syncMCP()}>{t('plugins.retrySync')}</Button>;
  const sameRevision = Boolean(preview && preview.previousDigest === preview.digest);
  const editedServers = preview?.editedMcpServers ?? [];
  const previewDescription = editedServers.length ? 'plugins.editedUpdate'
    : preview?.affectedMcpServers?.length ? 'plugins.disconnectUpdate'
    : sameRevision ? 'plugins.current'
    : preview?.previousDigest ? 'plugins.updateNotice' : 'plugins.inactive';
  const install = async () => {
    if (!preview || busy || sameRevision || editedServers.length > 0) return;
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
    } catch (cause) {
      if (!mounted.current) return;
      const failure = installationError(cause);
      setError(failure);
      if (failure === 'plugins.syncPending') await refresh();
    } finally {
      if (mounted.current) { restoreFocus.current = true; setBusy(false); setPreview(null); }
    }
  };
  return <section className="plugins-page" aria-label={t('menu.plugins')}>
    <Toolbar left={<h1 className="page-toolbar__title">{t('menu.plugins')}</h1>}
      right={<><Button ref={chooseButton} onClick={() => void inspect()} disabled={busy}>{t('plugins.choose')}</Button>{!details && !preview && syncAction}</>} />
    <h2>{t('plugins.installed')}</h2>
    {!preview && !details && error && <p>{t(error)}</p>}
    {listFailed && <p>{t('plugins.listError')}</p>}
    {items && items.length > 0 ? <DataGrid items={items} onActivate={row => void openDetails(row)} getItemId={row => row.id} label={t('plugins.installed')}
      columns={[{ key: 'name', label: t('plugins.name') }, { key: 'version', label: t('plugins.version') }, { key: 'origin', label: t('plugins.origin') }]} />
      : items && !busy && !error && !listFailed && <p>{t('plugins.empty')}</p>}
    {details && <Modal isOpen title={details.installation.name} onClose={() => setDetails(null)} allowClose={!busy} readingMode>
      {error && <p>{t(error)}</p>}
      {syncAction}
      <h2>{t('plugins.components')}</h2>
      {details.components.length === 0 ? <p>{t('plugins.noComponents')}</p> : <ul>{details.components.map(component => <li key={component.key}>
        <strong>{component.name}</strong> · {t(component.kind === 'skill' ? 'plugins.skill' : component.kind === 'mcp' ? 'plugins.mcp' : 'plugins.invalid')}
        {component.kind === 'mcp' && <p>{mcpConnectionLabel(component)}</p>}
        {component.kind === 'skill' && !component.reason ? <>
          {component.active && <code> /{component.slug}</code>}
          <Button disabled={busy || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component)}>{t(component.active ? 'plugins.deactivate' : 'plugins.activate', { name: component.name })}</Button>
        </> : component.kind === 'mcp' && !component.reason ? <>
          <Button disabled={busy || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component)}>{t(component.active ? 'plugins.removeServer' : 'plugins.registerServer', { name: component.name })}</Button>
          {component.active && <Button disabled={busy || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component, true)}>{t('plugins.keepServer', { name: component.name })}</Button>}
        </> : <p>{t(componentRequirement(component) ?? 'plugins.unavailable')}</p>}
      </li>)}</ul>}
    </Modal>}
    {preview && <DecisionDialog isOpen title={t('plugins.preview', { name: preview.name })}
      description={t(previewDescription)} allowClose={!busy} size="lg"
      actions={[
        { id: 'install', label: t(preview.previousDigest ? 'plugins.update' : 'plugins.install'), primary: true, polarity: 'affirmative', disabled: busy || sameRevision || editedServers.length > 0 },
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
      {(preview.components ?? []).length > 0 && <ul aria-label={t('plugins.components')}>{preview.components.map(component => <li key={component.key}>{component.name} · {t(component.kind === 'skill' ? 'plugins.skill' : component.kind === 'mcp' ? 'plugins.mcp' : 'plugins.invalid')}{component.kind === 'mcp' && <p>{mcpConnectionLabel(component)}</p>}{componentRequirement(component) && <p>{t(componentRequirement(component)!)}</p>}</li>)}</ul>}
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
      </> }, ...(preview.affectedMcpServers?.length ? [{
        id: 'affected-mcp', label: t('plugins.affectedServers'), content: <>
          <p>{t('plugins.affectedServersNotice')}</p>
          <ul>{preview.affectedMcpServers.map(name => <li key={name}>{name}</li>)}</ul>
        </>,
      }] : []), ...(editedServers.length ? [{
        id: 'edited-mcp', label: t('plugins.editedServers'), content: <>
          <p>{t('plugins.editedServersNotice')}</p>
          <ul>{editedServers.map(name => <li key={name}>{name}</li>)}</ul>
        </>,
      }] : [])]} />}
  </section>;
}
