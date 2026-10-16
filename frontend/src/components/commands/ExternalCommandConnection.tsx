import { useCallback, useEffect, useId, useRef, useState, useSyncExternalStore } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { Button, Checkbox } from '../ui';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { parseExternalUIInvitation, type ExternalUIConnectionService, type ExternalUIInvitation } from '../../services/externalUIConnection';
import type { ExternalUIDestination } from '../../services/externalUIConnection';

const NO_CONNECTION = null;
const MAX_TIMER_DELAY = 2_147_483_647;

export interface ExternalCommandConnectionProps {
  /** Null somente enquanto o provider do App não estiver disponível. */
  readonly service: ExternalUIConnectionService | null;
  /** Destino capturado do contexto autorizado da UI; nunca sintetizado pelo formulário. */
  readonly target: ExternalUIDestination | null;
  /** Alvo opcional para renderizar apenas o checkbox de consentimento na toolbar. */
  readonly consentTarget?: HTMLElement | null;
}

export function ExternalCommandConnection({ service, target, consentTarget }: ExternalCommandConnectionProps) {
  const { t, i18n } = useTranslation();
  const { announce } = useAnnouncer();
  const consentDescriptionId = useId();
  const subscribe = useCallback((listener: () => void) => service?.subscribe(listener) ?? (() => undefined), [service]);
  const getSnapshot = useCallback(() => service?.getSnapshot() ?? NO_CONNECTION, [service]);
  const connection = useSyncExternalStore(subscribe, getSnapshot, () => NO_CONNECTION);
  const state = connection?.state ?? 'disconnected';
  const [consented, setConsented] = useState(false);
  const [invitation, setInvitation] = useState<ExternalUIInvitation | null>(null);
  const [invitationExpired, setInvitationExpired] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const previousStatus = useRef(state);

  useEffect(() => {
    if (state === 'connected') {
      setInvitation(null);
      setInvitationExpired(false);
    }
  }, [state]);

  // Se o Begin foi aceito mas a primeira leitura de status falhou, continue
  // consultando enquanto o convite estiver visível. Em waiting_claim o provider
  // assume o polling normal; este fallback para quando o estado muda.
  useEffect(() => {
    if (!service || !invitation || state !== 'disconnected') return;
    const timer = window.setInterval(() => { void service.refresh().catch(() => undefined); }, 10_000);
    return () => window.clearInterval(timer);
  }, [invitation, service, state]);

  useEffect(() => {
    if (!invitation) return;
    let timer: number | undefined;
    const expireOrRecheck = () => {
      const remaining = Date.parse(invitation.expiresAt) - Date.now();
      if (remaining <= 0) {
        setInvitation(null);
        setInvitationExpired(true);
        announce(t('commandSettings.externalConnection.expired'), 'assertive');
      } else {
        timer = window.setTimeout(expireOrRecheck, Math.min(remaining, MAX_TIMER_DELAY));
      }
    };
    expireOrRecheck();
    return () => { if (timer !== undefined) window.clearTimeout(timer); };
  }, [announce, invitation, t]);

  useEffect(() => {
    if (previousStatus.current !== state) {
      previousStatus.current = state;
      announce(t(`commandSettings.externalConnection.state.${state}`));
    }
  }, [announce, state, t]);

  const begin = async () => {
    if (!service || !target || !consented || busy || state !== 'disconnected') return;
    setBusy(true);
    setError('');
    setInvitationExpired(false);
    try {
      const value = parseExternalUIInvitation(await service.begin(target));
      if (!value || Date.parse(value.expiresAt) <= Date.now()) {
        throw new Error('external-ui-invitation-invalid');
      }
      setInvitation(Object.freeze({ invitation: value.invitation, expiresAt: value.expiresAt }));
      setConsented(false);
      announce(t('commandSettings.externalConnection.invitationCreated'));
    } catch {
      setError(t('commandSettings.externalConnection.actionFailed'));
      announce(t('commandSettings.externalConnection.actionFailed'), 'assertive');
    } finally {
      setBusy(false);
    }
  };

  const disconnect = async () => {
    if (!service || state !== 'connected' || busy) return;
    setBusy(true);
    setError('');
    try {
      await service.disconnect();
    } catch {
      setError(t('commandSettings.externalConnection.actionFailed'));
      announce(t('commandSettings.externalConnection.actionFailed'), 'assertive');
    } finally {
      setBusy(false);
    }
  };

  const expiresLabel = invitation
    ? new Intl.DateTimeFormat(i18n.language, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(invitation.expiresAt))
    : '';
  const consentCheckbox = (
    <Checkbox
      checked={consented}
      label={t('commandSettings.externalConnection.consent')}
      aria-describedby={consentDescriptionId}
      onChange={event => setConsented(event.currentTarget.checked)}
    />
  );

  return (
    <section className="command-settings__external-connection" aria-labelledby="external-command-connection-title">
      <h2 id="external-command-connection-title">{t('commandSettings.externalConnection.title')}</h2>
      <p id={consentDescriptionId}>{t('commandSettings.externalConnection.description')}</p>
      <p className="command-settings__info">
        {t(`commandSettings.externalConnection.state.${state}`)}
      </p>
      {invitation && (
        <div className="command-settings__external-invitation">
          <p>{t('commandSettings.externalConnection.invitationInstructions', { expiresAt: expiresLabel })}</p>
          <p>{t('commandSettings.externalConnection.invitationCannotCancel')}</p>
          <label htmlFor="external-command-invitation">{t('commandSettings.externalConnection.invitationLabel')}</label>
          <textarea id="external-command-invitation" readOnly rows={3} value={invitation.invitation} />
        </div>
      )}
      {invitationExpired && <p>{t('commandSettings.externalConnection.expired')}</p>}
      {!service && <p className="command-settings__info">{t('commandSettings.externalConnection.notReady')}</p>}
      {error && <p className="command-settings__error">{error}</p>}
      {state === 'disconnected' && (
        <>
          {consentTarget ? createPortal(consentCheckbox, consentTarget) : consentCheckbox}
          <Button
            type="button"
            variant="primary"
            disabled={!service || !target || !consented || busy || invitation !== null}
            loading={busy}
            onClick={() => void begin()}
          >
            {t('commandSettings.externalConnection.begin')}
          </Button>
        </>
      )}
      {state === 'connected' && (
        <Button type="button" variant="danger" disabled={!service || busy} loading={busy} onClick={() => void disconnect()}>
          {t('commandSettings.externalConnection.disconnect')}
        </Button>
      )}
    </section>
  );
}
