import React, { useState } from 'react';
import { CheckCircleOutlined, CloseCircleOutlined, DownOutlined, LoadingOutlined, SettingOutlined, ToolOutlined } from '@ant-design/icons';
import type { toolinvocations } from '@wailsjs/go/models';
import { useTranslation } from 'react-i18next';
import type { ToolInvocationSummary } from '../../lib/chatMessageTree';
import { announce } from '../../hooks/useAnnouncer';
import { loadToolInvocationDetails } from '../../services/toolInvocationDetailsCache';
import { useAuthStore } from '../../store/authStore';
import { isAppToolEvent, type ToolCallStatus, type ToolOrigin } from '../../types/chat';
import { formatDuration } from '../../utils/format';
import { Button } from '../ui/Button';
import './ToolCallsSection.css';

interface ToolCallsSectionProps {
  /** Projeção leve persistida pelo ledger canônico. */
  toolInvocations?: ToolInvocationSummary[];
  /** Tool calls ativos durante streaming (do store) */
  activeToolCalls?: ToolCallStatus[];
  /** Controles internos só entram na ordem de Tab no modo de leitura. */
  tabNavigationEnabled?: boolean;
}

const ORIGIN_LABEL_KEYS: Record<ToolOrigin, string> = {
  builtin: 'chat.toolOriginBuiltin',
  mcp_bridge: 'chat.toolOriginMcpBridge',
  mcp_native: 'chat.toolOriginMcpNative',
  acp_agent: 'chat.toolOriginAcpAgent',
  archival: 'chat.toolOriginArchival',
};

function originLabelKey(origin?: string): string {
  return ORIGIN_LABEL_KEYS[origin as ToolOrigin] ?? ORIGIN_LABEL_KEYS.builtin;
}

/**
 * ToolCallsSection renderiza indicadores de ferramentas chamadas pelo assistente.
 * 
 * Dois modos de uso:
 * 1. **Streaming**: mostra `activeToolCalls` com status em tempo real (running/done/error)
 * 2. **Histórico**: usa a projeção canônica `toolInvocations`
 */
