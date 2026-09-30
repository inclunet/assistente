import { useAuthStore } from '../../store/authStore';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AuthorizeChatGPT, CancelChatGPT, ChatGPTConnection as GetConnection, CreateChatGPTConnection, DisconnectChatGPT } from '@wailsjs/go/wailsapi/LLMProviders';
import { Input } from '../ui/Input';
import { Button } from '../ui/Button';
import { DialogActions } from '../ui/DialogActions';
import { useAnnouncer } from '../../hooks/useAnnouncer';

interface Props { id?: string; onChanged: () => void; onClose: () => void; onCloseBlockedChange?: (closeBlocked: boolean) => void }
export function ChatGPTConnection({ id, onChanged, onClose, onCloseBlockedChange }: Props) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const localUserID = useAuthStore(s => s.user?.userId);
  const [welcome, setWelcome] = useState(false);
  const currentID = useRef(id || '');
  const connectButton = useRef<HTMLButtonElement>(null);
  const restoreFocus = useRef(false);
  useEffect(() => {
    if (!welcome && restoreFocus.current) {
      connectButton.current?.focus();
      restoreFocus.current = false;
    }
  }, [welcome]);
  const mounted = useRef(true);
  const loadGeneration = useRef(0);
  const [name, setName] = useState('');
  const [closeBlocked, setCloseBlocked] = useState(false);
  const [busy, setBusy] = useState(false);
  const [state, setState] = useState('pending');
  const [email, setEmail] = useState('');
  const [error, setError] = useState('');
  useEffect(() => {
    mounted.current = true;
    const generation = ++loadGeneration.current;
    if (id) void GetConnection(id).then(value => {
      if (mounted.current && generation === loadGeneration.current) { setState(value.state); setEmail(value.email || ''); announce(t(`chatgpt.states.${value.state}`, { defaultValue: t('chatgpt.connectionError') })); }
    }).catch(() => { if (mounted.current && generation === loadGeneration.current) { setError(t('chatgpt.connectionError')); announce(t('chatgpt.connectionError'), 'assertive'); } });
    return () => { loadGeneration.current++; mounted.current = false; if (currentID.current) void CancelChatGPT(currentID.current).catch(() => undefined); };
  }, [id, t, announce]);
  const connect = async () => {
    loadGeneration.current++;
    setBusy(true); setError('');
    try {
      if (!currentID.current) {
        setCloseBlocked(true); onCloseBlockedChange?.(true); announce(t('chatgpt.operationPending'));
        try {
          const created = await CreateChatGPTConnection(name);
          currentID.current = created.id;
        } finally {
          if (mounted.current) { setCloseBlocked(false); onCloseBlockedChange?.(false); }
        }
        if (!mounted.current) { onChanged(); return; }
      }
      announce(t('chatgpt.waiting'));
      const result = await AuthorizeChatGPT(currentID.current, t('chatgpt.browserReturn'));
      if (!mounted.current) return;
      setState(result.state); setEmail(result.email || '');
      if (result.state === 'connected' && localUserID) {
        const key = `chatgpt-plan-notice:${localUserID}`;
        try { if (!localStorage.getItem(key)) { setWelcome(true); localStorage.setItem(key, 'seen'); } } catch { setWelcome(true); }
      }
      announce(t(`chatgpt.states.${result.state}`, { defaultValue: t('chatgpt.connectionError') }));
    } catch {
      if (mounted.current) { setError(t('chatgpt.connectionError')); announce(t('chatgpt.connectionError')); }
    } finally { if (mounted.current) setBusy(false); }
  };
  const disconnect = async () => {
    loadGeneration.current++;
    setBusy(true); setError('');
    setCloseBlocked(true); onCloseBlockedChange?.(true); announce(t('chatgpt.operationPending'));
    try {
      const revoked = await DisconnectChatGPT(currentID.current);
      if (!mounted.current) return;
      setState('disconnected'); setEmail('');
      const message = t(revoked ? 'chatgpt.states.disconnected' : 'chatgpt.revocationUnconfirmed');
      setError(revoked ? '' : message); announce(message);
    } catch (failure: unknown) {
      if (mounted.current) {
        const code = failure instanceof Error ? failure.message : String(failure);
        const key = code === 'oauth_vault_persistence_required' || code === 'oauth_vault_unavailable'
          ? 'chatgpt.vaultUnavailable' : 'chatgpt.connectionError';
        const message = t(key);
        setError(message); announce(message, 'assertive');
      }
    }
    finally {
      if (mounted.current) { setBusy(false); setCloseBlocked(false); onCloseBlockedChange?.(false); onChanged(); }
    }
  };
  const close = () => {
    if (closeBlocked) return;
    if (currentID.current) void CancelChatGPT(currentID.current).catch(() => undefined);
    onClose(); onChanged();
  };
  if (welcome) return <div>
    <h3>{t('chatgpt.usingPlan')}</h3><p>{t('chatgpt.planNotice')}</p>
    <p><a href="https://chatgpt.com/settings/usage" target="_blank" rel="noopener noreferrer">{t('chatgpt.usage')}</a></p>
    <DialogActions primary={<Button variant="primary" autoFocus onClick={() => { restoreFocus.current = true; setWelcome(false); }}>{t('chatgpt.gotIt')}</Button>} />
  </div>;
  return <div>
    <p>{t('chatgpt.planNotice')}</p>
    <p>{t('chatgpt.limitations')}</p>
    {!id && <Input label={t('chatgpt.label')} value={name} maxLength={100} disabled={busy || !!currentID.current} onChange={e => setName(e.target.value)} />}
    <p>{t('chatgpt.status', { state: t(`chatgpt.states.${state}`, { defaultValue: state }) })}</p>
    {email && <p>{t('chatgpt.account', { email })}</p>}
    {id && <p>{t('chatgpt.providerID', { id })}</p>}
    {busy && <p>{t(closeBlocked ? 'chatgpt.operationPending' : 'chatgpt.waiting')}</p>}
    {error && <p>{error}</p>}
    <p><a href="https://chatgpt.com/settings/usage" target="_blank" rel="noopener noreferrer">{t('chatgpt.usage')}</a></p>
    <DialogActions primary={
      <Button ref={connectButton} variant="primary" disabled={busy || (!currentID.current && !name.trim())} onClick={() => void connect()}>{t(state === 'connected' ? 'chatgpt.reconnect' : 'chatgpt.connect')}</Button>} secondary={<>
      {currentID.current && <Button disabled={busy || state === 'disconnected'} onClick={() => void disconnect()}>{t('chatgpt.disconnect')}</Button>}
      <Button disabled={closeBlocked} onClick={close}>{t(busy ? 'common.cancel' : 'common.close')}</Button>
    </>} />
  </div>;
}
