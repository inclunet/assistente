import React, { useState } from 'react';
import { CheckCircleOutlined, CloseCircleOutlined, DownOutlined, LoadingOutlined, SettingOutlined, ToolOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { isAppToolEvent, type ToolCallStatus, type ToolOrigin } from '../../types/chat';
import { formatDuration } from '../../utils/format';
import './ToolCallsSection.css';

/**
 * Representa uma tool call individual parseada do JSON.
 * O campo `result` é adicionado pela consolidação no MessageList.
 * Campos de metadata (AEP-0039 Fase 5) são opcionais para retrocompatibilidade.
 */
export interface ParsedToolCall {
  id: string;
  type: string;
  function: {
    name: string;
    arguments: string;
  };
  /** Resultado retornado pela ferramenta (adicionado pela consolidação) */
  result?: string;
  /** Origem da ferramenta (AEP-0039, AEP-0084) */
  origin?: ToolOrigin;
  /** Label do servidor MCP (AEP-0039) */
  server_label?: string;
  /** Iteração do agentic loop (0-based) (AEP-0039) */
  iteration?: number;
  /** Duração da execução em milissegundos (AEP-0039) */
  duration_ms?: number;
}

interface ToolCallsSectionProps {
  /** JSON string de tool_calls (do campo toolCalls da mensagem consolidada) */
  toolCallsJson?: string;
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
};

function originLabelKey(origin?: string): string {
  return ORIGIN_LABEL_KEYS[origin as ToolOrigin] ?? ORIGIN_LABEL_KEYS.builtin;
}

/** Limite de caracteres para exibir resultado truncado */
const RESULT_PREVIEW_LENGTH = 300;
const LARGE_TOOL_CALLS_JSON_LENGTH = 8_000;

function countTopLevelArrayItems(raw: string): number {
  const trimmed = raw.trim();
  if (!trimmed.startsWith('[') || !trimmed.endsWith(']')) return 0;
  let depth = 0;
  let count = 0;
  let inString = false;
  let escaped = false;
  for (const char of trimmed) {
    if (escaped) {
      escaped = false;
      continue;
    }
    if (char === '\\' && inString) {
      escaped = true;
      continue;
    }
    if (char === '"') {
      inString = !inString;
      continue;
    }
    if (inString) continue;
    if (char === '{') {
      if (depth === 1) count += 1;
      depth += 1;
      continue;
    }
    if (char === '}' && depth > 0) {
      depth -= 1;
      continue;
    }
    if (char === '[') depth += 1;
    if (char === ']' && depth > 0) depth -= 1;
  }
  return depth === 0 && !inString && !escaped ? count : 0;
}

/**
 * ToolCallsSection renderiza indicadores de ferramentas chamadas pelo assistente.
 * 
 * Dois modos de uso:
 * 1. **Streaming**: mostra `activeToolCalls` com status em tempo real (running/done/error)
 * 2. **Histórico**: parseia `toolCallsJson` para exibir chamadas + resultados
 */
export const ToolCallsSection = React.memo<ToolCallsSectionProps>(function ToolCallsSection({
  toolCallsJson,
  activeToolCalls,
  tabNavigationEnabled = false,
}) {
  const { t } = useTranslation();
  const [isExpanded, setIsExpanded] = useState(false);
  const [expandedResults, setExpandedResults] = useState<Set<string>>(new Set());
  const shouldDeferSavedParsing = !!toolCallsJson && toolCallsJson.length > LARGE_TOOL_CALLS_JSON_LENGTH && !isExpanded;
  const deferredToolCount = shouldDeferSavedParsing && toolCallsJson
    ? countTopLevelArrayItems(toolCallsJson)
    : 0;

  // Parseia tool calls do JSON (modo histórico)
  let parsedCalls: ParsedToolCall[] = [];
  if (toolCallsJson && !shouldDeferSavedParsing) {
    try {
      parsedCalls = JSON.parse(toolCallsJson);
    } catch {
      // JSON inválido — ignora
    }
  }

  // Determina quais calls mostrar
  const hasActiveCalls = activeToolCalls && activeToolCalls.length > 0;
  const hasSavedCalls = parsedCalls.length > 0 || deferredToolCount > 0;

  if (!hasActiveCalls && !hasSavedCalls) return null;

  const toolCount = hasActiveCalls ? activeToolCalls!.length : Math.max(parsedCalls.length, deferredToolCount);
  const isRunning = hasActiveCalls && activeToolCalls!.some(tc => tc.status === 'running');

  const handleToggle = () => setIsExpanded(!isExpanded);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      handleToggle();
    }
  };

  const toggleResultExpanded = (callId: string) => {
    setExpandedResults(prev => {
      const next = new Set(prev);
      if (next.has(callId)) {
        next.delete(callId);
      } else {
        next.add(callId);
      }
      return next;
    });
  };

  // Nomes das tools para exibição rápida
  const toolNames = hasActiveCalls
    ? activeToolCalls!.map(tc => tc.name)
    : shouldDeferSavedParsing ? [t('chat.toolDetails')] : parsedCalls.map(tc => tc.function.name);

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
        onKeyDown={handleKeyDown}
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
            // Modo histórico: mostra chamadas + resultados
            <ul className="tool-calls-section__list">
              {parsedCalls.map((tc) => {
                const isResultExpanded = expandedResults.has(tc.id);
                const hasResult = !!tc.result;
                const isLongResult = hasResult && tc.result!.length > RESULT_PREVIEW_LENGTH;

                return (
                  <li key={tc.id} className="tool-calls-section__item tool-calls-section__item--done">
                    <div className="tool-calls-section__item-header">
                      <span className="tool-calls-section__status-icon" aria-hidden="true"><CheckCircleOutlined /></span>
                      <span className="tool-calls-section__name">{tc.function.name}</span>
                      {tc.origin && (
                        <span className={`tool-calls-section__origin-badge tool-calls-section__origin-badge--${tc.origin}`}>
                          {t(originLabelKey(tc.origin))}
                        </span>
                      )}
                      {tc.server_label && (
                        <span className="tool-calls-section__server-label">{tc.server_label}</span>
                      )}
                      {tc.duration_ms != null && tc.duration_ms > 0 && (
                        <span className="tool-calls-section__duration">{formatDuration(tc.duration_ms)}</span>
                      )}
                    </div>

                    {/* Parâmetros da chamada */}
                    {tc.function.arguments && (
                      <div className="tool-calls-section__section">
                        <h4 className="tool-calls-section__section-heading">{t('chat.parameters')}</h4>
                        <pre className="tool-calls-section__args">{formatArgs(tc.function.arguments)}</pre>
                      </div>
                    )}

                    {/* Resultado retornado pela ferramenta */}
                    {hasResult && (
                      <div className="tool-calls-section__section">
                        <h4 className="tool-calls-section__section-heading">{t('chat.response')}</h4>
                        <pre className="tool-calls-section__result-content">
                          {isLongResult && !isResultExpanded
                            ? normalizeResult(tc.result!.slice(0, RESULT_PREVIEW_LENGTH)) + '…'
                            : normalizeResult(tc.result!)}
                        </pre>
                        {isLongResult && (
                          <button
                            className="tool-calls-section__result-toggle"
                            onClick={() => toggleResultExpanded(tc.id)}
                            type="button"
                            tabIndex={tabNavigationEnabled ? 0 : -1}
                          >
                            {isResultExpanded ? t('chat.showLess') : `${t('chat.showAll')} (${formatSize(tc.result!.length)})`}
                          </button>
                        )}
                      </div>
                    )}
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

/**
 * Formata tamanho em bytes/KB para exibição
 */
function formatSize(chars: number): string {
  if (chars < 1024) return `${chars} chars`;
  return `${(chars / 1024).toFixed(1)} KB`;
}
