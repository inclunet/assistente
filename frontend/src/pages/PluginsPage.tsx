import { repositoryError } from './pluginRepositoryError';
import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useResourceEditRequest } from '../hooks/useResourceEditRequest';
import { useNavigationStore } from '../store/navigationStore';
import { create } from 'zustand';
import { useTranslation } from 'react-i18next';
import { AddReviewedRepository, ProviderRemovalPreview, UninstallReviewed, CreateListFromTemplate, ChangeInstallation, DiscardPreview, InstalledComponents, InstallReviewed, ListInstalled, PreviewLocal, PreviewLocalZIP, RegisterProvider, RegisterMCP, RemoveMCP, PrepareJob, SetProfileActive, SetSkillActive, SyncMCP, SyncPendingJobs } from '@wailsjs/go/wailsapi/Plugins';
import type { database, jobs, plugins, wailsapi } from '@wailsjs/go/models';
import { Button } from '../components/ui/Button';
import { DataGrid } from '../components/ui/DataGrid';
import { ChatGPTConnection } from '../components/settings/ChatGPTConnection';
import { Modal } from '../components/ui/Modal';
import { trackRepositoryWrite } from './pluginRepositoryWrites';
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

function triggerParameter(trigger: jobs.Trigger) {
  switch (trigger.type) {
    case 'interval': return trigger.every;
    case 'cron': return trigger.expression;
    case 'event': return trigger.listen;
    case 'hotkey': return trigger.keys;
    case 'webhook': return trigger.path;
  }
}

function connectionLabel(component: plugins.Component) {
  const endpoint = component.processArgs?.length ? JSON.stringify([component.endpoint, ...component.processArgs]) : component.endpoint;
  return [component.transport, endpoint].filter(Boolean).join(' · ');
}

function componentLabel(kind: string) {
  switch (kind) {
    case 'agent': return 'plugins.agent';
    case 'skill': return 'plugins.skill';
    case 'mcp': return 'plugins.mcp';
    case 'tasklistTemplate': return 'plugins.tasklistTemplate';
    case 'job': return 'plugins.job';
    case 'provider': return 'plugins.provider';
    case 'profile': return 'plugins.profile';
    default: return 'plugins.invalid';
  }
}

function componentRequirement(component: plugins.Component) {
  if (component.reason === 'providerSlugTaken') return 'plugins.providerSlugTaken';
  if (component.reason === 'invalidProvider') return 'plugins.invalidProvider';
  if (component.reason === 'invalidJob') return 'plugins.invalidJob';
  if (component.reason === 'invalidProfile') return 'plugins.invalidProfile';
  if (component.reason === 'shellCommands') return 'plugins.shellPending';
  if (component.reason === 'supportingFiles') return 'plugins.filesPending';
  if (component.reason === 'missingTemplate') return 'plugins.missingTemplate';
  if (component.reason === 'invalidTemplate') return 'plugins.invalidTemplate';
  if (component.reason === 'manifestConfiguration') return 'plugins.manifestPending';
  if (component.reason === 'packagePaths') return 'plugins.packagePathsPending';
  if (component.reason === 'agentDelegation') return 'plugins.agentDelegationPending';
  if (component.reason === 'rootSkill') return 'plugins.rootSkillPending';
  if (component.reason === 'frontmatter') return 'plugins.fieldsPending';
  if (component.reason === 'configuration') return 'plugins.mcpConfigRequired';
  if ((component.kind === 'mcp' || component.kind === 'profile' || component.kind === 'job' || component.kind === 'provider' || component.kind === 'tasklistTemplate') && !component.reason) return undefined;
  return component.kind === 'skill' ? undefined : 'plugins.unavailable';
}

function JobRequirements({component}: {component: plugins.Component}) {
  const {t} = useTranslation();
  if (!component.tool) return null;
  return <p>
    {t('plugins.jobTool', {name: component.tool})}
    <span> · {component.jobMaxRunsPerHour ? t('plugins.jobRate', {limit: component.jobMaxRunsPerHour}) : t('plugins.jobDefaultRate')}</span>
    {component.jobTriggers?.map((trigger, index) => <span key={index}> · {t(`plugins.jobTrigger.${trigger.type}`)} {triggerParameter(trigger)} {trigger.when}</span>)}
  </p>;
}

