import { repositoryError } from './pluginRepositoryError';
import { useCallback, useEffect, useRef, useState } from 'react';
import { create } from 'zustand';
import { useTranslation } from 'react-i18next';
import { CreateListFromTemplate, ChangeInstallation, DiscardPreview, InstalledComponents, InstallReviewed, ListInstalled, PreviewLocal, RegisterMCP, RemoveMCP, SetSkillActive, SyncMCP } from '@wailsjs/go/wailsapi/Plugins';
import type { database, plugins, wailsapi } from '@wailsjs/go/models';
import { Button } from '../components/ui/Button';
import { DataGrid } from '../components/ui/DataGrid';
import { Modal } from '../components/ui/Modal';
import PluginRepositories from './PluginRepositories';
import PluginBuilder from './PluginBuilder';
import { Tabs, Tab, TabList, TabPanel } from '../components/ui/tabs/Tabs';
import { DecisionDialog } from '../components/ui/DecisionDialog';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { Toolbar } from '../components/ui/Toolbar';
import { useConfirm } from '../hooks/useConfirm';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { logger } from '../utils/logger';
import { useAuthStore } from '../store/authStore';

type InstallationRow = Pick<database.PluginInstallation, 'id' | 'origin' | 'name' | 'version' | 'digest' | 'disabled'>;

function mcpConnectionLabel(component: plugins.Component) {
  return [component.transport, component.endpoint].filter(Boolean).join(' · ');
}

function componentRequirement(component: plugins.Component) {
  if (component.reason === 'shellCommands') return 'plugins.shellPending';
  if (component.reason === 'supportingFiles') return 'plugins.filesPending';
  if (component.reason === 'missingTemplate') return 'plugins.missingTemplate';
  if (component.reason === 'invalidTemplate') return 'plugins.invalidTemplate';
  if (component.reason === 'manifestConfiguration') return 'plugins.manifestPending';
  if (component.reason === 'packagePaths') return 'plugins.packagePathsPending';
  if (component.reason === 'rootSkill') return 'plugins.rootSkillPending';
  if (component.reason === 'frontmatter') return 'plugins.fieldsPending';
  if (component.reason === 'configuration') return 'plugins.mcpConfigRequired';
  if ((component.kind === 'mcp' || component.kind === 'tasklistTemplate') && !component.reason) return undefined;
  return component.kind === 'skill' ? undefined : 'plugins.unavailable';
}

function installationError(cause: unknown, fallback = 'plugins.installError') {
  const message = String(cause);
  if (message.includes('PLUGIN_MCP_SYNC_PENDING')) return 'plugins.syncPending';
  if (message.includes('PLUGIN_COMPONENT_EDITED')) return 'plugins.edited';
  return repositoryError(cause, fallback);
}

// Cleanup can outlive the session that owned the preview. The backend still
// checks the owner and the exact token for every discard.
function discardAbandonedPreview(token: string) {
  void DiscardPreview(token).catch(() => logger.warn('Unable to discard an abandoned plugin preview'));
}

// Confirmed inventory writes outlive the page that submitted them.
const listRequests = new Map<string, string>();
useAuthStore.subscribe((state, previous) => {
  if (state.user?.sessionId !== previous.user?.sessionId) listRequests.clear();
});

const publications = new Map<string, Promise<void>>();
const usePendingLifecycleSync = create<ReadonlyMap<string, symbol>>(() => new Map());

async function reconcileMCP(session: string, operation: Promise<void>) {
  const pending = usePendingLifecycleSync.getState().get(session);
  await operation;
  if (pending && usePendingLifecycleSync.getState().get(session) === pending) {
    const next = new Map(usePendingLifecycleSync.getState());
    next.delete(session);
    usePendingLifecycleSync.setState(next, true);
  }
}

