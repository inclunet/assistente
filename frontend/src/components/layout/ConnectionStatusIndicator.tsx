import { useTranslation } from 'react-i18next';
import { CheckCircleOutlined, CloseCircleOutlined, LoadingOutlined } from '@ant-design/icons';
import { useConnectionStore } from '../../store/connectionStore';
import type { ConnectionState } from '../../types/connection';
import './ConnectionStatusIndicator.css';

const STATE_ICON: Record<Exclude<ConnectionState, 'unknown'>, React.ReactNode> = {
  online: <CheckCircleOutlined aria-hidden="true" />,
  offline: <CloseCircleOutlined aria-hidden="true" />,
  checking: <LoadingOutlined spin aria-hidden="true" />,
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

  const label =
    state === 'online'
      ? t('connectionStatus.online')
      : state === 'offline'
        ? t('connectionStatus.offline')
        : t('connectionStatus.checking');

  const ariaLabel =
    state === 'online'
      ? showLatency
        ? t('connectionStatus.aria.onlineLatency', { provider: providerName, latency: avgLatency })
        : t('connectionStatus.aria.online', { provider: providerName })
      : state === 'offline'
        ? t('connectionStatus.aria.offline', { provider: providerName })
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
