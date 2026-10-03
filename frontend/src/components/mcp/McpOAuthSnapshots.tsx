import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CreateMCPOAuthSnapshot, ListMCPOAuthSnapshots, RestoreMCPOAuthSnapshot, DiscardMCPOAuthSnapshot, ConvertMCPOAuthClientSnapshot, ReconnectMCPOAuthSnapshot } from '@wailsjs/go/wailsapi/MCP';
import type { credentials, mcp } from '../../../wailsjs/go/models';
import { Button } from '../ui/Button';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useConfirm } from '../../hooks/useConfirm';
import { mcpOAuthErrorKey } from '../../lib/mcpOAuthErrors';
import './McpOAuthSnapshots.css';

export function McpOAuthSnapshots({ consumers, onConverted }: { consumers: mcp.OAuthInventoryItem[]; onConverted?: () => Promise<void> }) {
  const { t, i18n } = useTranslation();
  const { announce } = useAnnouncer();
  const confirm = useConfirm();
  const [snapshots, setSnapshots] = useState<credentials.OAuthSnapshotInfo[] | null>(null);
  const [selected, setSelected] = useState('');
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState('');
  const [methods, setMethods] = useState<Record<string, string>>({});
  const [converted, setConverted] = useState<string[]>([]);
  const active = useRef(false);
  const consumerSelect = useRef<HTMLSelectElement>(null);
  const restoreFocus = useRef(false);
  const query = useRef<Promise<credentials.OAuthSnapshotInfo[]> | null>(null);
  const eligible = consumers.filter((item) => item.kind === 'legacy' || item.kind === 'client_credentials' || (item.kind === 'hostname' && !item.issues.some((issue) => issue === 'external_source' || issue === 'snapshot_ineligible')));

  useEffect(() => {
    active.current = true;
    let current = true;
    if (!query.current) query.current = Promise.resolve().then(() => ListMCPOAuthSnapshots());
    void query.current.then((items) => {
      if (current) setSnapshots(items || []);
    }).catch(() => {
      if (current) { setFailed('mcp.snapshots.failed'); announce(t('mcp.snapshots.failed'), 'assertive'); }
    });
    return () => { current = false; active.current = false; };
  }, [announce, t]);

  useEffect(() => {
    if (!busy && restoreFocus.current) {
      restoreFocus.current = false;
      consumerSelect.current?.focus();
    }
  }, [busy]);

  async function run(action: () => Promise<unknown>, success: string, reload = true) {
    if (!active.current) return;
    setBusy(true);
    setFailed('');
    try {
      await action();
      if (!active.current) return;
      if (reload) {
        const items = await ListMCPOAuthSnapshots();
        if (active.current) { query.current = Promise.resolve(items); setSnapshots(items); }
      }
      if (active.current) announce(t(success));
    } catch (error) {
      const key = mcpOAuthErrorKey(error) ?? 'mcp.snapshots.failed';
      if (active.current) { setFailed(key); announce(t(key), 'assertive'); }
    } finally {
      if (active.current) setBusy(false);
    }
  }

  async function restore(item: credentials.OAuthSnapshotInfo) {
    const hostname = item.consumerId.startsWith('credential:');
    const accepted = await confirm({ title: t('mcp.snapshots.restore'), message: t(hostname ? 'mcp.snapshots.restoreHostnameConfirm' : 'mcp.snapshots.restoreConfirm'), confirmText: t('mcp.snapshots.restore'), cancelText: t('common.cancel'), variant: 'warning' });
    if (accepted && active.current) await run(() => RestoreMCPOAuthSnapshot(item.id), hostname ? 'mcp.snapshots.hostnameRestored' : 'mcp.snapshots.restored');
  }
  async function discard(id: string) {
    const accepted = await confirm({ title: t('mcp.snapshots.discard'), message: t('mcp.snapshots.discardConfirm'), confirmText: t('mcp.snapshots.discard'), cancelText: t('common.cancel'), variant: 'danger' });
    if (accepted && active.current) {
      restoreFocus.current = true;
      await run(() => DiscardMCPOAuthSnapshot(id, true), 'mcp.snapshots.discarded');
    }
  }

  async function convert(item: credentials.OAuthSnapshotInfo, reconnect = false) {
    const method = methods[item.id];
    if (!method) return;
    const actionKey = reconnect ? 'mcp.snapshots.reconnect' : 'mcp.snapshots.convert';
    const accepted = await confirm({ title: t(actionKey), message: t(reconnect ? 'mcp.snapshots.reconnectConfirm' : 'mcp.snapshots.convertConfirm'), confirmText: t(actionKey), cancelText: t('common.cancel'), variant: 'warning' });
    if (accepted && active.current) {
      restoreFocus.current = true;
      await run(async () => {
        if (reconnect) await ReconnectMCPOAuthSnapshot(item.id, method);
        else await ConvertMCPOAuthClientSnapshot(item.id, method);
        if (active.current) { setConverted((ids) => [...ids, item.consumerId]); setSelected(''); }
        try { await onConverted?.(); } catch {
          if (active.current) announce(t('mcp.inventory.failed'), 'assertive');
        }
      }, reconnect ? 'mcp.snapshots.reconnected' : 'mcp.snapshots.converted', false);
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
    {failed && <p>{t(failed)}</p>}
    {snapshots === null ? <p>{t('mcp.snapshots.loading')}</p> : snapshots.length === 0 ? <p>{t('mcp.snapshots.empty')}</p> : <ul>
      {snapshots.map((item) => <li key={item.id}>
        <h3>{item.name}</h3>
        <p>{t('mcp.snapshots.createdAt', { date: new Date(String(item.createdAt)).toLocaleString(i18n?.language) })}</p>
        <p>{t('mcp.snapshots.retainUntil', { date: new Date(String(item.retainUntil)).toLocaleString(i18n?.language) })}</p>
        <p>{t('mcp.snapshots.location', { path: item.location })}</p>
        {item.expired && <p>{t('mcp.snapshots.expired')}</p>}
        {!converted.includes(item.consumerId) && consumers.some((consumer) => consumer.id === item.consumerId && consumer.kind === 'legacy') && <>
          <p>{t('mcp.snapshots.reconnectHelp')}</p>
          <label htmlFor={`oauth-reconnect-method-${item.id}`}>{t('mcp.connection.tokenAuthMethod')}</label>
          <select id={`oauth-reconnect-method-${item.id}`} value={methods[item.id] || ''} disabled={busy || item.expired} onChange={(event) => setMethods((previous) => ({ ...previous, [item.id]: event.target.value }))}>
            <option value="">{t('mcp.snapshots.select')}</option>
            <option value="none">{t('mcp.snapshots.publicClient')}</option>
            <option value="client_secret_basic">{t('mcp.connection.tokenAuthBasic')}</option>
            <option value="client_secret_post">{t('mcp.connection.tokenAuthPost')}</option>
          </select>
          <Button disabled={busy || item.expired || !methods[item.id]} aria-label={t('mcp.snapshots.reconnectNamed', { name: item.name })} onClick={() => void convert(item, true)}>{t('mcp.snapshots.reconnect')}</Button>
        </>}
        {!converted.includes(item.consumerId) && consumers.some((consumer) => consumer.id === item.consumerId && consumer.kind === 'client_credentials') && <>
          <p>{t('mcp.snapshots.convertHelp')}</p>
          <label htmlFor={`oauth-convert-method-${item.id}`}>{t('mcp.connection.tokenAuthMethod')}</label>
          <select id={`oauth-convert-method-${item.id}`} value={methods[item.id] || ''} disabled={busy || item.expired} onChange={(event) => setMethods((previous) => ({ ...previous, [item.id]: event.target.value }))}>
            <option value="">{t('mcp.snapshots.select')}</option>
            <option value="client_secret_basic">{t('mcp.connection.tokenAuthBasic')}</option>
            <option value="client_secret_post">{t('mcp.connection.tokenAuthPost')}</option>
          </select>
          <Button disabled={busy || item.expired || !methods[item.id]} aria-label={t('mcp.snapshots.convertNamed', { name: item.name })} onClick={() => void convert(item)}>{t('mcp.snapshots.convert')}</Button>
        </>}
        <Button disabled={busy || item.expired} aria-label={t('mcp.snapshots.restoreNamed', { name: item.name })} onClick={() => void restore(item)}>{t('mcp.snapshots.restore')}</Button>
        <Button disabled={busy} variant="danger" aria-label={t('mcp.snapshots.discardNamed', { name: item.name })} onClick={() => void discard(item.id)}>{t('mcp.snapshots.discard')}</Button>
      </li>)}
    </ul>}
  </section>;
}
