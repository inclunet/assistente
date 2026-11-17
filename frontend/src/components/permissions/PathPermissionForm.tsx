import { FormEvent, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AddPathAllowlistEntry } from '@wailsjs/go/wailsapi/FSTrust';
import { Input } from '../ui/Input';
import { Select } from '../ui/Select';
import { Button } from '../ui/Button';
import { DialogActions } from '../ui/DialogActions';
import { useUIStore } from '../../store/uiStore';

export function PathPermissionForm({ onSaved, onCancel }: { onSaved: () => void; onCancel: () => void }) {
  const { t } = useTranslation();
  const toast = useUIStore((state) => state.addToast);
  const [form, setForm] = useState({ path: '', kind: 'file', operation: '', effect: 'deny', scope: 'workspace', reason: '' });
  const [busy, setBusy] = useState(false);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    try {
      await AddPathAllowlistEntry(form.path.trim(), form.kind, form.operation.trim(), form.effect, form.scope, form.reason.trim());
      toast(t(form.effect === 'deny' ? 'pathAllowlist.toast.denyAdded' : 'pathAllowlist.toast.allowAdded'), 'success');
      onSaved();
    } catch { toast(t('pathAllowlist.error.addRuleFailed'), 'error'); }
    finally { setBusy(false); }
  };
  const options = (values: string[], prefix: string) => values.map((value) => ({ value, label: t(`pathAllowlist.${prefix}.${value}`) }));
  return <form onSubmit={(event) => void submit(event)} className="permission-path-form">
    <Input id="path-rule-path" label={t('pathAllowlist.form.path')} value={form.path} required onChange={(e) => setForm({ ...form, path: e.target.value })} />
    <Select id="path-rule-kind" label={t('pathAllowlist.form.kind')} value={form.kind} options={options(['file', 'dir'], 'kind')} onChange={(e) => setForm({ ...form, kind: e.target.value })} />
    <Input id="path-rule-operation" label={t('pathAllowlist.form.operation')} value={form.operation} required onChange={(e) => setForm({ ...form, operation: e.target.value })} />
    <Select id="path-rule-effect" label={t('pathAllowlist.form.effect')} value={form.effect} options={options(['allow', 'deny'], 'effect')} onChange={(e) => setForm({ ...form, effect: e.target.value })} />
    <Select id="path-rule-scope" label={t('pathAllowlist.form.scope')} value={form.scope} options={options(['workspace', 'profile', 'global'], 'scope')} onChange={(e) => setForm({ ...form, scope: e.target.value })} />
    <Input id="path-rule-reason" label={t('pathAllowlist.form.reason')} value={form.reason} onChange={(e) => setForm({ ...form, reason: e.target.value })} />
    <DialogActions
      primary={<Button type="submit" variant="primary" loading={busy}>{t(form.effect === 'allow' ? 'pathAllowlist.form.submitAllow' : 'pathAllowlist.form.submitDeny')}</Button>}
      secondary={<Button onClick={onCancel} disabled={busy}>{t('common.cancel')}</Button>}
    />
  </form>;
}
