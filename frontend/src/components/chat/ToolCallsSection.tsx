import React, { useMemo, useState } from 'react';
import { CheckCircleOutlined, CloseCircleOutlined, DownOutlined, LoadingOutlined, ToolOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { ToolInvocationSummary } from '../../lib/chatMessageTree';
import { presentTool } from '../../lib/toolPresentation';
import { type ToolCallStatus } from '../../types/chat';
import { formatDuration } from '../../utils/format';
import { Button } from '../ui/Button';
import { ToolInvocationDialogsProvider, useToolInvocationDialogs, displayToolStatus, type InvocationForDetails } from './ToolInvocationDialogs';
import './ToolCallsSection.css';

export interface ToolCallsSectionProps {
  toolInvocations?: ToolInvocationSummary[];
  activeToolCalls?: ToolCallStatus[];
  tabNavigationEnabled?: boolean;
}


const ToolCallsSectionContent = React.memo<ToolCallsSectionProps>(function ToolCallsSection({
  toolInvocations,
  activeToolCalls,
  tabNavigationEnabled = false,
}) {
  const { t } = useTranslation();
  const dialogs = useToolInvocationDialogs();
  const [isExpanded, setIsExpanded] = useState(false);

  const calls = activeToolCalls?.length ? activeToolCalls : toolInvocations;
  const isStreaming = !!activeToolCalls?.length;
  const isRunning = calls?.some((call) => displayToolStatus(call.status) === 'running') ?? false;
  const summaryText = isRunning
    ? `${t('chat.executing')} ${calls?.length ?? 0} ${t('chat.toolsRunning')}`
    : `${calls?.length ?? 0} ${t('chat.toolsUsed')}`;
  const presentations = useMemo(() => (calls ?? []).map((call) => presentTool(
    call.name, call.origin, 'serverLabel' in call ? call.serverLabel : undefined,
    'args' in call ? call.args : ('inputPreview' in call ? call.inputPreview : undefined),
  )), [calls]);

  if (!calls?.length) return null;

  return (
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
            const callStatus = displayToolStatus(call.status);
            const isActive = callStatus === 'running';
            const preview = isStreaming ? (call as ToolCallStatus).summary : (call as ToolInvocationSummary).outputPreview;
            const invocation = call as InvocationForDetails;
            const target = presentation.target;
            return <li key={call.callId} className={`tool-calls-section__item tool-calls-section__item--${callStatus}`} onContextMenu={(event) => {
              event.preventDefault();
              dialogs?.openDetails(invocation, event.currentTarget.querySelector<HTMLElement>('button.tool-calls-section__result-toggle:last-of-type') ?? undefined);
            }}>
              <div className="tool-calls-section__item-header">
                <span className="tool-calls-section__status-icon" aria-hidden="true">{isActive ? <LoadingOutlined spin /> : callStatus === 'failed' || callStatus === 'cancelled' ? <CloseCircleOutlined /> : callStatus === 'succeeded' ? <CheckCircleOutlined /> : <ToolOutlined />}</span>
                <span className="tool-calls-section__intent">{t(presentation.labelKey, presentation.labelValues)}</span>
                <span className={`tool-calls-section__state tool-calls-section__state--${callStatus}`}>{t(`chat.toolStatus${callStatus.charAt(0).toUpperCase()}${callStatus.slice(1)}`)}</span>
                {!isStreaming && (invocation.securityOutcome === 'approved' || invocation.securityOutcome === 'blocked') && <span className={`tool-calls-section__security tool-calls-section__security--${invocation.securityOutcome}`}>{t(invocation.securityOutcome === 'approved' ? 'chat.toolSecurityApproved' : 'chat.toolSecurityBlocked')}</span>}
                {!isStreaming && !!(call as ToolInvocationSummary).durationMs && <span className="tool-calls-section__duration">{formatDuration((call as ToolInvocationSummary).durationMs!)}</span>}
              </div>
              {target && <button type="button" className="tool-calls-section__target" onClick={() => { if (target.kind === 'url') void import('@wailsjs/runtime/runtime').then(({ BrowserOpenURL }) => BrowserOpenURL(target.url)); else void import('../../lib/deepLinks').then(({ executeDeepLink }) => executeDeepLink({ type: 'tab:new', tabType: 'editor', file: target.path }, { navigate: () => undefined })); }} tabIndex={tabNavigationEnabled ? 0 : -1}>{target.label}</button>}
              {preview && <p className="tool-calls-section__result-summary">{isActive ? `${t('chat.partialOutput')}: ${preview}` : preview}</p>}
              {!isStreaming && invocation.hasSearchResults && <Button className="tool-calls-section__result-toggle" onClick={(event) => void dialogs?.openSearchResults(invocation, event.currentTarget)} type="button" variant="ghost" size="sm" tabIndex={tabNavigationEnabled ? 0 : -1}>{t('chat.viewSearchResults', { count: invocation.searchResultCount ?? 0 })}</Button>}
              <Button className="tool-calls-section__result-toggle" onClick={(event) => void dialogs?.openDetails(invocation, event.currentTarget)} type="button" variant="ghost" size="sm" tabIndex={tabNavigationEnabled ? 0 : -1}>{t('chat.technicalDetails')}</Button>
            </li>;
          })}
        </ul>
      </div>}
    </div>
  );
});

export function ToolCallsSection(props: ToolCallsSectionProps) {
  const existingDialogs = useToolInvocationDialogs();
  const calls = props.activeToolCalls?.length ? props.activeToolCalls : props.toolInvocations;
  if (existingDialogs) return <ToolCallsSectionContent {...props} />;
  return <ToolInvocationDialogsProvider currentCalls={calls ?? []}><ToolCallsSectionContent {...props} /></ToolInvocationDialogsProvider>;
}
