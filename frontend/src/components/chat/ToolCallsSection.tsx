import React, { useMemo, useState } from 'react';
import { CheckCircleOutlined, CloseCircleOutlined, DownOutlined, LoadingOutlined, ToolOutlined } from '@ant-design/icons';
import type { toolinvocations } from '@wailsjs/go/models';
import { BrowserOpenURL } from '@wailsjs/runtime/runtime';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import type { ToolInvocationSummary } from '../../lib/chatMessageTree';
import { executeDeepLink } from '../../lib/deepLinks';
import { presentTool, type ToolPresentation } from '../../lib/toolPresentation';
import { announce } from '../../hooks/useAnnouncer';
import { loadToolInvocationDetails } from '../../services/toolInvocationDetailsCache';
import { useAuthStore } from '../../store/authStore';
import { type ToolCallStatus } from '../../types/chat';
import { formatDuration } from '../../utils/format';
import { Button } from '../ui/Button';
import { Modal } from '../ui/Modal';
import './ToolCallsSection.css';

interface ToolCallsSectionProps {
  toolInvocations?: ToolInvocationSummary[];
  activeToolCalls?: ToolCallStatus[];
  tabNavigationEnabled?: boolean;
}

type InvocationForDetails = ToolInvocationSummary & { args?: string; summary?: string };

function statusKey(status: string): string {
  if (status === 'running') return 'chat.toolStatusRunning';
  if (status === 'failed' || status === 'error') return 'chat.toolStatusFailed';
  if (status === 'cancelled' || status === 'canceled') return 'chat.toolStatusCancelled';
  return 'chat.toolStatusSucceeded';
}

function formatArgs(raw: string): string {
  try { return JSON.stringify(JSON.parse(raw), null, 2); } catch { return raw.replace(/\t/g, '  '); }
}

function detailArguments(detail: toolinvocations.Detail): string {
  try {
    const metadata = JSON.parse(detail.metadata ?? '') as { display?: { arguments?: unknown } };
    if (typeof metadata.display?.arguments === 'string') return metadata.display.arguments;
  } catch { /* mantém entrada integral */ }
  return detail.input ?? '';
}

function detailResult(detail: toolinvocations.Detail): string {
  try {
    const output = JSON.parse(detail.output ?? '') as { content?: unknown };
    if (typeof output.content === 'string') return output.content;
  } catch { /* resultados históricos podem ser texto */ }
  return detail.output ?? '';
}

