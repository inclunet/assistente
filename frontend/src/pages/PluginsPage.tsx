import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import { useNavigationStore } from '../store/navigationStore';
import { useTranslation } from 'react-i18next';
import { CreateListFromTemplate, ChangeInstallation, DiscardPreview, InstalledComponents, InstallReviewed, ListInstalled, PreviewLocal, RegisterMCP, RemoveMCP, PrepareJob, SetProfileActive, SetSkillActive, SyncMCP, SyncPendingJobs } from '@wailsjs/go/wailsapi/Plugins';
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
import { useAuthStore } from '../store/authStore';

function componentLabel(kind: string) {
  switch (kind) {
    case 'skill': return 'plugins.skill';
    case 'mcp': return 'plugins.mcp';
    case 'tasklistTemplate': return 'plugins.tasklistTemplate';
    case 'job': return 'plugins.job';
    case 'profile': return 'plugins.profile';
    default: return 'plugins.invalid';
  }
}

function componentRequirement(component: plugins.Component) {
  if (component.reason === 'invalidJob') return 'plugins.invalidJob';
  if (component.reason === 'invalidProfile') return 'plugins.invalidProfile';
  if (component.reason === 'shellCommands') return 'plugins.shellPending';
  if (component.reason === 'supportingFiles') return 'plugins.filesPending';
  if (component.reason === 'manifestConfiguration') return 'plugins.manifestPending';
  if (component.reason === 'frontmatter') return 'plugins.fieldsPending';
  if (component.reason === 'configuration') return 'plugins.mcpConfigRequired';
  if ((component.kind === 'mcp' || component.kind === 'profile' || component.kind === 'job' || component.kind === 'tasklistTemplate') && !component.reason) return undefined;
  return component.kind === 'skill' ? undefined : 'plugins.unavailable';
}

function JobRequirements({component}: {component: plugins.Component}) {
  const {t} = useTranslation();
  if (!component.tool) return null;
  return <p>
    {t('plugins.jobTool', {name: component.tool})}
      {Boolean(component.jobRequiredInputs?.length) && <span> · {t('plugins.jobInputs', {fields:component.jobRequiredInputs!.join(', ')})}</span>}
    {component.jobTriggers?.map((trigger, index) => <span key={index}> · {t(`plugins.jobTrigger.${trigger}`)}</span>)}
  </p>;
}

export default function PluginsPage() {
  const session = useAuthStore(state => state.user?.sessionId);
  return session ? <InstalledPlugins key={session} /> : null;
}

