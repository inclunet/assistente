import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { GetPermissions, RevokePermission } from '@wailsjs/go/wailsapi/Permissions';
import { controllers } from '../../wailsjs/go/models';
import { Toolbar } from '../components/ui/Toolbar';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Select } from '../components/ui/Select';
import { Modal } from '../components/ui/Modal';
import { PageLoading } from '../components/ui/PageLoading';
import { useConfirm } from '../hooks/useConfirm';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useUIStore } from '../store/uiStore';
import { useAuthStore } from '../store/authStore';
import { LegacyPermissionAssignment } from '../components/permissions/LegacyPermissionAssignment';
import AllowlistPage from './AllowlistPage';
import ProfilesPage from './ProfilesPage';
import { PathPermissionForm } from '../components/permissions/PathPermissionForm';
import './PermissionsPage.css';

type Permission = controllers.PermissionView;
const domains = ['network', 'filesystem', 'shell', 'acp'];
const scopes = ['session', 'workspace', 'profile', 'global'];
const origins = ['consent', 'migration'];
const effects = ['allow', 'deny', 'ask'];

export default function PermissionsPage() {
  const { t, i18n } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const isAdmin = useAuthStore((s) => s.user?.role === 'admin');
  const confirm = useConfirm();
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'permissions-page' });
  const [rows, setRows] = useState<Permission[]>([]);
  const [errors, setErrors] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);
  const [focused, setFocused] = useState<Permission | null>(null);
  const [domain, setDomain] = useState('');
  const [scope, setScope] = useState('');
  const [origin, setOrigin] = useState('');
  const [effect, setEffect] = useState('');
  const [status, setStatus] = useState('active');
  const [search, setSearch] = useState('');
  const [editor, setEditor] = useState<'policies' | 'associations' | 'path' | 'migration' | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setFocused(null);
    try {
      const snapshot = await GetPermissions();
      setRows(snapshot.rules ?? []);
      setErrors(snapshot.migrationErrors ?? []);
      if (snapshot.migrationErrors?.length) announce(t('permissions.incomplete'));
      setFailed(false);
    } catch {
      setFailed(true);
      addToast(t('permissions.loadFailed'), 'error');
    } finally { setLoading(false); }
  }, [addToast, t, announce]);
  useEffect(() => { void load(); }, [load]);

  const visible = useMemo(() => rows.filter((row) =>
    (!domain || row.domain === domain) && (!scope || row.scope === scope) &&
    (!origin || row.origin === origin) && (!effect || row.effect === effect) &&
    (!status || (status === 'revoked' ? !!row.revokedAt : !row.revokedAt)) &&
    (!search || `${row.resource} ${row.scopeKey} ${row.operation} ${row.createdAt}`.toLocaleLowerCase().includes(search.toLocaleLowerCase())),
  ), [rows, domain, scope, origin, effect, status, search]);

  const revoke = useCallback(async (row: Permission) => {
    if (row.revokedAt) return;
    const accepted = await confirm({
      title: t('permissions.revoke'),
      message: t(row.effect === 'deny' ? 'permissions.confirmDenyRemoval' : 'permissions.confirmRevoke', { resource: row.resource || row.resourceKind, scope: t(`permissions.scope.${row.scope}`), scopeName: row.scopeKey }),
      confirmText: t('permissions.revoke'), cancelText: t('common.cancel'), variant: 'danger',
    });
    if (!accepted) return;
    try {
      await RevokePermission(row.id);
      setRows((current) => current.filter((item) => item.id !== row.id));
      setFocused(null);
      addToast(t('permissions.revoked'), 'success');
      await load();
    } catch { addToast(t('permissions.revokeFailed'), 'error'); }
  }, [confirm, t, addToast, load]);

  const date = (value: string) => value && !Number.isNaN(Date.parse(value)) ? new Date(value).toLocaleString(i18n.language) : '';
  const columns: DataGridColumn<Permission>[] = [
    { key: 'domain', label: t('permissions.domainLabel'), format: (_, row) => t(`permissions.domain.${row.domain}`) },
    { key: 'resource', label: t('permissions.resource'), format: (_, row) => row.domain === 'acp' ? `${row.resource || t('permissions.legacyAgent')} — ${row.resourceKind.replace('legacy:', '')}` : row.resource },
    { key: 'operation', label: t('permissions.operation') },
    { key: 'resourceKind', label: t('permissions.resourceKind'), format: (_, row) => row.domain === 'filesystem' ? t(`pathAllowlist.kind.${row.resourceKind}`) : row.resourceKind },
    { key: 'scope', label: t('permissions.scopeLabel'), format: (_, row) => t(`permissions.scope.${row.scope}`) },
    { key: 'scopeKey', label: t('permissions.context') },
    { key: 'effect', label: t('permissions.effectLabel'), format: (_, row) => t(`permissions.effect.${row.effect}`) },
    { key: 'origin', label: t('permissions.originLabel'), format: (_, row) => t(`permissions.origin.${row.origin}`) },
    { key: 'createdAt', label: t('permissions.createdAt'), format: (_, row) => date(row.createdAt) },
    { key: 'revokedAt', label: t('permissions.revokedAt'), format: (_, row) => date(row.revokedAt) },
    { key: 'expiresAt', label: t('permissions.expiresAt'), format: (_, row) => date(row.expiresAt) },
  ];
  const options = (values: string[], prefix: string) => [{ value: '', label: t('permissions.all') }, ...values.map((value) => ({ value, label: t(`permissions.${prefix}.${value}`) }))];
  const closeEditor = () => { setEditor(null); void load(); };

  return <div className="permissions-page">
    <Toolbar ariaLabel={t('permissions.title')} left={<h1 className="page-toolbar__title">{t('permissions.title')}</h1>}
      searchValue={search} onSearchChange={setSearch} searchPlaceholder={t('permissions.search')}
      actions={[
        { key: 'revoke', label: t('permissions.revoke'), disabled: !focused || !!focused.revokedAt || loading, onClick: () => focused && void revoke(focused), variant: 'danger' },
        { key: 'reload', label: t('permissions.reload'), disabled: loading, onClick: () => void load() },
        { key: 'policies', label: t('permissions.policies'), onClick: () => setEditor('policies') },
        { key: 'associations', label: t('permissions.associations'), onClick: () => setEditor('associations') },
        { key: 'path', label: t('permissions.addPath'), onClick: () => setEditor('path') },
        ...(isAdmin ? [{ key: 'migration', label: t('permissions.assign'), onClick: () => setEditor('migration') }] : []),
      ]} />
    <div className="permissions-page__content">
      <p>{t('permissions.description')}</p>
      <p>{t('permissions.acpLimit')}</p>
      <div className="permissions-page__filters">
        <Select id="permission-domain" label={t('permissions.domainLabel')} value={domain} options={options(domains, 'domain')} onChange={(e) => setDomain(e.target.value)} />
        <Select id="permission-scope" label={t('permissions.scopeLabel')} value={scope} options={options(scopes, 'scope')} onChange={(e) => setScope(e.target.value)} />
        <Select id="permission-origin" label={t('permissions.originLabel')} value={origin} options={options(origins, 'origin')} onChange={(e) => setOrigin(e.target.value)} />
        <Select id="permission-effect" label={t('permissions.effectLabel')} value={effect} options={options(effects, 'effect')} onChange={(e) => setEffect(e.target.value)} />
        <Select id="permission-status" label={t('permissions.statusLabel')} value={status} options={options(['active', 'revoked'], 'status')} onChange={(e) => setStatus(e.target.value)} />
      </div>
      {errors.length > 0 && <div><p>{t('permissions.incomplete')}</p><ul>{errors.map((error, index) => <li key={`${index}:${error}`}>{error}</li>)}</ul></div>}
      {loading ? <PageLoading message={t('permissions.loading')} /> : failed ? <p>{t('permissions.loadFailed')}</p> :
        <DataGrid items={visible} columns={columns} label={t('permissions.title')} getItemId={(row) => row.id}
          onGridReady={handleGridReady} onFocusChange={setFocused} autoFocusOnMount={false}
          getRowActions={(row) => row.revokedAt ? [] : [{ id: 'revoke', label: t('permissions.revoke'), danger: true, onClick: () => void revoke(row) }]} />}
    </div>
    <Modal isOpen={editor !== null} onClose={closeEditor} title={t(editor === 'policies' ? 'permissions.policies' : editor === 'associations' ? 'permissions.associations' : editor === 'migration' ? 'permissions.assign' : 'permissions.addPath')}>
      {editor === 'policies' ? <AllowlistPage /> : editor === 'associations' ? <ProfilesPage /> : editor === 'path' ? <PathPermissionForm onSaved={closeEditor} onCancel={closeEditor} /> : editor === 'migration' ? <LegacyPermissionAssignment onSaved={closeEditor} onCancel={closeEditor} /> : null}
    </Modal>
  </div>;
}
