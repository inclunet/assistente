import React, { useMemo, useState } from 'react';
import { CheckCircleOutlined, CloseCircleOutlined, DownOutlined, LoadingOutlined, ToolOutlined } from '@ant-design/icons';
import type { toolinvocations } from '@wailsjs/go/models';
import { useTranslation } from 'react-i18next';
import type { ToolInvocationSummary } from '../../lib/chatMessageTree';
import { presentTool, type ToolPresentation } from '../../lib/toolPresentation';
import { parseSearchResultPresentation, type SearchResultPresentation, type SearchResultTarget } from '../../lib/searchResultPresentation';
import { sanitizeToolDetailArguments } from '../../lib/toolDetailSanitization';
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
const SEARCH_RESULTS_PAGE_SIZE = 20;

type ToolDisplayStatus = 'running' | 'succeeded' | 'failed' | 'cancelled' | 'unknown';

/** Mantém a timeline conservadora: estado fora do contrato jamais parece sucesso. */
function displayStatus(status: string | undefined): ToolDisplayStatus {
  switch (status?.trim().toLowerCase()) {
    case 'queued':
    case 'pending':
    case 'running': return 'running';
    case 'succeeded':
    case 'completed': return 'succeeded'; // registros anteriores à normalização do ledger
    case 'failed':
    case 'error': return 'failed';
    case 'cancelled':
    case 'canceled': return 'cancelled';
    default: return 'unknown';
  }
}

function statusKey(status: ToolDisplayStatus): string {
  return `chat.toolStatus${status.charAt(0).toUpperCase()}${status.slice(1)}`;
}

function formatArgs(raw: string): string {
  try { return JSON.stringify(JSON.parse(raw), null, 2); } catch { return raw.replace(/\t/g, '  '); }
}

