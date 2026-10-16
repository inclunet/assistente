import React, { useMemo } from 'react';
import { CheckCircleOutlined, CloseCircleOutlined, LoadingOutlined, ToolOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import type { ToolInvocationSummary } from '../../lib/chatMessageTree';
import { formatToolPresentation, presentTool } from '../../lib/toolPresentation';
import { type ToolCallStatus } from '../../types/chat';
import { formatDuration } from '../../utils/format';
import { announce } from '../../hooks/useAnnouncer';
import { openToolNavigationTarget } from '../../lib/toolTargetNavigation';
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
  const calls = activeToolCalls?.length ? activeToolCalls : toolInvocations;
  const isStreaming = !!activeToolCalls?.length;
  const presentations = useMemo(() => (calls ?? []).map((call) => presentTool(
    call.name, call.origin, 'serverLabel' in call ? call.serverLabel : undefined,
    'args' in call ? call.args : ('inputPreview' in call ? call.inputPreview : undefined),
  )), [calls]);

  if (!calls?.length) return null;

  return (
    <div className="tool-calls-section" role="list" aria-label={t('chat.toolActivity')}>
          {calls.map((call, index) => {
            const presentation = presentations[index];
            const callStatus = displayToolStatus(call.status);
            const friendlyLabel = formatToolPresentation(presentation, (key, values) => t(key, values), callStatus === 'succeeded');
            const isActive = callStatus === 'running';
            const preview = isStreaming ? (call as ToolCallStatus).summary : (call as ToolInvocationSummary).outputPreview;
            const hasPartialOutput = isActive && !!preview;
            const invocation = call as InvocationForDetails;
            const target = presentation.target;
            const openTarget = async () => {
              if (!target) return;
              try {
                await openToolNavigationTarget(target);
              } catch {
                announce(t('chat.toolTargetOpenFailed'), 'assertive');
              }
            };
            return <div role="listitem" key={call.callId} className={`tool-calls-section__item tool-calls-section__item--${callStatus}`} onContextMenu={(event) => {
              event.preventDefault();
              dialogs?.openDetails(invocation, event.currentTarget.querySelector<HTMLElement>('button.tool-calls-section__result-toggle:last-of-type') ?? undefined);
            }}>
              <div className="tool-calls-section__item-header">
                <span className="tool-calls-section__status-icon" aria-hidden="true">{isActive ? <LoadingOutlined spin /> : callStatus === 'failed' || callStatus === 'cancelled' ? <CloseCircleOutlined /> : callStatus === 'succeeded' ? <CheckCircleOutlined /> : <ToolOutlined />}</span>
                {target
                  ? <button type="button" className="tool-calls-section__intent tool-calls-section__intent--target" onClick={() => void openTarget()} tabIndex={tabNavigationEnabled ? 0 : -1}>{friendlyLabel}</button>
                  : <span className="tool-calls-section__intent">{friendlyLabel}</span>}
                <span className={`tool-calls-section__state tool-calls-section__state--${callStatus}`}>{t(`chat.toolStatus${callStatus.charAt(0).toUpperCase()}${callStatus.slice(1)}`)}</span>
                {!isStreaming && (invocation.securityOutcome === 'approved' || invocation.securityOutcome === 'blocked') && <span className={`tool-calls-section__security tool-calls-section__security--${invocation.securityOutcome}`}>{t(invocation.securityOutcome === 'approved' ? 'chat.toolSecurityApproved' : 'chat.toolSecurityBlocked')}</span>}
                {!isStreaming && !!(call as ToolInvocationSummary).durationMs && <span className="tool-calls-section__duration">{formatDuration((call as ToolInvocationSummary).durationMs!)}</span>}
              </div>
              {hasPartialOutput && <p className="tool-calls-section__result-summary">{t('chat.partialOutputAvailable')}</p>}
              {!isStreaming && invocation.hasSearchResults && <Button className="tool-calls-section__result-toggle" onClick={(event) => void dialogs?.openSearchResults(invocation, event.currentTarget)} type="button" variant="ghost" size="sm" tabIndex={tabNavigationEnabled ? 0 : -1}>{t('chat.viewSearchResults', { count: invocation.searchResultCount ?? 0 })}</Button>}
              <Button className="tool-calls-section__result-toggle" onClick={(event) => void dialogs?.openDetails(invocation, event.currentTarget)} type="button" variant="ghost" size="sm" tabIndex={tabNavigationEnabled ? 0 : -1}>{t('chat.technicalDetails')}</Button>
            </div>;
          })}
    </div>
  );
});

export function ToolCallsSection(props: ToolCallsSectionProps) {
  const existingDialogs = useToolInvocationDialogs();
  const calls = props.activeToolCalls?.length ? props.activeToolCalls : props.toolInvocations;
  if (existingDialogs) return <ToolCallsSectionContent {...props} />;
  return <ToolInvocationDialogsProvider currentCalls={calls ?? []}><ToolCallsSectionContent {...props} /></ToolInvocationDialogsProvider>;
}