export const ToolCallsSection = React.memo<ToolCallsSectionProps>(function ToolCallsSection({
  toolInvocations,
  activeToolCalls,
  tabNavigationEnabled = false,
}) {
  const { t } = useTranslation();
  const userId = useAuthStore((state) => state.user?.userId ?? '');
  const [isExpanded, setIsExpanded] = useState(false);
  const [expandedResults, setExpandedResults] = useState<Set<string>>(new Set());
  const [loadedDetails, setLoadedDetails] = useState<Record<string, toolinvocations.Detail>>({});
  const [loadingDetails, setLoadingDetails] = useState<Set<string>>(new Set());
  const [detailErrors, setDetailErrors] = useState<Set<string>>(new Set());

  // Determina quais calls mostrar
  const hasActiveCalls = activeToolCalls && activeToolCalls.length > 0;
  const hasInvocationSummaries = !!toolInvocations?.length;

  if (!hasActiveCalls && !hasInvocationSummaries) return null;

  const toolCount = hasActiveCalls
    ? activeToolCalls!.length
    : toolInvocations!.length;
  const isRunning = hasActiveCalls && activeToolCalls!.some(tc => tc.status === 'running');

  const handleToggle = () => setIsExpanded(!isExpanded);

  const loadDetails = async (invocation: ToolInvocationSummary) => {
    if (!invocation.invocationId || !invocation.hasDetails || loadingDetails.has(invocation.invocationId)) return;
    const invocationId = invocation.invocationId;
    if (loadedDetails[invocationId]) {
      setExpandedResults((previous) => {
        const next = new Set(previous);
        if (next.has(invocationId)) next.delete(invocationId);
        else next.add(invocationId);
        return next;
      });
      return;
    }
    setLoadingDetails((previous) => new Set(previous).add(invocationId));
    setDetailErrors((previous) => {
      const next = new Set(previous);
      next.delete(invocationId);
      return next;
    });
    try {
      const details = await loadToolInvocationDetails(userId, [invocationId]);
      const detail = details.get(invocationId);
      if (!detail) throw new Error('detail unavailable');
      setLoadedDetails((previous) => ({ ...previous, [invocationId]: detail }));
      setExpandedResults((previous) => new Set(previous).add(invocationId));
    } catch {
      setDetailErrors((previous) => new Set(previous).add(invocationId));
      announce(t('chat.toolDetailsLoadError'), 'assertive');
    } finally {
      setLoadingDetails((previous) => {
        const next = new Set(previous);
        next.delete(invocationId);
        return next;
      });
    }
  };

  // Nomes das tools para exibição rápida
  const toolNames = hasActiveCalls
    ? activeToolCalls!.map(tc => tc.name)
    : toolInvocations!.map((invocation) => invocation.name);

  const uniqueNames = [...new Set(toolNames)];
  const summaryText = isRunning
    ? `${t('chat.executing')} ${toolCount} ${t('chat.toolsRunning')}`
    : `${toolCount} ${t('chat.toolsUsed')}`;

  return (
    <div
      className={`tool-calls-section ${isExpanded ? 'tool-calls-section--expanded' : ''} ${isRunning ? 'tool-calls-section--running' : ''}`}
    >
      <button
        className="tool-calls-section__header"
        onClick={handleToggle}
        aria-expanded={isExpanded}
        type="button"
        tabIndex={tabNavigationEnabled ? 0 : -1}
      >
        <span className="tool-calls-section__icon" aria-hidden="true">
          {isRunning ? <SettingOutlined spin /> : <ToolOutlined />}
        </span>
        <span className="tool-calls-section__title">
          {uniqueNames.join(', ')}
        </span>
        <span className="tool-calls-section__summary">
          {summaryText}
        </span>
        <span
          className={`tool-calls-section__chevron ${isExpanded ? 'tool-calls-section__chevron--expanded' : ''}`}
          aria-hidden="true"
        >
          <DownOutlined />
        </span>
      </button>

      {isExpanded && (
        <div className="tool-calls-section__content" role="region" aria-label={t('chat.toolDetails')}>
          {hasActiveCalls ? (
            // Modo streaming: mostra status em tempo real
            <ul className="tool-calls-section__list">
              {activeToolCalls!.map((tc) => (
                <li key={tc.callId} className={`tool-calls-section__item tool-calls-section__item--${tc.status}`}>
                  <div className="tool-calls-section__item-header">
                    <span className="tool-calls-section__status-icon" aria-hidden="true">
                      {tc.status === 'running' ? <LoadingOutlined spin /> : tc.status === 'done' ? <CheckCircleOutlined /> : <CloseCircleOutlined />}
                    </span>
                    <span className="tool-calls-section__name">{tc.name}</span>
                    {/* Ferramenta de agente externo é marcada enquanto roda: quem
                        acompanha precisa saber que o app não é o autor (AEP-0084 D7). */}
                    {!isAppToolEvent(tc.origin) && (
                      <span className={`tool-calls-section__origin-badge tool-calls-section__origin-badge--${tc.origin}`}>
                        {t(originLabelKey(tc.origin))}
                      </span>
                    )}
                    {tc.summary && (
                      <span className="tool-calls-section__result-summary">{tc.summary}</span>
                    )}
                  </div>
                  {tc.args && (
                    <div className="tool-calls-section__section">
                      <h4 className="tool-calls-section__section-heading">{t('chat.parameters')}</h4>
                      <pre className="tool-calls-section__args">{formatArgs(tc.args)}</pre>
                    </div>
                  )}
                </li>
              ))}
            </ul>
          ) : (
            <ul className="tool-calls-section__list">
              {toolInvocations!.map((invocation) => {
                const key = invocation.invocationId || invocation.callId;
                const detail = invocation.invocationId ? loadedDetails[invocation.invocationId] : undefined;
                const isDetailExpanded = !!invocation.invocationId && expandedResults.has(invocation.invocationId);
                const isLoading = !!invocation.invocationId && loadingDetails.has(invocation.invocationId);
                const hasError = !!invocation.invocationId && detailErrors.has(invocation.invocationId);
                const visibleDetail = isDetailExpanded ? detail : undefined;
                const argumentsText = visibleDetail
                  ? detailArguments(visibleDetail.input ?? '', visibleDetail.metadata ?? '')
                  : invocation.inputPreview;
                const resultText = visibleDetail ? detailResult(visibleDetail.output ?? '') : invocation.outputPreview;
                return (
                  <li key={key} className={`tool-calls-section__item tool-calls-section__item--${invocation.status}`}>
                    <div className="tool-calls-section__item-header">
                      <span className="tool-calls-section__status-icon" aria-hidden="true">
                        {invocation.status === 'running' ? <LoadingOutlined spin /> : invocation.status === 'failed' ? <CloseCircleOutlined /> : <CheckCircleOutlined />}
                      </span>
                      <span className="tool-calls-section__name">{invocation.name}</span>
                      {invocation.origin && (
                        <span className={`tool-calls-section__origin-badge tool-calls-section__origin-badge--${invocation.origin}`}>
                          {t(originLabelKey(invocation.origin))}
                        </span>
                      )}
                      {invocation.serverLabel && <span className="tool-calls-section__server-label">{invocation.serverLabel}</span>}
                      {!!invocation.durationMs && (
                        <span className="tool-calls-section__duration">{formatDuration(invocation.durationMs)}</span>
                      )}
                    </div>
                    {argumentsText && (
                      <div className="tool-calls-section__section">
                        <h4 className="tool-calls-section__section-heading">{t('chat.parameters')}</h4>
                        <pre className="tool-calls-section__args">{formatArgs(argumentsText)}</pre>
                      </div>
                    )}
                    {resultText && (
                      <div className="tool-calls-section__section">
                        <h4 className="tool-calls-section__section-heading">{t('chat.response')}</h4>
                        <pre className="tool-calls-section__result-content">{normalizeResult(resultText)}</pre>
                      </div>
                    )}
                    {invocation.hasDetails && invocation.invocationId && (
                      <Button
                        className="tool-calls-section__result-toggle"
                        onClick={() => void loadDetails(invocation)}
                        type="button"
                        variant="ghost"
                        size="sm"
                        aria-expanded={isDetailExpanded}
                        tabIndex={tabNavigationEnabled ? 0 : -1}
                        loading={isLoading}
                      >
                        {isLoading ? t('chat.loadingToolDetails') : isDetailExpanded ? t('chat.showLess') : t('chat.showAll')}
                      </Button>
                    )}
                    {!invocation.hasDetails && invocation.resultAvailability !== 'available' && (
                      <p className="tool-calls-section__result-summary">{t('chat.toolDetailsUnavailable')}</p>
                    )}
                    {hasError && <p className="tool-calls-section__result-summary">{t('chat.toolDetailsLoadError')}</p>}
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      )}
    </div>
  );
});

/**
 * Formata string JSON de argumentos para exibição legível.
 * Converte tabs em espaços e re-indenta com 2 espaços.
 */
function formatArgs(raw: string): string {
  try {
    const parsed = JSON.parse(raw);
    return JSON.stringify(parsed, null, 2);
  } catch {
    // Se não é JSON válido, apenas substitui tabs por 2 espaços
    return raw.replace(/\t/g, '  ');
  }
}

/**
 * Normaliza tabs em conteúdo de resultado de ferramenta.
 */
function normalizeResult(raw: string): string {
  return raw.replace(/\t/g, '  ');
}

function detailArguments(input: string, metadata: string): string {
  try {
    const parsed = JSON.parse(metadata) as { display?: { arguments?: unknown } };
    if (typeof parsed.display?.arguments === 'string') return parsed.display.arguments;
  } catch {
    // Metadata histórica pode não ser JSON; o input integral continua disponível.
  }
  return input;
}

function detailResult(output: string): string {
  try {
    const parsed = JSON.parse(output) as { content?: unknown };
    if (typeof parsed.content === 'string') return parsed.content;
  } catch {
    // Resultados históricos podem ser texto simples.
  }
  return output;
}