export const ToolCallsSection = React.memo<ToolCallsSectionProps>(function ToolCallsSection({
  toolInvocations,
  activeToolCalls,
  tabNavigationEnabled = false,
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const userId = useAuthStore((state) => state.user?.userId ?? '');
  const [isExpanded, setIsExpanded] = useState(false);
  const [selected, setSelected] = useState<InvocationForDetails | null>(null);
  const [detail, setDetail] = useState<toolinvocations.Detail | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState(false);

  const calls = activeToolCalls?.length ? activeToolCalls : toolInvocations;
  const isStreaming = !!activeToolCalls?.length;
  const isRunning = calls?.some((call) => call.status === 'running') ?? false;
  const summaryText = isRunning
    ? `${t('chat.executing')} ${calls?.length ?? 0} ${t('chat.toolsRunning')}`
    : `${calls?.length ?? 0} ${t('chat.toolsUsed')}`;
  const presentations = useMemo(() => (calls ?? []).map((call) => presentTool(
    call.name, call.origin, 'serverLabel' in call ? call.serverLabel : undefined,
    'args' in call ? call.args : ('inputPreview' in call ? call.inputPreview : undefined),
  )), [calls]);

  if (!calls?.length) return null;

  const openTarget = (presentation: ToolPresentation) => {
    if (!presentation.target) return;
    if (presentation.target.kind === 'url') {
      BrowserOpenURL(presentation.target.url);
      return;
    }
    void executeDeepLink({ type: 'tab:new', tabType: 'editor', file: presentation.target.path }, { navigate });
  };

  const openDetails = async (invocation: InvocationForDetails) => {
    setSelected(invocation);
    setDetail(null);
    setLoadError(false);
    if (!invocation.invocationId || !invocation.hasDetails) return;
    setLoading(true);
    try {
      const details = await loadToolInvocationDetails(userId, [invocation.invocationId]);
      const loaded = details.get(invocation.invocationId);
      if (!loaded) throw new Error('detail unavailable');
      setDetail(loaded);
    } catch {
      setLoadError(true);
      announce(t('chat.toolDetailsLoadError'), 'assertive');
    } finally { setLoading(false); }
  };

  return <>
    <div className={`tool-calls-section ${isExpanded ? 'tool-calls-section--expanded' : ''} ${isRunning ? 'tool-calls-section--running' : ''}`}>
      <button className="tool-calls-section__header" onClick={() => setIsExpanded((value) => !value)} aria-expanded={isExpanded} type="button" tabIndex={tabNavigationEnabled ? 0 : -1}>
        <span className="tool-calls-section__icon" aria-hidden="true">{isRunning ? <LoadingOutlined spin /> : <ToolOutlined />}</span>
        <span className="tool-calls-section__title">{t(isRunning ? 'chat.toolsRunningLabel' : 'chat.toolsUsedLabel')}</span>
        <span className="tool-calls-section__summary">{summaryText}</span>
        <span className={`tool-calls-section__chevron ${isExpanded ? 'tool-calls-section__chevron--expanded' : ''}`} aria-hidden="true"><DownOutlined /></span>
      </button>
      {isExpanded && <div className="tool-calls-section__content" role="region" aria-label={t('chat.toolDetails')}>
        <ul className="tool-calls-section__list">
          {calls.map((call, index) => {
            const presentation = presentations[index];
            const isActive = call.status === 'running';
            const preview = isStreaming ? (call as ToolCallStatus).summary : (call as ToolInvocationSummary).outputPreview;
            const invocation = call as InvocationForDetails;
            return <li key={call.callId} className={`tool-calls-section__item tool-calls-section__item--${call.status}`} onContextMenu={(event) => {
              event.preventDefault();
              void openDetails(invocation);
            }}>
              <div className="tool-calls-section__item-header">
                <span className="tool-calls-section__status-icon" aria-hidden="true">{isActive ? <LoadingOutlined spin /> : (call.status === 'failed' || call.status === 'error') ? <CloseCircleOutlined /> : <CheckCircleOutlined />}</span>
                <span className="tool-calls-section__intent">{t(presentation.labelKey, presentation.labelValues)}</span>
                <span className={`tool-calls-section__state tool-calls-section__state--${call.status}`}>{t(statusKey(call.status))}</span>
                {!isStreaming && !!(call as ToolInvocationSummary).durationMs && <span className="tool-calls-section__duration">{formatDuration((call as ToolInvocationSummary).durationMs!)}</span>}
              </div>
              {presentation.target && <button type="button" className="tool-calls-section__target" onClick={() => openTarget(presentation)} tabIndex={tabNavigationEnabled ? 0 : -1}>{presentation.target.label}</button>}
              {preview && <p className="tool-calls-section__result-summary">{isActive ? `${t('chat.partialOutput')}: ${preview}` : preview}</p>}
              <Button className="tool-calls-section__result-toggle" onClick={() => void openDetails(invocation)} type="button" variant="ghost" size="sm" tabIndex={tabNavigationEnabled ? 0 : -1}>{t('chat.technicalDetails')}</Button>
            </li>;
          })}
        </ul>
      </div>}
    </div>
    <Modal isOpen={!!selected} onClose={() => setSelected(null)} title={t('chat.technicalDetails')} size="lg" readingMode>
      {loading && <p>{t('chat.loadingToolDetails')}</p>}
      {loadError && <p>{t('chat.toolDetailsLoadError')}</p>}
      {selected && !loading && !loadError && <>
        <p className="tool-calls-section__technical-name">{selected.name}</p>
        <section className="tool-calls-section__section"><h2 className="tool-calls-section__section-heading">{t('chat.parameters')}</h2><pre className="tool-calls-section__args">{formatArgs(detail ? detailArguments(detail) : selected.inputPreview ?? selected.args ?? '')}</pre></section>
        <section className="tool-calls-section__section"><h2 className="tool-calls-section__section-heading">{t('chat.response')}</h2><pre className="tool-calls-section__result-content">{detail ? detailResult(detail) : selected.outputPreview ?? selected.summary ?? t('chat.toolDetailsUnavailable')}</pre></section>
      </>}
    </Modal>
  </>;
});