function InstalledPlugins() {
  const { t } = useTranslation();
  useGridPageLandmarks({ pageClass: 'plugins-page' });
  const { announce } = useAnnouncer();
  const confirm = useConfirm();
  const [section, setSection] = useState('installed');
  const catalogOpener = useRef<HTMLElement | null>(null);
  const [items, setItems] = useState<database.PluginInstallation[]>([]);
  const [preview, setPreview] = useState<wailsapi.PluginPreview | null>(null);
  const [details, setDetails] = useState<{ installation: database.PluginInstallation; components: plugins.Component[] } | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(true);
  const pendingSync = useRef({jobs:false, mcp:false});
  useEffect(() => {
    if (error) announce(t(error), 'assertive');
  }, [error, announce, t]);
  const detailsOpener = useRef<HTMLElement | null>(null);
  const mounted = useRef(false);
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
      if (catalogOpener.current?.isConnected) catalogOpener.current.focus(); else chooseButton.current?.focus();
      catalogOpener.current = null;
    }
  }, [preview, details, busy]);
  const refresh = useCallback(async () => {
    const rows = await ListInstalled();
    if (mounted.current) setItems(rows ?? []);
  }, []);
  useEffect(() => {
    mounted.current = true;
    void SyncPendingJobs().catch(() => {
      pendingSync.current.jobs = true;
      if (mounted.current) setError('plugins.syncPending');
    });
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
  const openDetails = async (installation: database.PluginInstallation) => {
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
  useResourceEditRequest('plugins', {
    ready: !busy,
    onEdit: id => {
      const installation = items.find(item => item.id === id);
      if (installation) void openDetails(installation);
      else setError('plugins.originUnavailable');
    },
  });
  const refreshAfterSync = async () => {
    if (details) {
      const { installation } = details;
      const components = await InstalledComponents(installation.id, installation.digest);
      if (mounted.current) setDetails({ installation, components: components ?? [] });
    } else {
      await refresh();
    }
  };
  const recordPendingSync = (cause: unknown) => {
    const message = String(cause);
    const jobs = message.includes('PLUGIN_JOB_SYNC_PENDING');
    const mcp = message.includes('PLUGIN_MCP_SYNC_PENDING');
    pendingSync.current.jobs ||= jobs;
    pendingSync.current.mcp ||= mcp;
    return jobs || mcp;
  };
  const showPendingSync = async () => {
    try {
      await refreshAfterSync();
      if (mounted.current) setError('plugins.syncPending');
    } catch { if (mounted.current) setError('plugins.syncPendingStale'); }
  };
  const listRequests = useRef(new Map<string,string>());
  const [createdList, setCreatedList] = useState('');
  const changeComponent = async (component: plugins.Component, preserve = false) => {
    if (!details || busy) return;
    setBusy(true);
    setError('');
    const { installation } = details;
    try {
      if (component.kind === 'tasklistTemplate') {
        const key = JSON.stringify([installation.id, installation.digest, component.key]);
        const request = listRequests.current.get(key) ?? crypto.randomUUID();
        listRequests.current.set(key, request);
        await CreateListFromTemplate(installation.id, installation.digest, component.key, request);
        listRequests.current.delete(key);
        if (mounted.current) {
          setCreatedList(component.name);
          announce(t('plugins.createdList', { name: component.name }));
        }
        return;
      }
      if (component.kind === 'mcp') {
        if (component.active) {
          const accepted = await confirm({ title: t(preserve ? 'plugins.keepServer' : 'plugins.removeServer', { name: component.name }), message: t(preserve ? 'plugins.keepConfirm' : 'plugins.removeConfirm', { name: component.name }), confirmText: t(preserve ? 'plugins.keepServer' : 'plugins.removeServer', { name: component.name }), cancelText: t('common.cancel') });
          if (!accepted || !mounted.current) return;
          await RemoveMCP(installation.id, installation.digest, component.key, preserve);
        } else {
          await RegisterMCP(installation.id, installation.digest, component.key);
        }
      } else if (component.kind === 'job') {
        await PrepareJob(installation.id, installation.digest, component.key);
      } else if (component.kind === 'profile') {
        await SetProfileActive(installation.id, installation.digest, component.key, !component.active);
      } else {
        await SetSkillActive(installation.id, installation.digest, component.key, !component.active);
      }
      const components = await InstalledComponents(installation.id, installation.digest);
      if (mounted.current) {
        setDetails({ installation, components: components ?? [] });
        announce(t(component.kind === 'job' ? 'plugins.jobPrepared' : component.kind === 'mcp' ? 'plugins.serverChanged' : component.active ? 'plugins.deactivated' : 'plugins.activated', { name: component.name }));
      }
    } catch (cause) {
      if (recordPendingSync(cause)) {
        await showPendingSync();
      } else if (mounted.current) setError(String(cause).includes('PLUGIN_COMPONENT_EDITED') ? 'plugins.edited' : 'plugins.componentError');
    }
    finally { if (mounted.current) setBusy(false); }
  };
  const syncMCP = async () => {
    if (busy) return;
    setBusy(true);
    try {
      const attempts: Promise<void>[] = [];
      if (pendingSync.current.jobs) attempts.push(Promise.resolve().then(SyncPendingJobs).then(() => { pendingSync.current.jobs = false; }));
      if (pendingSync.current.mcp) attempts.push(Promise.resolve().then(SyncMCP).then(() => { pendingSync.current.mcp = false; }));
      const results = await Promise.allSettled(attempts);
      if (results.some(result => result.status === 'rejected')) {
        if (mounted.current) setError('plugins.syncPending');
        return;
      }
      try { await refreshAfterSync(); } catch { if (mounted.current) setError('plugins.syncPendingStale'); return; }
      if (mounted.current) { setError(''); announce(t('plugins.synchronized')); }
    } catch { if (mounted.current) setError('plugins.syncPending'); }
    finally { if (mounted.current) setBusy(false); }
  };
  const retrySync = (error === 'plugins.syncPending' || error === 'plugins.syncPendingStale') && <Button disabled={busy} onClick={() => void syncMCP()}>{t('plugins.retrySync')}</Button>;
  const changeInstallation = async (action: 'enable' | 'disable' | 'uninstall') => {
    if (busy || !details) return;
    setBusy(true);
    setError('');
    const { installation } = details;
    try {
      const accepted = await confirm({
        title: t('plugins.' + action, { name: installation.name }),
        message: t('plugins.' + action + 'Confirm', { name: installation.name }),
        confirmText: t('plugins.' + action, { name: installation.name }),
        cancelText: t('common.cancel'),
      });
      if (!accepted || !mounted.current) return;
      await ChangeInstallation(installation.id, installation.digest, action);
      if (!mounted.current) return;
      setDetails(null);
      restoreFocus.current = true;
      announce(t('plugins.' + action + 'Done', { name: installation.name }));
      try { await refresh(); } catch { if (mounted.current) setError('plugins.listError'); }
    } catch (cause) {
      if (!mounted.current) return;
      if (recordPendingSync(cause)) {
        setDetails(null);
        restoreFocus.current = true;
        try { await refresh(); setError('plugins.syncPending'); }
        catch { if (mounted.current) setError('plugins.syncPendingStale'); }
      } else setError(String(cause).includes('PLUGIN_COMPONENT_EDITED') ? 'plugins.edited' : 'plugins.componentError');
    } finally { if (mounted.current) setBusy(false); }
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
    } catch (cause) {
      if (recordPendingSync(cause)) {
        await showPendingSync();
      } else if (mounted.current) setError(String(cause).includes('PLUGIN_COMPONENT_EDITED') ? 'plugins.edited' : 'plugins.installError');
    } finally {
      if (mounted.current) { restoreFocus.current = true; setBusy(false); setPreview(null); }
    }
  };
  return <section className="plugins-page" aria-label={t('menu.plugins')}>
    <Tabs value={section} onValueChange={setSection}>
      <TabList ariaLabel={t('menu.plugins')}>{['installed','catalog','repositories','builder'].map(value => <Tab value={value} key={value}>{t('plugins.' + value)}</Tab>)}</TabList>
      <TabPanel value="installed">{section === 'installed' && <>
    <Toolbar left={<h1 className="page-toolbar__title">{t('menu.plugins')}</h1>}
      right={<Button ref={chooseButton} onClick={() => void inspect()} disabled={busy}>{t('plugins.choose')}</Button>} />
    <h2>{t('plugins.installed')}</h2>
    {items.length > 0 ? <DataGrid items={items} onActivate={row => void openDetails(row)} getItemId={row => row.id} label={t('plugins.installed')}
      columns={[{ key: 'name', label: t('plugins.name') }, { key: 'version', label: t('plugins.version') }, { key: 'origin', label: t('plugins.origin') }]} />
      : !busy && !error && <p>{t('plugins.empty')}</p>}
      </>}</TabPanel>
      {['catalog','repositories'].map(value => <TabPanel key={value} value={value}>{section === value && <PluginRepositories key={value} catalog={value === 'catalog'} onPreview={(selection, opener) => { catalogOpener.current = opener; setError(''); setPreview(selection); }} />}</TabPanel>)}
      <TabPanel value="builder">{section === 'builder' && <PluginBuilder />}</TabPanel>
    </Tabs>
    {!preview && !details && error && <><p>{t(error)}</p>{retrySync}</>}
    {details && <Modal isOpen title={details.installation.name} onClose={() => setDetails(null)} allowClose={!busy} readingMode>
      {error && <><p>{t(error)}</p>{retrySync}</>}
      {details.installation.disabled && <p>{t('plugins.disabledNotice')}</p>}
      <Button disabled={busy} onClick={() => void changeInstallation(details.installation.disabled ? 'enable' : 'disable')}>
        {t(details.installation.disabled ? 'plugins.enable' : 'plugins.disable', { name: details.installation.name })}
      </Button>
      <Button disabled={busy} onClick={() => void changeInstallation('uninstall')}>{t('plugins.uninstall', { name: details.installation.name })}</Button>
      {createdList && <p>{t('plugins.createdList', {name:createdList})}</p>}
      <h2>{t('plugins.components')}</h2>
      {details.components.length === 0 ? <p>{t('plugins.noComponents')}</p> : <ul>{details.components.map(component => <li key={component.key}>
        <strong>{component.name}</strong> · {t(componentLabel(component.kind))}<JobRequirements component={component} />
        {component.kind === 'job' && !component.reason ?
          (component.active && component.slug ?
            <Link to="/jobs" onClick={() => useNavigationStore.getState().requestResourceEdit('jobs', component.slug!)}>{t('plugins.editJob', {name:component.name})}</Link> :
            <Button disabled={busy || details.installation.disabled} onClick={() => void changeComponent(component)}>{t('plugins.prepareJob', {name:component.name})}</Button>)
          : component.kind === 'tasklistTemplate' && !component.reason ?
          <Button disabled={busy || details.installation.disabled} onClick={() => void changeComponent(component)}>{t('plugins.createList', {name:component.name})}</Button>
          : (component.kind === 'skill' || component.kind === 'profile') && !component.reason ? <>
          {component.requestsShell && <p>{t('plugins.commandPolicy')}</p>}
          {component.active && component.kind === 'skill' && <code> /{component.slug}</code>}
          <Button disabled={busy || details.installation.disabled} onClick={() => void changeComponent(component)}>{t(component.active ? 'plugins.deactivate' : 'plugins.activate', { name: component.name })}</Button>
        </> : component.kind === 'mcp' && !component.reason ? <>
          <p>{component.transport} · {component.endpoint}</p>
          <Button disabled={busy || details.installation.disabled} onClick={() => void changeComponent(component)}>{t(component.active ? 'plugins.removeServer' : 'plugins.registerServer', { name: component.name })}</Button>
          {component.active && <Button disabled={busy} onClick={() => void changeComponent(component, true)}>{t('plugins.keepServer', { name: component.name })}</Button>}
        </> : <p>{t(componentRequirement(component) ?? 'plugins.unavailable')}</p>}
      </li>)}</ul>}
    </Modal>}
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
        {preview.authentication && <><dt>{t('plugins.authentication')}</dt><dd>{preview.authentication}</dd></>}
        <dt>SHA-256</dt><dd>{preview.digest}</dd>
      </dl>
      {preview.previousDigest && <p>{t(sameRevision ? 'plugins.current' : 'plugins.updateNotice')}</p>}
      {(preview.components ?? []).length > 0 && <ul aria-label={t('plugins.components')}>{preview.components.map(component => <li key={component.key}>{component.name} · {t(componentLabel(component.kind))}<JobRequirements component={component} />{component.requestsShell && <span> · {t('plugins.commandPolicy')}</span>}{component.endpoint && <span> · {component.endpoint}</span>}{componentRequirement(component) && <p>{t(componentRequirement(component)!)}</p>}</li>)}</ul>}
      {(preview.diagnostics ?? []).length > 0 && <p>{t('plugins.warnings', { count: preview.diagnostics.length })}</p>}
      <details><summary>{t('plugins.files', { count: preview.files.length })}</summary><ul>{preview.files.map(path => <li key={path}>{path}</li>)}</ul></details>
      </> }]} />}
  </section>;
}
