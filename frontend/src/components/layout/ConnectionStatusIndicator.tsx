import { useTranslation } from 'react-i18next';
import {
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  LoadingOutlined,
} from '@ant-design/icons';
import { useConnectionStore } from '../../store/connectionStore';
import type { ConnectionState } from '../../types/connection';
import './ConnectionStatusIndicator.css';

const STATE_ICON: Record<Exclude<ConnectionState, 'unknown'>, React.ReactNode> = {
  online: <CheckCircleOutlined aria-hidden="true" />,
  offline: <CloseCircleOutlined aria-hidden="true" />,
  unauthenticated: <ExclamationCircleOutlined aria-hidden="true" />,
  checking: <LoadingOutlined spin aria-hidden="true" />,
};

const STATE_LABEL: Record<Exclude<ConnectionState, 'unknown'>, string> = {
  online: 'connectionStatus.online',
  offline: 'connectionStatus.offline',
  unauthenticated: 'connectionStatus.unauthenticated',
  checking: 'connectionStatus.checking',
};

/**
 * Indicador de status de conexão com a API LLM exibido na topbar (Issue #38).
 *
 * Acessibilidade: o estado é transmitido por ÍCONE + TEXTO + `aria-label`
 * (nunca só por cor). A cor é apenas reforço visual via tokens semânticos.
 * Não é uma live region — anúncios de mudança vão pelo announcer global
 * (useConnectionStatusListener).
 */
export function ConnectionStatusIndicator() {
  const { t } = useTranslation();
  const status = useConnectionStore((s) => s.status);
  const state = useConnectionStore((s) => s.state);

  // Antes da primeira verificação não há nada útil para mostrar.
  if (state === 'unknown' || !status) {
    return null;
  }

  const providerName = status.providerName || t('connectionStatus.provider');
  const avgLatency = Math.max(0, Math.round(status.avgLatencyMs));
  const showLatency = state === 'online' && avgLatency > 0;

  const label = t(STATE_LABEL[state]);

  // O rótulo de "sem login" diz o que fazer, e não só que algo está errado: é o
  // estado que a pessoa resolve fora do app, rodando o login do CLI do agente
  // (AEP-0084 D12).
  const ariaLabel =
    state === 'online'
      ? showLatency
        ? t('connectionStatus.aria.onlineLatency', { provider: providerName, latency: avgLatency })
        : t('connectionStatus.aria.online', { provider: providerName })
      : state === 'offline'
        ? t('connectionStatus.aria.offline', { provider: providerName })
        : state === 'unauthenticated'
          ? t('connectionStatus.aria.unauthenticated', { provider: providerName })
          : t('connectionStatus.aria.checking', { provider: providerName });

  return (
    <div
      className="connection-status"
      data-state={state}
      aria-label={ariaLabel}
      // Tooltip localizado: reutiliza o ariaLabel já traduzido. Evita vazar o
      // status.error cru do backend (texto técnico não localizado) na UI.
      title={ariaLabel}
    >
      <span className="connection-status__icon">{STATE_ICON[state]}</span>
      <span className="connection-status__label">{label}</span>
      {showLatency && (
        <span className="connection-status__latency">{t('connectionStatus.latency', { latency: avgLatency })}</span>
      )}
    </div>
  );
}
