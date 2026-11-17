import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AssignPermissionOrigin, GetPermissionMigration } from '@wailsjs/go/wailsapi/Permissions';
import { wailsapi } from '../../../wailsjs/go/models';
import { Select } from '../ui/Select';
import { Button } from '../ui/Button';
import { DialogActions } from '../ui/DialogActions';
import { useUIStore } from '../../store/uiStore';

export function LegacyPermissionAssignment({ onSaved, onCancel }: { onSaved: () => void; onCancel: () => void }) {
  const { t } = useTranslation();
  const toast = useUIStore((s) => s.addToast);
  const [migration, setMigration] = useState<wailsapi.PermissionMigration | null>(null);
  const [origin, setOrigin] = useState('');
  const [owner, setOwner] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let active = true;
    void GetPermissionMigration().then((result) => { if (active) setMigration(result); }).catch(() => toast(t('permissions.loadFailed'), 'error'));
    return () => { active = false; };
  }, [toast, t]);
  const assign = async () => {
    setBusy(true);
    try { await AssignPermissionOrigin(origin, owner); toast(t('permissions.assigned'), 'success'); onSaved(); }
    catch { toast(t('permissions.assignmentFailed'), 'error'); }
    finally { setBusy(false); }
  };
  return <div className="permission-path-form">
    <p>{t('permissions.assignmentHelp')}</p>
    <Select id="permission-legacy-origin" label={t('permissions.originLabel')} value={origin}
      options={[{ value: '', label: t('permissions.choose') }, ...(migration?.origins ?? []).map((item) => ({ value: item.id, label: `${t(`permissions.domain.${item.domain}`)} — ${item.source}` }))]}
      onChange={(event) => setOrigin(event.target.value)} />
    <Select id="permission-legacy-owner" label={t('permissions.owner')} value={owner}
      options={[{ value: '', label: t('permissions.choose') }, ...(migration?.owners ?? []).map((item) => ({ value: item.id, label: item.name }))]}
      onChange={(event) => setOwner(event.target.value)} />
    <DialogActions
      primary={<Button variant="primary" onClick={() => void assign()} disabled={!origin || !owner || busy}>{t('permissions.assign')}</Button>}
      secondary={<Button onClick={onCancel} disabled={busy}>{t('common.cancel')}</Button>}
    />
  </div>;
}