function detailArguments(detail: toolinvocations.Detail): string {
  try {
    const metadata = JSON.parse(detail.metadata ?? '') as { display?: { arguments?: unknown } };
    if (typeof metadata.display?.arguments === 'string') return sanitizeToolDetailArguments(metadata.display.arguments);
  } catch { /* mantém entrada integral */ }
  return sanitizeToolDetailArguments(detail.input ?? '');
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
  const userId = useAuthStore((state) => state.user?.userId ?? '');
  const [isExpanded, setIsExpanded] = useState(false);
  const [selected, setSelected] = useState<InvocationForDetails | null>(null);
  const [detail, setDetail] = useState<toolinvocations.Detail | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState(false);
  const [searchSelected, setSearchSelected] = useState<InvocationForDetails | null>(null);
  const [searchPresentation, setSearchPresentation] = useState<SearchResultPresentation | null>(null);
  const [searchLoading, setSearchLoading] = useState(false);
  const [searchLoadError, setSearchLoadError] = useState(false);
  const [searchPage, setSearchPage] = useState(0);

  const calls = activeToolCalls?.length ? activeToolCalls : toolInvocations;
  const isStreaming = !!activeToolCalls?.length;
  const isRunning = calls?.some((call) => displayStatus(call.status) === 'running') ?? false;
  const summaryText = isRunning
    ? `${t('chat.executing')} ${calls?.length ?? 0} ${t('chat.toolsRunning')}`
    : `${calls?.length ?? 0} ${t('chat.toolsUsed')}`;
  const presentations = useMemo(() => (calls ?? []).map((call) => presentTool(
    call.name, call.origin, 'serverLabel' in call ? call.serverLabel : undefined,
    'args' in call ? call.args : ('inputPreview' in call ? call.inputPreview : undefined),
  )), [calls]);

  if (!calls?.length) return null;

  const openTarget = async (target: NonNullable<ToolPresentation['target']> | SearchResultTarget | undefined) => {
    if (!target) return;
    if (target.kind === 'url') {
      const { BrowserOpenURL } = await import('@wailsjs/runtime/runtime');
      BrowserOpenURL(target.url);
      return;
    }
    // Esta seção também aparece em superfícies isoladas sem Router. Abrir uma
    // aba de editor só precisa da navegação de workspace; a rota raiz é a
    // mesma, portanto a dependência de navegação pode ser neutra aqui.
    const { executeDeepLink } = await import('../../lib/deepLinks');
    await executeDeepLink({ type: 'tab:new', tabType: 'editor', file: target.path }, { navigate: () => undefined });
  };

  const openSearchResults = async (invocation: InvocationForDetails) => {
    setSearchSelected(invocation);
    setSearchPresentation(null);
    setSearchLoadError(false);
    setSearchPage(0);
    if (!invocation.invocationId) return;
    setSearchLoading(true);
    try {
      const details = await loadToolInvocationDetails(userId, [invocation.invocationId]);
      const loaded = details.get(invocation.invocationId);
      const presentation = loaded && parseSearchResultPresentation(loaded.metadata);
      if (!presentation) throw new Error('search presentation unavailable');
      setSearchPresentation(presentation);
    } catch {
      setSearchLoadError(true);
      announce(t('chat.searchResultsLoadError'), 'assertive');
    } finally { setSearchLoading(false); }
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

  const searchPageCount = searchPresentation ? Math.max(1, Math.ceil(searchPresentation.items.length / SEARCH_RESULTS_PAGE_SIZE)) : 0;
  const visibleSearchItems = searchPresentation?.items.slice(
    searchPage * SEARCH_RESULTS_PAGE_SIZE,
    (searchPage + 1) * SEARCH_RESULTS_PAGE_SIZE,
  ) ?? [];

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
            const callStatus = displayStatus(call.status);
            const isActive = callStatus === 'running';
            const preview = isStreaming ? (call as ToolCallStatus).summary : (call as ToolInvocationSummary).outputPreview;
            const invocation = call as InvocationForDetails;
            return <li key={call.callId} className={`tool-calls-section__item tool-calls-section__item--${callStatus}`} onContextMenu={(event) => {
              event.preventDefault();
              void openDetails(invocation);
            }}>
              <div className="tool-calls-section__item-header">
                <span className="tool-calls-section__status-icon" aria-hidden="true">{isActive ? <LoadingOutlined spin /> : callStatus === 'failed' || callStatus === 'cancelled' ? <CloseCircleOutlined /> : callStatus === 'succeeded' ? <CheckCircleOutlined /> : <ToolOutlined />}</span>
                <span className="tool-calls-section__intent">{t(presentation.labelKey, presentation.labelValues)}</span>
                <span className={`tool-calls-section__state tool-calls-section__state--${callStatus}`}>{t(statusKey(callStatus))}</span>
                {!isStreaming && !!(call as ToolInvocationSummary).durationMs && <span className="tool-calls-section__duration">{formatDuration((call as ToolInvocationSummary).durationMs!)}</span>}
              </div>
              {presentation.target && <button type="button" className="tool-calls-section__target" onClick={() => void openTarget(presentation.target)} tabIndex={tabNavigationEnabled ? 0 : -1}>{presentation.target.label}</button>}
              {preview && <p className="tool-calls-section__result-summary">{isActive ? `${t('chat.partialOutput')}: ${preview}` : preview}</p>}
              {!isStreaming && invocation.hasSearchResults && <Button className="tool-calls-section__result-toggle" onClick={() => void openSearchResults(invocation)} type="button" variant="ghost" size="sm" tabIndex={tabNavigationEnabled ? 0 : -1}>{t('chat.viewSearchResults', { count: invocation.searchResultCount ?? 0 })}</Button>}
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
    <Modal isOpen={!!searchSelected} onClose={() => setSearchSelected(null)} title={t('chat.searchResults')} size="lg" readingMode>
      {searchLoading && <p>{t('chat.loadingSearchResults')}</p>}
      {searchLoadError && <p>{t('chat.searchResultsLoadError')}</p>}
      {searchPresentation && !searchLoading && !searchLoadError && <>
        <p className="tool-calls-section__search-summary">{t('chat.searchResultsSummary', { count: searchPresentation.total })}{searchPresentation.truncated ? ` ${t('chat.searchResultsTruncated')}` : ''}</p>
        <ul className="tool-calls-section__search-results">
          {visibleSearchItems.map((item, index) => <li key={`${item.title}-${searchPage * SEARCH_RESULTS_PAGE_SIZE + index}`} className="tool-calls-section__search-result">
            {item.target ? <button type="button" className="tool-calls-section__target" onClick={() => void openTarget(item.target)}>{item.title}</button> : <span>{item.title}</span>}
            {item.snippet && <p>{item.snippet}</p>}
          </li>)}
        </ul>
        {searchPageCount > 1 && <nav className="tool-calls-section__search-pagination" aria-label={t('chat.searchResultsPagination')}>
          <Button type="button" variant="ghost" size="sm" disabled={searchPage === 0} onClick={() => setSearchPage((page) => Math.max(0, page - 1))}>{t('chat.previousPage')}</Button>
          <span>{t('chat.searchResultsPage', { current: searchPage + 1, total: searchPageCount })}</span>
          <Button type="button" variant="ghost" size="sm" disabled={searchPage + 1 >= searchPageCount} onClick={() => setSearchPage((page) => Math.min(searchPageCount - 1, page + 1))}>{t('chat.nextPage')}</Button>
        </nav>}
      </>}
    </Modal>
  </>;
});