function installationError(cause: unknown, fallback = 'plugins.installError') {
  const message = String(cause);
  if (message.includes('PLUGIN_MCP_SYNC_PENDING') || message.includes('PLUGIN_JOB_SYNC_PENDING')) return 'plugins.syncPending';
  if (message.includes('PLUGIN_PROVIDER_SLUG_TAKEN')) return 'plugins.providerSlugTaken';
  if (message.includes('PLUGIN_PROVIDER_SYNC_PENDING')) return 'plugins.providerSyncPending';
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
type SyncDomain = 'mcp' | 'jobs';
const domains: SyncDomain[] = ['mcp', 'jobs'];
const pendingCode = {mcp: 'PLUGIN_MCP_SYNC_PENDING', jobs: 'PLUGIN_JOB_SYNC_PENDING'};
const usePendingLifecycleSync = create<ReadonlyMap<string, {generation: symbol; deferred: boolean}>>(() => new Map());

function syncKey(session: string, domain: SyncDomain) {
  return session + '/' + domain;
}

function rememberPending(session: string, cause: unknown, discovery = false) {
  const next = new Map(usePendingLifecycleSync.getState());
  for (const domain of domains) {
    const key = syncKey(session, domain);
    if (String(cause).includes(pendingCode[domain]) && !(discovery && next.has(key))) {
      next.set(key, {generation: Symbol(), deferred: false});
    }
  }
  usePendingLifecycleSync.setState(next, true);
}

async function reconcile(session: string, domain: SyncDomain, operation: Promise<void>) {
  const key = syncKey(session, domain);
  const pending = usePendingLifecycleSync.getState().get(key);
  await operation;
  if (pending && usePendingLifecycleSync.getState().get(key)?.generation === pending.generation) {
    const next = new Map(usePendingLifecycleSync.getState());
    next.delete(key);
    usePendingLifecycleSync.setState(next, true);
  }
}

async function synchronize(session: string, selected: SyncDomain[]) {
  const results = await Promise.allSettled(selected.map(domain =>
    reconcile(session, domain, domain === 'jobs' ? SyncPendingJobs(session) : SyncMCP(session))
  ));
  const failed = selected.filter((_, index) => results[index].status === 'rejected');
  if (failed.length) throw new Error(failed.map(domain => pendingCode[domain]).join(' '));
}

async function publish(session: string, operation: Promise<unknown>, lifecycle = false) {
  const before = usePendingLifecycleSync.getState();
  const pending = operation.then(async () => {
    const current = usePendingLifecycleSync.getState();
    const outstanding = domains.filter(domain => {
      const key = syncKey(session, domain);
      return before.has(key) && before.get(key)?.generation === current.get(key)?.generation;
    });
    await synchronize(session, outstanding);
  }).catch(cause => {
    const message = installationError(cause, lifecycle ? 'plugins.componentError' : 'plugins.installError');
    if (message === 'plugins.syncPending') rememberPending(session, cause);
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
  const [chatGPTSlug, setChatGPTSlug] = useState('');
  const [chatGPTCloseBlocked, setChatGPTCloseBlocked] = useState(false);
  const [details, setDetails] = useState<{ installation: InstallationRow; components: plugins.Component[] } | null>(null);
  const [listError, setListError] = useState('');
  const [operationError, setError] = useState<string | {key:string; name:string}>('');
  const pending = usePendingLifecycleSync();
  const pendingSync = domains.some(domain => pending.has(syncKey(session, domain)));
  const newPending = domains.some(domain => pending.get(syncKey(session, domain))?.deferred === false);
  const acknowledgePending = () => {
    restoreAfterSync.current = true;
    const next = new Map(usePendingLifecycleSync.getState());
    for (const domain of domains) {
      const key = syncKey(session, domain);
      const shown = pending.get(key);
      if (shown && shown.generation === next.get(key)?.generation) next.set(key, {...shown, deferred:true});
    }
    usePendingLifecycleSync.setState(next, true);
  };
  const error = operationError || (pendingSync ? 'plugins.syncPending' : '');
  const [busy, setBusy] = useState(true);
  const errorText = typeof error === 'string' ? (error && t(error)) : t(error.key, {name:error.name});
  useEffect(() => {
    if (errorText) announce(errorText, 'assertive');
  }, [errorText, announce]);
  const detailsOpener = useRef<HTMLElement | null>(null);
  const listErrorText = listError ? t(listError) : '';
  useEffect(() => {
    if (listErrorText) announce(listErrorText, 'assertive');
  }, [listErrorText, announce]);
  const mounted = useRef(false);
  const [providerRemoval, setProviderRemoval] = useState<{ versions: Record<string,string>; opener: HTMLButtonElement } | null>(null);
  const lifecycleOpener = useRef<HTMLButtonElement | null>(null);
  const openToken = useRef<string | null>(null);
  const chooseButton = useRef<HTMLButtonElement>(null);
  const restoreFocus = useRef(false);
  const retryButton = useRef<HTMLButtonElement>(null);
  const detailsAction = useRef<HTMLButtonElement>(null);
  const tabList = useRef<HTMLDivElement>(null);
  const restoreAfterSync = useRef(false);
  const restoreAfterReload = useRef(false);
  const reloadButton = useRef<HTMLButtonElement>(null);
  const [choosingSource, setChoosingSource] = useState(false);
  const cancelSource = () => { restoreFocus.current = true; setChoosingSource(false); };
  const [storeFormOpen,setStoreFormOpen] = useState(false);
  const syncDialogOpen = newPending && !storeFormOpen && !choosingSource && !busy && !preview && !providerRemoval && !chatGPTSlug;
  const activeTab = () => tabList.current?.querySelector<HTMLElement>('[aria-selected="true"]');
  useEffect(() => {
    if (!busy && details && lifecycleOpener.current?.isConnected) lifecycleOpener.current.focus();
    if (!busy) lifecycleOpener.current = null;
    if (!details && !preview && !busy && detailsOpener.current) {
      const opener = detailsOpener.current;
      detailsOpener.current = null;
      if (opener.isConnected) opener.focus(); else chooseButton.current?.focus();
    }
    if (!preview && !choosingSource && !syncDialogOpen && !busy && restoreFocus.current) {
      restoreFocus.current = false;
      if (catalogOpener.current?.isConnected) catalogOpener.current.focus(); else (chooseButton.current ?? activeTab())?.focus();
      catalogOpener.current = null;
    }
    if (!syncDialogOpen && !busy && restoreAfterReload.current) {
      restoreAfterReload.current = false;
      if (document.activeElement === document.body) {
        const cell = chooseButton.current?.closest('[role="tabpanel"]')?.querySelector<HTMLElement>('[role="gridcell"][tabindex="0"]');
        (reloadButton.current ?? cell ?? chooseButton.current ?? activeTab())?.focus();
      }
    }
    if (!syncDialogOpen && !busy && restoreAfterSync.current) {
      restoreAfterSync.current = false;
      if (document.activeElement === document.body) {
        (retryButton.current ?? detailsAction.current ?? chooseButton.current ?? activeTab())?.focus();
      }
    }
  }, [preview, details, busy, syncDialogOpen, choosingSource]);
  const refresh = useCallback(async (failureMessage = 'plugins.listError') => {
    const pending = publications.get(session);
    if (pending) {
      try {
        await pending;
      } catch (cause) {
        if (mounted.current && !(cause === 'plugins.syncPending' && domains.some(domain => usePendingLifecycleSync.getState().has(syncKey(session, domain))))) setError(String(cause));
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
    void reconcile(session, 'jobs', SyncPendingJobs(session)).catch(() => {
      rememberPending(session, pendingCode.jobs, true);
      if (mounted.current) setError('plugins.syncPending');
    });
    void refresh()
      .finally(() => { if (mounted.current) setBusy(false); });
    return () => {
      mounted.current = false;
      if (openToken.current) discardAbandonedPreview(openToken.current);
      openToken.current = null;
    };
  }, [refresh]);
  const reloadList = async () => {
    restoreAfterReload.current = true;
    setBusy(true);
    try { await refresh(); }
    finally { if (mounted.current) setBusy(false); }
  };
  const inspect = async (zip: boolean) => {
    setChoosingSource(false);
    setBusy(true);
    setError('');
    try {
      const next = await (zip ? PreviewLocalZIP() : PreviewLocal());
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
  useResourceEditRequest('plugins', {
    ready: !busy,
    onEdit: id => {
      const installation = items?.find(item => item.id === id);
      if (installation) void openDetails(installation);
      else setError('plugins.originUnavailable');
    },
  });
  const refreshDetails = async (installation: InstallationRow) => {
    const components = await InstalledComponents(installation.id, installation.digest);
    if (mounted.current) setDetails({ installation, components: components ?? [] });
  };
  const closeChatGPT = async () => {
    if (!details || chatGPTCloseBlocked) return;
    setChatGPTSlug('');
    setBusy(true);
    try { await refreshDetails(details.installation); }
    catch { if (mounted.current) setError('plugins.listError'); }
    finally { if (mounted.current) setBusy(false); }
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
        if (listRequests.get(key) === request) listRequests.delete(key);
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
          await reconcile(session, 'mcp', RemoveMCP(installation.id, installation.digest, component.key, preserve));
        } else {
          await reconcile(session, 'mcp', RegisterMCP(installation.id, installation.digest, component.key));
        }
      } else if (component.kind === 'provider') {
        const title = t(component.active ? 'plugins.syncProvider' : 'plugins.registerProvider', { name: component.name });
        const accepted = await confirm({ title, message: component.active ? t('plugins.syncLocalProvider', { name: component.name }) : connectionLabel(component), confirmText: title, cancelText: t('common.cancel') });
        if (!accepted || !mounted.current) return;
        await RegisterProvider(installation.id, installation.digest, component.key);
      } else if (component.kind === 'job') {
        await PrepareJob(installation.id, installation.digest, component.key);
      } else if (component.kind === 'profile') {
        await SetProfileActive(installation.id, installation.digest, component.key, !component.active);
      } else {
        await SetSkillActive(installation.id, installation.digest, component.key, !component.active);
      }
      if (!mounted.current) return;
      try {
        await refreshDetails(installation);
        if (mounted.current) {
          announce(t(component.kind === 'provider' ? 'plugins.providerRegistered' : component.kind === 'job' ? 'plugins.jobPrepared' : component.kind === 'mcp' ? 'plugins.serverChanged' : component.active ? 'plugins.deactivated' : 'plugins.activated', { name: component.name }));
        }
      } catch {
        if (mounted.current) {
          setDetails(null);
          setError('plugins.componentReloadError');
        }
      }
    } catch (cause) {
      if (!mounted.current) return;
      if (installationError(cause) === 'plugins.syncPending') {
        rememberPending(session, cause);
        try {
          await refreshDetails(installation);
          if (mounted.current) setError('plugins.syncPending');
        } catch { if (mounted.current) setError('plugins.syncPendingStale'); }
      } else if (String(cause).includes('PLUGIN_LIST_CAPACITY')) setError('plugins.listCapacity');
      else {
        const missing = /PLUGIN_PROFILE_(PROVIDER|ALLOWLIST)_MISSING: (.+)$/.exec(String(cause));
        if (missing) {
          setError({ key: missing[1] === 'ALLOWLIST' ? 'plugins.missingProfileAllowlist' : 'plugins.missingProfileProvider', name: missing[2] });
          return;
        }
        const dependency = /PLUGIN_JOB_(PIPELINE_UNAVAILABLE|TOOL_UNAVAILABLE|TOOL_AMBIGUOUS): (.+)$/.exec(String(cause));
        if (dependency) setError({key: 'plugins.jobDependency.' + dependency[1], name:dependency[2]});
        else setError(installationError(cause, 'plugins.componentError'));
      }
    }
    finally { if (mounted.current) setBusy(false); }
  };
  const retryPendingSync = async () => {
    if (busy) return;
    restoreAfterSync.current = true;
    setBusy(true);
    try {
      const needed = domains.filter(domain => usePendingLifecycleSync.getState().has(syncKey(session, domain)));
      await synchronize(session, needed);
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
  const syncAction = pendingSync && <Button ref={retryButton} disabled={busy} onClick={() => void retryPendingSync()}>{t('plugins.retrySync')}</Button>;
  const changeInstallation = async (action: 'enable' | 'disable' | 'uninstall', opener: HTMLButtonElement, decision?: {choice: 'detach' | 'remove'; versions: Record<string,string>}) => {
    if (busy || !details || error === 'plugins.syncPendingStale') return;
    setBusy(true);
    setError('');
    const { installation } = details;
    lifecycleOpener.current = opener;
    try {
      if (action === 'uninstall' && !decision) {
        const versions = await ProviderRemovalPreview(installation.id, installation.digest);
        if (!mounted.current) return;
        if (Object.keys(versions).length > 0) { setProviderRemoval({ versions, opener }); return; }
      }
      const accepted = decision || await confirm({
        title: t('plugins.' + action, { name: installation.name }),
        message: [t('plugins.' + action + 'Confirm'), t('plugins.origin') + ': ' + installation.origin].join('\n'),
        confirmText: t('plugins.' + action, { name: installation.name }),
        cancelText: t('common.cancel'),
      });
      if (!accepted || !mounted.current) return;
      let syncPending = false;
      try {
        await publish(session, decision ? UninstallReviewed(installation.id, installation.digest, decision.choice, decision.versions) : ChangeInstallation(installation.id, installation.digest, action), true);
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
    <h1 className="page-toolbar__title">{t('menu.plugins')}</h1>
    <Tabs value={section} onValueChange={setSection}>
      <TabList listRef={tabList} ariaLabel={t('menu.plugins')}>{['installed','catalog','repositories','builder'].map(value => <Tab value={value} key={value}>{t('plugins.' + value)}</Tab>)}</TabList>
      <TabPanel value="installed">{section === 'installed' && <>
    <Toolbar ariaLabel={t('plugins.installed')} left={<h2>{t('plugins.installed')}</h2>}
      right={<><Button ref={chooseButton} onClick={() => setChoosingSource(true)} disabled={busy}>{t('plugins.choose')}</Button>{!details && !preview && syncAction}
        {listError && <Button ref={reloadButton} disabled={busy} onClick={() => void reloadList()}>{t('plugins.reloadList')}</Button>}</>} />
    {items && items.length > 0 ? <DataGrid items={items} onActivate={row => void openDetails(row)} getItemId={row => row.id} label={t('plugins.installed')}
      columns={[{ key: 'name', label: t('plugins.name') }, { key: 'version', label: t('plugins.version') }, { key: 'origin', label: t('plugins.origin') }]} />
      : items && !busy && !error && !listError && <p>{t('plugins.empty')}</p>}
      </>}</TabPanel>
      {['catalog','repositories'].map(value => <TabPanel key={value} value={value}>{section === value && <PluginRepositories onFormOpenChange={setStoreFormOpen} key={value} session={session} parentBusy={busy} catalog={value === 'catalog'} onPreview={(selection, opener) => { catalogOpener.current = opener; openToken.current = selection.token; setError(''); setPreview(selection); }} />}</TabPanel>)}
      <TabPanel value="builder">{section === 'builder' && <PluginBuilder formRef={builderForm} />}</TabPanel>
    </Tabs>
    {!preview && !details && error && <p>{errorText}</p>}
    {listErrorText && <p>{listErrorText}</p>}
    {details && <Modal isOpen title={details.installation.name} onClose={() => setDetails(null)} allowClose={!busy} readingMode>
      {error && <p>{errorText}</p>}
      {syncAction}
      <p>{t('plugins.origin')}: {details.installation.origin}</p>
      {details.installation.disabled && <p>{t('plugins.disabledNotice')}</p>}
      <Button ref={detailsAction} disabled={busy || error === 'plugins.syncPendingStale'} onClick={event => void changeInstallation(details.installation.disabled ? 'enable' : 'disable', event.currentTarget)}>
        {t(details.installation.disabled ? 'plugins.enable' : 'plugins.disable', { name: details.installation.name })}
      </Button>
      <Button disabled={busy || error === 'plugins.syncPendingStale'} onClick={event => void changeInstallation('uninstall', event.currentTarget)}>{t('plugins.uninstall', { name: details.installation.name })}</Button>
      {createdList && <p>{t('plugins.createdList', { name: createdList })}</p>}
      <h2>{t('plugins.components')}</h2>
      {details.components.length === 0 ? <p>{t('plugins.noComponents')}</p> : <ul>{details.components.map(component => <li key={component.key}>
        <strong>{component.name}</strong> · {t(componentLabel(component.kind))}<JobRequirements component={component} />
        {component.editedServer && <p>{t('plugins.editedServer', { name: component.editedServer })}</p>}
        {component.missingAllowlist && <p>{t('plugins.missingProfileAllowlist',{name:component.missingAllowlist})}</p>}{component.missingProvider && <p>{t('plugins.missingProfileProvider',{name:component.missingProvider})}</p>}{component.profileDefinition && <details><summary>{t('plugins.profileSettings')}</summary><pre>{component.profileDefinition}</pre></details>}{(component.kind === 'mcp' || component.kind === 'provider') && <p>{connectionLabel(component)}</p>}
        {component.kind === 'skill' && component.requestsShell && <p>{t('plugins.commandPolicy')}</p>}
        {component.kind === 'provider' && !component.reason ?
          <>{component.active && <p>{t('plugins.providerRegistered', { name: component.name })}</p>}
            <Button disabled={busy || details.installation.disabled || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component)}>{t(component.active ? 'plugins.syncProvider' : 'plugins.registerProvider', { name: component.name })}</Button></>
          : component.kind === 'job' && !component.reason ?
          (component.active && component.slug ?
            <Link to="/jobs" onClick={() => useNavigationStore.getState().requestResourceEdit('jobs', component.slug!)}>{t('plugins.editJob', {name:component.name})}</Link> :
            <Button disabled={busy || details.installation.disabled || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component)}>{t('plugins.prepareJob', {name:component.name})}</Button>)
          : component.kind === 'tasklistTemplate' && !component.reason ?
          <Button disabled={busy || details.installation.disabled || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component)}>{t('plugins.createList', { name: component.name })}</Button>
          : (component.kind === 'profile' || component.kind === 'skill') && !component.reason ? <>
          {component.kind === 'profile' && component.canCreateChatGPT && component.missingProvider && !details.components.some(c => c.kind === 'provider' && c.slug === component.missingProvider) &&
            <Button disabled={busy || details.installation.disabled || error === 'plugins.syncPendingStale'} onClick={() => setChatGPTSlug(component.missingProvider!)}>{t('plugins.createProfileChatGPT', { name: component.missingProvider })}</Button>}
          {component.active && component.kind === 'skill' && <code> /{component.slug}</code>}
          <Button disabled={busy || details.installation.disabled || (!component.active && !!(component.missingProvider || component.missingAllowlist)) || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component)}>{t(component.active ? 'plugins.deactivate' : 'plugins.activate', { name: component.name })}</Button>
        </> : component.kind === 'mcp' && !component.reason ? <>
          <Button disabled={busy || details.installation.disabled || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component)}>{t(component.active ? 'plugins.removeServer' : 'plugins.registerServer', { name: component.name })}</Button>
          {component.active && <Button disabled={busy || error === 'plugins.syncPendingStale'} onClick={() => void changeComponent(component, true)}>{t('plugins.keepServer', { name: component.name })}</Button>}
        </> : <p>{t(componentRequirement(component) ?? 'plugins.unavailable', { path: component.entry })}</p>}
      </li>)}</ul>}
    </Modal>}
    {choosingSource && <DecisionDialog isOpen title={t('plugins.choose')} description={t('plugins.sourcePrompt')}
      actions={[{id:'folder', label:t('plugins.folderSource'), primary:true}, {id:'zip',label:t('plugins.zipSource')}, {id:'cancel', label:t('common.cancel')}]}
      onAction={id => { if (id === 'cancel') cancelSource(); else void inspect(id === 'zip'); }}
      onCancel={cancelSource} />}
    {syncDialogOpen && <DecisionDialog isOpen
      title={t('plugins.syncQuestion')} description={t('plugins.syncPrompt')}
      actions={[{id:'retry', label:t('plugins.syncNow'), primary:true}, {id:'later', label:t('plugins.syncLater')}]}
      onAction={id => { acknowledgePending(); if (id === 'retry') void retryPendingSync(); }}
      onCancel={acknowledgePending} />}
    {chatGPTSlug && details && <Modal isOpen title={t('chatgpt.title')} allowClose={!chatGPTCloseBlocked}
      onClose={() => void closeChatGPT()}>
      <ChatGPTConnection profileSlug={chatGPTSlug} onCloseBlockedChange={setChatGPTCloseBlocked}
        onClose={() => void closeChatGPT()} />
    </Modal>}
    {providerRemoval && details && <DecisionDialog isOpen
      title={t('plugins.uninstall', {name: details.installation.name})}
      description={[t('plugins.uninstallConfirm'), t('plugins.origin') + ': ' + details.installation.origin, t('plugins.providerRemovalPrompt', {providers: Object.keys(providerRemoval.versions).sort().join(', ')})].join('\n')}
      actions={[
        {id:'detach', label:t('plugins.detachProviders'), primary:true},
        {id:'remove', label:t('plugins.removeProviders')},
        {id:'cancel', label:t('common.cancel')},
      ]}
      onCancel={() => setProviderRemoval(null)}
      onAction={choice => {
        setProviderRemoval(null);
        if (choice === 'detach' || choice === 'remove') void changeInstallation('uninstall', providerRemoval.opener, {choice, versions:providerRemoval.versions});
      }} />}
    {preview?.repository && <DecisionDialog isOpen title={t('plugins.storeDetected')} description={[t('plugins.storeDetectedPrompt',{name:preview.name,origin:preview.origin}),error && errorText].filter(Boolean).join('\n')}
      allowClose={!busy} actions={[{id:'add',label:t('plugins.addRepo'),primary:true,disabled:busy},{id:'cancel',label:t('common.cancel'),disabled:busy}]}
      onCancel={() => void discard()} onAction={async id => {
        if (id !== 'add') { await discard(); return; }
        setBusy(true);
        openToken.current = null;
        try {
          await trackRepositoryWrite(session,AddReviewedRepository(preview.token).then(() => true));
          if (mounted.current) { announce(t('plugins.repoUpdated')); setSection('repositories'); }
        } catch (cause) { if (mounted.current) setError(repositoryError(cause,'plugins.repositoryReadError')); }
        finally { if (mounted.current) { restoreFocus.current = true; setPreview(null); setBusy(false); } }
      }} />}
    {preview && !preview.repository && <DecisionDialog isOpen title={t('plugins.preview', { name: preview.name })}
      description={preview.previousDigest && !sameRevision ? t(previewDescription) + ' ' + t('plugins.preserveJobNotice') : t(previewDescription)} allowClose={!busy} size="lg"
      actions={[
        { id: 'install', label: t(preview.previousDigest ? 'plugins.update' : 'plugins.install'), primary: true, polarity: 'affirmative', disabled: busy || sameRevision || editedServers.length > 0 },
        { id: 'cancel', label: t('common.cancel'), disabled: busy },
      ]}
      onAction={id => { if (id === 'install') void install(); else void discard(); }}
      onCancel={() => void discard()}
      readingRegions={[{ id: 'package', label: t('plugins.preview', { name: preview.name }), content: <>
      {error && <p>{errorText}</p>}
      <dl>
        <dt>{t('plugins.origin')}</dt><dd>{preview.origin}</dd>
        {preview.version && <><dt>{t('plugins.version')}</dt><dd>{preview.version}</dd></>}
        {preview.previousDigest && <><dt>{t('plugins.previous')}</dt><dd>{preview.previousDigest}</dd></>}
        {preview.authentication && <><dt>{t('plugins.authentication')}</dt><dd>{preview.authentication}</dd></>}
        <dt>SHA-256</dt><dd>{preview.digest}</dd>
      </dl>
      {(preview.components ?? []).length > 0 && <ul aria-label={t('plugins.components')}>{preview.components.map(component => <li key={component.key}>{component.name} · {t(componentLabel(component.kind))}<JobRequirements component={component} />{component.requestsShell && <span> · {t('plugins.commandPolicy')}</span>}{component.missingAllowlist && <p>{t('plugins.missingProfileAllowlist',{name:component.missingAllowlist})}</p>}{component.missingProvider && <p>{t('plugins.missingProfileProvider',{name:component.missingProvider})}</p>}{component.profileDefinition && <details><summary>{t('plugins.profileSettings')}</summary><pre>{component.profileDefinition}</pre></details>}{(component.kind === 'mcp' || component.kind === 'provider') && <p>{connectionLabel(component)}</p>}{componentRequirement(component) && <p>{t(componentRequirement(component)!, { path: component.entry })}</p>}</li>)}</ul>}
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