async function publish(session: string, operation: Promise<unknown>, lifecycle = false) {
  const outstandingSync = usePendingLifecycleSync.getState().get(session);
  const pending = operation.then(async () => {
    // The write may skip reconciliation when it does not affect a linked server.
    // Finish an earlier removal before treating its warning as resolved.
    if (outstandingSync && usePendingLifecycleSync.getState().get(session) === outstandingSync) {
      try {
        await reconcileMCP(session, SyncMCP(session));
      } catch {
        throw new Error('PLUGIN_MCP_SYNC_PENDING');
      }
    }
  }).catch(cause => {
    const message = installationError(cause, lifecycle ? 'plugins.componentError' : 'plugins.installError');
    if (lifecycle && message === 'plugins.syncPending') {
      usePendingLifecycleSync.setState(current => new Map(current).set(session, Symbol()), true);
    }
    throw message;
  });
  publications.set(session, pending);
  try {
    await pending;
  } finally {
    if (publications.get(session) === pending) publications.delete(session);
  }
}

export default function PluginsPage() {
  const session = useAuthStore(state => state.user?.sessionId);
  return session ? <InstalledPlugins key={session} session={session} /> : null;
}

function InstalledPlugins({ session }: { session: string }) {
  const { t } = useTranslation();
  const [section, setSection] = useState('installed');
  const builderForm = useRef<HTMLFormElement>(null);
  useGridPageLandmarks({ pageClass: 'plugins-page', contentLandmark: section === 'builder' ? {
    id: 'plugin-builder', label: t('plugins.builder'),
    isAvailable: () => !!builderForm.current?.querySelector('input:enabled'),
    contains: () => !!builderForm.current?.contains(document.activeElement),
    focus: () => {
      const input = builderForm.current?.querySelector('input:enabled');
      if (!(input instanceof HTMLInputElement)) return false;
      input.focus();
      return document.activeElement === input;
    },
  } : undefined });
  const { announce } = useAnnouncer();
  const confirm = useConfirm();
  const catalogOpener = useRef<HTMLElement | null>(null);
  const [items, setItems] = useState<InstallationRow[] | null>(null);
  const [preview, setPreview] = useState<wailsapi.PluginPreview | null>(null);
  const [details, setDetails] = useState<{ installation: InstallationRow; components: plugins.Component[] } | null>(null);
  const [listError, setListError] = useState('');
  const [operationError, setError] = useState('');
  const pendingSync = usePendingLifecycleSync(state => state.has(session));
  const error = operationError || (pendingSync ? 'plugins.syncPending' : '');
  const [busy, setBusy] = useState(true);
  useEffect(() => {
    if (error) announce(t(error), 'assertive');
  }, [error, announce, t]);
  const detailsOpener = useRef<HTMLElement | null>(null);
  const listErrorText = listError ? t(listError) : '';
  useEffect(() => {
    if (listErrorText) announce(listErrorText, 'assertive');
  }, [listErrorText, announce]);
  const mounted = useRef(false);
  const lifecycleOpener = useRef<HTMLButtonElement | null>(null);
  const openToken = useRef<string | null>(null);
  const chooseButton = useRef<HTMLButtonElement>(null);
  const restoreFocus = useRef(false);
  useEffect(() => {
    if (!busy && details && lifecycleOpener.current?.isConnected) lifecycleOpener.current.focus();
    if (!busy) lifecycleOpener.current = null;
    if (!details && !preview && !busy && detailsOpener.current) {
      const opener = detailsOpener.current;
      detailsOpener.current = null;
      if (opener.isConnected) opener.focus(); else chooseButton.current?.focus();
    }
    if (!preview && !busy && restoreFocus.current) {
      restoreFocus.current = false;
      if (catalogOpener.current?.isConnected) catalogOpener.current.focus(); else chooseButton.current?.focus();
      catalogOpener.current = null;
    }
  }, [preview, details, busy]);
  const refresh = useCallback(async (failureMessage = 'plugins.listError') => {
    const pending = publications.get(session);
    if (pending) {
      try {
        await pending;
      } catch (cause) {
        if (mounted.current && !(cause === 'plugins.syncPending' && usePendingLifecycleSync.getState().has(session))) setError(String(cause));
      }
    }
    if (!mounted.current) return false;
    try {
      const rows = await ListInstalled();
      if (mounted.current) { setItems(rows ?? []); setListError(''); }
      return true;
    } catch {
      if (mounted.current) setListError(failureMessage);
      return false;
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
  const openDetails = async (installation: InstallationRow) => {
    if (busy) return;
    setCreatedList('');
    detailsOpener.current = document.activeElement instanceof HTMLElement ? document.activeElement.closest<HTMLElement>('[role="gridcell"]') : null;
    setBusy(true);
    setError('');
    try {
      const components = await InstalledComponents(installation.id, installation.digest);
      if (mounted.current) setDetails({ installation, components: components ?? [] });
    } catch { if (mounted.current) setError('plugins.listError'); }
    finally { if (mounted.current) setBusy(false); }
  };
  const refreshDetails = async (installation: InstallationRow) => {
    const components = await InstalledComponents(installation.id, installation.digest);
    if (mounted.current) setDetails({ installation, components: components ?? [] });
  };
  const [createdList, setCreatedList] = useState('');
  const changeComponent = async (component: plugins.Component, preserve = false) => {
    if (!details || busy || error === 'plugins.syncPendingStale') return;
    setBusy(true);
    setError('');
    const { installation } = details;
    try {
      if (component.kind === 'tasklistTemplate') {
        const key = JSON.stringify([session, installation.id, installation.digest, component.key]);
        setCreatedList('');
        const request = listRequests.get(key) ?? crypto.randomUUID();
        listRequests.set(key, request);
        await CreateListFromTemplate(installation.id, installation.digest, component.key, request);
        if (mounted.current) {
          if (listRequests.get(key) === request) listRequests.delete(key);
          setCreatedList(component.name);
          announce(t('plugins.createdList', { name: component.name }));
        }
        return;
      }
      if (component.kind === 'mcp') {
        if (component.active) {
          const accepted = await confirm({ title: t(preserve ? 'plugins.keepServer' : 'plugins.removeServer', { name: component.name }), message: t(preserve ? 'plugins.keepConfirm' : 'plugins.removeConfirm', { name: component.name }), confirmText: t(preserve ? 'plugins.keepServer' : 'plugins.removeServer', { name: component.name }), cancelText: t('common.cancel') });
          if (!accepted || !mounted.current) return;
          await reconcileMCP(session, RemoveMCP(installation.id, installation.digest, component.key, preserve));
        } else {
          await reconcileMCP(session, RegisterMCP(installation.id, installation.digest, component.key));
        }
      } else {
        await SetSkillActive(installation.id, installation.digest, component.key, !component.active);
      }
      if (!mounted.current) return;
      try {
        await refreshDetails(installation);
        if (mounted.current) {
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
        try {
          await refreshDetails(installation);
          if (mounted.current) setError('plugins.syncPending');
        } catch { if (mounted.current) setError('plugins.syncPendingStale'); }
      } else if (String(cause).includes('PLUGIN_LIST_CAPACITY')) setError('plugins.listCapacity');
      else setError(installationError(cause, 'plugins.componentError'));
    }
    finally { if (mounted.current) setBusy(false); }
  };
  const syncMCP = async () => {
    if (busy) return;
    setBusy(true);
    try {
      await reconcileMCP(session, SyncMCP(session));
      if (!mounted.current) return;
      setError('');
      if (details) {
        try {
          await refreshDetails(details.installation);
        } catch {
          if (mounted.current) { setDetails(null); setError('plugins.syncReloadError'); }
          return;
        }
      } else if (!await refresh('plugins.syncReloadError')) {
        return;
      }
      if (mounted.current) announce(t('plugins.synchronized'));
    } catch {
      if (!mounted.current) return;
      if (!details && listError) {
        setError('');
        setListError('plugins.syncFailedStale');
      } else if (error !== 'plugins.syncPendingStale') {
        setError('plugins.syncFailed');
      }
    }
    finally { if (mounted.current) setBusy(false); }
  };
  const syncAction = <Button disabled={busy} onClick={() => void syncMCP()}>{t('plugins.retrySync')}</Button>;
  const changeInstallation = async (action: 'enable' | 'disable' | 'uninstall', opener: HTMLButtonElement) => {
    if (busy || !details || error === 'plugins.syncPendingStale') return;
    setBusy(true);
    setError('');
    const { installation } = details;
    lifecycleOpener.current = opener;
    try {
      const accepted = await confirm({
        title: t('plugins.' + action, { name: installation.name }),
        message: [t('plugins.' + action + 'Confirm'), t('plugins.origin') + ': ' + installation.origin].join('\n'),
        confirmText: t('plugins.' + action, { name: installation.name }),
        cancelText: t('common.cancel'),
      });
      if (!accepted || !mounted.current) return;
      let syncPending = false;
      try {
        await publish(session, ChangeInstallation(installation.id, installation.digest, action), true);
      } catch (cause) {
        if (cause !== 'plugins.syncPending') throw cause;
        syncPending = true;
      }
      if (!mounted.current) return;
      setItems(rows => rows === null ? null : action === 'uninstall'
        ? rows.filter(row => row.id !== installation.id)
        : rows.map(row => row.id === installation.id ? { ...row, disabled: action === 'disable' } : row));
      setDetails(null);
      if (!syncPending) announce(t('plugins.' + action + 'Done', { name: installation.name }));
      await refresh();
    } catch (cause) {
      if (!mounted.current) return;
      if (cause === 'plugins.edited') {
        try {
          await refreshDetails(installation);
          if (mounted.current) setError('plugins.editedLifecycle');
        } catch {
          if (mounted.current) { setDetails(null); setError('plugins.listError'); }
        }
      } else setError('plugins.componentError');
    } finally { if (mounted.current) setBusy(false); }
  };
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
      await publish(session, InstallReviewed(preview.token));
      if (!mounted.current) return;
      announce(t('plugins.installedNotice', { name: preview.name }));
      await refresh();
    } catch (cause) {
      if (!mounted.current) return;
      const failure = String(cause);
      setError(failure);
      if (failure === 'plugins.syncPending') await refresh();
    } finally {
      if (mounted.current) { restoreFocus.current = true; setBusy(false); setPreview(null); }
    }
  };
  return <section className="plugins-page" aria-label={t('menu.plugins')}>
    <Toolbar ariaLabel={t('menu.plugins')} left={<h1 className="page-toolbar__title">{t('menu.plugins')}</h1>}
      right={<><Button ref={chooseButton} onClick={() => void inspect()} disabled={busy}>{t('plugins.choose')}</Button>{!details && !preview && syncAction}</>} />
    <Tabs value={section} onValueChange={setSection}>
      <TabList ariaLabel={t('menu.plugins')}>{['installed','catalog','repositories','builder'].map(value => <Tab value={value} key={value}>{t('plugins.' + value)}</Tab>)}</TabList>
      <TabPanel value="installed">{section === 'installed' && <>
    <h2>{t('plugins.installed')}</h2>
    {items && items.length > 0 ? <DataGrid items={items} onActivate={row => void openDetails(row)} getItemId={row => row.id} label={t('plugins.installed')}
      columns={[{ key: 'name', label: t('plugins.name') }, { key: 'version', label: t('plugins.version') }, { key: 'origin', label: t('plugins.origin') }]} />
      : items && !busy && !error && !listError && <p>{t('plugins.empty')}</p>}
      </>}</TabPanel>
      {['catalog','repositories'].map(value => <TabPanel key={value} value={value}>{section === value && <PluginRepositories key={value} session={session} parentBusy={busy} catalog={value === 'catalog'} onPreview={(selection, opener) => { catalogOpener.current = opener; openToken.current = selection.token; setError(''); setPreview(selection); }} />}</TabPanel>)}
      <TabPanel value="builder">{section === 'builder' && <PluginBuilder formRef={builderForm} />}</TabPanel>
    </Tabs>
    {!preview && !details && error && <p>{t(error)}</p>}
    {listErrorText && <p>{listErrorText}</p>}
    {details && <Modal isOpen title={details.installation.name} onClose={() => setDetails(null)} allowClose={!busy} readingMode>
      {error && <p>{t(error)}</p>}
      {syncAction}
      <p>{t('plugins.origin')}: {details.installation.origin}</p>
      {details.installation.disabled && <p>{t('plugins.disabledNotice')}</p>}
      <Button disabled={busy || error === 'plugins.syncPendingStale'} onClick={event => void changeInstallation(details.installation.disabled ? 'enable' : 'disable', event.currentTarget)}>
        {t(details.installation.disabled ? 'plugins.enable' : 'plugins.disable', { name: details.installation.name })}
      </Button>
      <Button disabled={busy || error === 'plugins.syncPendingStale'} onClick={event => void changeInstallation('uninstall', event.currentTarget)}>{t('plugins.uninstall', { name: details.installation.name })}</Button>
      {createdList && <p>{t('plugins.createdList', { name: createdList })}</p>}
      <h2>{t('plugins.components')}</h2>
      {details.components.length === 0 ? <p>{t('plugins.noComponents')}</p> : <ul>{details.components.map(component => <li key={component.key}>
        <strong>{component.name}</strong> · {t(component.kind === 'skill' ? 'plugins.skill' : component.kind === 'mcp' ? 'plugins.mcp' : component.kind === 'tasklistTemplate' ? 'plugins.tasklistTemplate' : 'plugins.invalid')}
        {component.editedServer && <p>{t('plugins.editedServer', { name: component.editedServer })}</p>}
        {component.kind === 'mcp' && <p>{mcpConnectionLabel(component)}</p>}
        {component.kind === 'skill' && component.requestsShell && <p>{t('plugins.commandPolicy')}</p>}
        {component.kind === 'tasklistTemplate' && !component.reason ?
          <Button disabled={busy || details.installation.disabled || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component)}>{t('plugins.createList', { name: component.name })}</Button>
          : component.kind === 'skill' && !component.reason ? <>
          {component.active && <code> /{component.slug}</code>}
          <Button disabled={busy || details.installation.disabled || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component)}>{t(component.active ? 'plugins.deactivate' : 'plugins.activate', { name: component.name })}</Button>
        </> : component.kind === 'mcp' && !component.reason ? <>
          <Button disabled={busy || details.installation.disabled || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component)}>{t(component.active ? 'plugins.removeServer' : 'plugins.registerServer', { name: component.name })}</Button>
          {component.active && <Button disabled={busy || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component, true)}>{t('plugins.keepServer', { name: component.name })}</Button>}
        </> : <p>{t(componentRequirement(component) ?? 'plugins.unavailable', { path: component.entry })}</p>}
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
        {preview.authentication && <><dt>{t('plugins.authentication')}</dt><dd>{preview.authentication}</dd></>}
        <dt>SHA-256</dt><dd>{preview.digest}</dd>
      </dl>
      {(preview.components ?? []).length > 0 && <ul aria-label={t('plugins.components')}>{preview.components.map(component => <li key={component.key}>{component.name} · {t(component.kind === 'skill' ? 'plugins.skill' : component.kind === 'mcp' ? 'plugins.mcp' : component.kind === 'tasklistTemplate' ? 'plugins.tasklistTemplate' : 'plugins.invalid')}{component.requestsShell && <span> · {t('plugins.commandPolicy')}</span>}{component.kind === 'mcp' && <p>{mcpConnectionLabel(component)}</p>}{componentRequirement(component) && <p>{t(componentRequirement(component)!, { path: component.entry })}</p>}</li>)}</ul>}
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
