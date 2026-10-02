import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CreateMCPOAuthSnapshot, ListMCPOAuthSnapshots, RestoreMCPOAuthSnapshot, DiscardMCPOAuthSnapshot } from '@wailsjs/go/wailsapi/MCP';
import type { credentials, mcp } from '../../../wailsjs/go/models';
import { Button } from '../ui/Button';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useConfirm } from '../../hooks/useConfirm';
import './McpOAuthSnapshots.css';

export function McpOAuthSnapshots({ consumers }: { consumers: mcp.OAuthInventoryItem[] }) {
  const { t, i18n } = useTranslation();
  const { announce } = useAnnouncer();
  const confirm = useConfirm();
  const [snapshots, setSnapshots] = useState<credentials.OAuthSnapshotInfo[] | null>(null);
  const [selected, setSelected] = useState('');
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const active = useRef(false);
  const consumerSelect = useRef<HTMLSelectElement>(null);
  const restoreFocus = useRef(false);
  const query = useRef<Promise<credentials.OAuthSnapshotInfo[]> | null>(null);
  const eligible = consumers.filter((item) => item.kind === 'legacy');

  useEffect(() => {
    active.current = true;
    let current = true;
    if (!query.current) query.current = Promise.resolve().then(() => ListMCPOAuthSnapshots());
    void query.current.then((items) => {
      if (current) setSnapshots(items || []);
    }).catch(() => {
      if (current) { setFailed(true); announce(t('mcp.snapshots.failed'), 'assertive'); }
    });
    return () => { current = false; active.current = false; };
  }, [announce, t]);

  useEffect(() => {
    if (!busy && restoreFocus.current) {
      restoreFocus.current = false;
      consumerSelect.current?.focus();
    }
  }, [busy]);

  async function run(action: () => Promise<unknown>, success: string) {
    if (!active.current) return;
    setBusy(true);
    setFailed(false);
    try {
      await action();
      if (!active.current) return;
      const items = await ListMCPOAuthSnapshots();
      if (active.current) { query.current = Promise.resolve(items); setSnapshots(items); announce(t(success)); }
    } catch {
      if (active.current) { setFailed(true); announce(t('mcp.snapshots.failed'), 'assertive'); }
    } finally {
      if (active.current) setBusy(false);
    }
  }

  async function restore(id: string) {
    const accepted = await confirm({ title: t('mcp.snapshots.restore'), message: t('mcp.snapshots.restoreConfirm'), confirmText: t('mcp.snapshots.restore'), cancelText: t('common.cancel'), variant: 'warning' });
    if (accepted && active.current) await run(() => RestoreMCPOAuthSnapshot(id), 'mcp.snapshots.restored');
  }
  async function discard(id: string) {
    const accepted = await confirm({ title: t('mcp.snapshots.discard'), message: t('mcp.snapshots.discardConfirm'), confirmText: t('mcp.snapshots.discard'), cancelText: t('common.cancel'), variant: 'danger' });
    if (accepted && active.current) {
      restoreFocus.current = true;
      await run(() => DiscardMCPOAuthSnapshot(id, true), 'mcp.snapshots.discarded');
    }
  }

  return <section className="mcp-oauth-snapshots" aria-label={t('mcp.snapshots.title')} aria-busy={busy}>
    <h2>{t('mcp.snapshots.title')}</h2>
    <p>{t('mcp.snapshots.description')}</p>
    <p>{t('mcp.snapshots.retention')}</p>
    <label htmlFor="oauth-snapshot-consumer">{t('mcp.snapshots.consumer')}</label>
    <select ref={consumerSelect} id="oauth-snapshot-consumer" value={selected} disabled={busy} onChange={(event) => setSelected(event.target.value)}>
      <option value="">{t('mcp.snapshots.select')}</option>
      {eligible.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
    </select>
    <Button disabled={busy || snapshots === null || !selected} onClick={() => void run(() => CreateMCPOAuthSnapshot(selected), 'mcp.snapshots.created')}>{t('mcp.snapshots.create')}</Button>
    {failed && <p>{t('mcp.snapshots.failed')}</p>}
    {snapshots === null ? <p>{t('mcp.snapshots.loading')}</p> : snapshots.length === 0 ? <p>{t('mcp.snapshots.empty')}</p> : <ul>
      {snapshots.map((item) => <li key={item.id}>
        <h3>{item.name}</h3>
        <p>{t('mcp.snapshots.createdAt', { date: new Date(String(item.createdAt)).toLocaleString(i18n?.language) })}</p>
        <p>{t('mcp.snapshots.retainUntil', { date: new Date(String(item.retainUntil)).toLocaleString(i18n?.language) })}</p>
        <p>{t('mcp.snapshots.location', { path: item.location })}</p>
        {item.expired && <p>{t('mcp.snapshots.expired')}</p>}
        <Button disabled={busy || item.expired} aria-label={t('mcp.snapshots.restoreNamed', { name: item.name })} onClick={() => void restore(item.id)}>{t('mcp.snapshots.restore')}</Button>
        <Button disabled={busy} variant="danger" aria-label={t('mcp.snapshots.discardNamed', { name: item.name })} onClick={() => void discard(item.id)}>{t('mcp.snapshots.discard')}</Button>
      </li>)}
    </ul>}
  </section>;
}
