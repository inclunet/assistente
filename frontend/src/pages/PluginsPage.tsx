import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DiscardPreview, InstalledComponents, InstallReviewed, ListInstalled, PreviewLocal, SetSkillActive } from '@wailsjs/go/wailsapi/Plugins';
import type { database, plugins, wailsapi } from '@wailsjs/go/models';
import { Button } from '../components/ui/Button';
import { DataGrid } from '../components/ui/DataGrid';
import { Modal } from '../components/ui/Modal';
import { DecisionDialog } from '../components/ui/DecisionDialog';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { Toolbar } from '../components/ui/Toolbar';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useAuthStore } from '../store/authStore';

function componentRequirement(component: plugins.Component) {
  if (component.reason === 'shellCommands') return 'plugins.shellPending';
  if (component.reason === 'supportingFiles') return 'plugins.filesPending';
  if (component.reason === 'frontmatter') return 'plugins.fieldsPending';
  return component.kind === 'skill' ? undefined : 'plugins.unavailable';
}

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
  const [details, setDetails] = useState<{ installation: database.PluginInstallation; components: plugins.Component[] } | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(true);
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
      chooseButton.current?.focus();
    }
  }, [preview, details, busy]);
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
  const toggleSkill = async (component: plugins.Component) => {
    if (!details || busy) return;
    setBusy(true);
    setError('');
    const { installation } = details;
    try {
      await SetSkillActive(installation.id, installation.digest, component.key, !component.active);
      const components = await InstalledComponents(installation.id, installation.digest);
      if (mounted.current) {
        setDetails({ installation, components: components ?? [] });
        announce(t(component.active ? 'plugins.deactivated' : 'plugins.activated', { name: component.name }));
      }
    } catch { if (mounted.current) setError('plugins.componentError'); }
    finally { if (mounted.current) setBusy(false); }
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
    {!preview && !details && error && <p>{t(error)}</p>}
    {items.length > 0 ? <DataGrid items={items} onActivate={row => void openDetails(row)} getItemId={row => row.id} label={t('plugins.installed')}
      columns={[{ key: 'name', label: t('plugins.name') }, { key: 'version', label: t('plugins.version') }, { key: 'origin', label: t('plugins.origin') }]} />
      : !busy && !error && <p>{t('plugins.empty')}</p>}
    {details && <Modal isOpen title={details.installation.name} onClose={() => setDetails(null)} allowClose={!busy} readingMode>
      {error && <p>{t(error)}</p>}
      <h2>{t('plugins.components')}</h2>
      {details.components.length === 0 ? <p>{t('plugins.noComponents')}</p> : <ul>{details.components.map(component => <li key={component.key}>
        <strong>{component.name}</strong> · {t(component.kind === 'skill' ? 'plugins.skill' : component.kind === 'mcp' ? 'plugins.mcp' : 'plugins.invalid')}
        {component.kind === 'skill' && !component.reason ? <>
          {component.active && <code> /{component.slug}</code>}
          <Button disabled={busy} onClick={() => void toggleSkill(component)}>{t(component.active ? 'plugins.deactivate' : 'plugins.activate', { name: component.name })}</Button>
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
        <dt>SHA-256</dt><dd>{preview.digest}</dd>
      </dl>
      {preview.previousDigest && <p>{t(sameRevision ? 'plugins.current' : 'plugins.updateNotice')}</p>}
      {(preview.components ?? []).length > 0 && <ul aria-label={t('plugins.components')}>{preview.components.map(component => <li key={component.key}>{component.name} · {t(component.kind === 'skill' ? 'plugins.skill' : component.kind === 'mcp' ? 'plugins.mcp' : 'plugins.invalid')}{componentRequirement(component) && <p>{t(componentRequirement(component)!)}</p>}</li>)}</ul>}
      {(preview.diagnostics ?? []).length > 0 && <p>{t('plugins.warnings', { count: preview.diagnostics.length })}</p>}
      <details><summary>{t('plugins.files', { count: preview.files.length })}</summary><ul>{preview.files.map(path => <li key={path}>{path}</li>)}</ul></details>
      </> }]} />}
  </section>;
}
