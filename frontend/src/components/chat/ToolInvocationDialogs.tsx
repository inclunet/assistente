import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import type { toolinvocations } from '@wailsjs/go/models';
import { useTranslation } from 'react-i18next';
import type { ToolInvocationSummary } from '../../lib/chatMessageTree';
import {
  parseSearchResultPresentation,
  type SearchResultPresentation,
  type SearchResultTarget,
} from '../../lib/searchResultPresentation';
import { sanitizeToolDetailArguments } from '../../lib/toolDetailSanitization';
import { openToolNavigationTarget } from '../../lib/toolTargetNavigation';
import { announce } from '../../hooks/useAnnouncer';
import { loadToolInvocationDetails } from '../../services/toolInvocationDetailsCache';
import { useAuthStore } from '../../store/authStore';
import { type ToolCallStatus } from '../../types/chat';
import { Button } from '../ui/Button';
import { isModalOpen, Modal } from '../ui/Modal';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import './ToolCallsSection.css';

export type ToolInvocationDialogCall = ToolInvocationSummary | ToolCallStatus;
export type InvocationForDetails = {
  callId: string;
  name: string;
  status?: string;
  invocationId?: string;
  hasDetails?: boolean;
  inputPreview?: string;
  outputPreview?: string;
  args?: string;
  summary?: string;
  securityOutcome?: string;
  durationMs?: number;
  hasSearchResults?: boolean;
  searchResultCount?: number;
  [key: string]: unknown;
};
const SEARCH_RESULTS_PAGE_SIZE = 20;
type ToolDisplayStatus = 'running' | 'succeeded' | 'failed' | 'cancelled' | 'unknown';

export function displayToolStatus(status: string | undefined): ToolDisplayStatus {
  switch (status?.trim().toLowerCase()) {
    case 'queued':
    case 'pending':
    case 'running':
      return 'running';
    case 'succeeded':
    case 'completed':
    case 'done':
      return 'succeeded';
    case 'failed':
    case 'error':
    case 'timed_out':
    case 'timeout':
      return 'failed';
    case 'cancelled':
    case 'canceled':
      return 'cancelled';
    default:
      return 'unknown';
  }
}
function statusKey(status: ToolDisplayStatus): string {
  return `chat.toolStatus${status.charAt(0).toUpperCase()}${status.slice(1)}`;
}
function formatArgs(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw.replace(/\t/g, '  ');
  }
}
function detailArguments(detail: toolinvocations.Detail): string {
  try {
    const metadata = JSON.parse(detail.metadata ?? '') as { display?: { arguments?: unknown } };
    if (typeof metadata.display?.arguments === 'string')
      return sanitizeToolDetailArguments(metadata.display.arguments);
  } catch {
    /* fallback */
  }
  return sanitizeToolDetailArguments(detail.input ?? '');
}
function detailResult(detail: toolinvocations.Detail): string {
  try {
    const output = JSON.parse(detail.output ?? '') as { content?: unknown };
    if (typeof output.content === 'string') return output.content;
  } catch {
    /* historical text */
  }
  return detail.output ?? '';
}

interface ToolInvocationDialogsContextValue {
  openDetails: (invocation: InvocationForDetails, origin?: HTMLElement) => void;
  openSearchResults: (invocation: InvocationForDetails, origin?: HTMLElement) => void;
}
const ToolInvocationDialogsContext = createContext<ToolInvocationDialogsContextValue | null>(null);
export function useToolInvocationDialogs() {
  return useContext(ToolInvocationDialogsContext);
}

export function ToolInvocationDialogsProvider({
  currentCalls,
  children,
}: {
  currentCalls: ToolInvocationDialogCall[];
  children: React.ReactNode;
}) {
  const { t } = useTranslation();
  const userId = useAuthStore((state) => state.user?.userId ?? '');
  const [selected, setSelected] = useState<InvocationForDetails | null>(null);
  const [detail, setDetail] = useState<toolinvocations.Detail | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState(false);
  const [searchSelected, setSearchSelected] = useState<InvocationForDetails | null>(null);
  const [searchPresentation, setSearchPresentation] = useState<SearchResultPresentation | null>(
    null
  );
  const [searchLoading, setSearchLoading] = useState(false);
  const [searchLoadError, setSearchLoadError] = useState(false);
  const [searchPage, setSearchPage] = useState(0);
  const detailRequestRef = useRef(0);
  const searchRequestRef = useRef(0);
  const previousSelectedStatusRef = useRef<string | undefined>(undefined);
  const detailOriginRef = useRef<HTMLElement | null>(null);
  const searchOriginRef = useRef<HTMLElement | null>(null);
  const detailOwnerRef = useRef<string | null>(null);
  const searchOwnerRef = useRef<string | null>(null);
  useEffect(() => {
    if (detailOwnerRef.current && detailOwnerRef.current !== userId) {
      detailRequestRef.current += 1;
      setSelected(null);
      setDetail(null);
      setLoadError(false);
    }
    if (searchOwnerRef.current && searchOwnerRef.current !== userId) {
      searchRequestRef.current += 1;
      setSearchSelected(null);
      setSearchPresentation(null);
      setSearchLoadError(false);
    }
  }, [userId]);
  const selectedCurrent = useMemo(() => {
    if (!selected) return null;
    return (
      (currentCalls.find(
        (call) =>
          call.callId === selected.callId ||
          (!!selected.invocationId &&
            'invocationId' in call &&
            call.invocationId === selected.invocationId)
      ) as InvocationForDetails | undefined) ?? selected
    );
  }, [currentCalls, selected]);
  useEffect(() => {
    const status = selectedCurrent ? displayToolStatus(selectedCurrent.status) : 'unknown';
    if (
      !selected ||
      !selectedCurrent?.invocationId ||
      !selectedCurrent.hasDetails ||
      status === 'running' ||
      detail ||
      loadError
    ) {
      setLoading(false);
      return;
    }
    const requestId = ++detailRequestRef.current;
    setLoading(true);
    const requestUserId = userId;
    void loadToolInvocationDetails(requestUserId, [selectedCurrent.invocationId])
      .then((details) => {
        if (
          requestId !== detailRequestRef.current ||
          requestUserId !== userId ||
          detailOwnerRef.current !== userId
        )
          return;
        const loaded = details.get(selectedCurrent.invocationId!);
        if (!loaded) throw new Error('detail unavailable');
        setDetail(loaded);
      })
      .catch(() => {
        if (requestId !== detailRequestRef.current || requestUserId !== userId) return;
        setLoadError(true);
        announce(t('chat.toolDetailsLoadError'), 'assertive');
      })
      .finally(() => {
        if (requestId === detailRequestRef.current && requestUserId === userId) setLoading(false);
      });
    return () => {
      detailRequestRef.current += 1;
    };
  }, [detail, loadError, selected, selectedCurrent, t, userId]);
  useEffect(() => {
    const status = selectedCurrent ? displayToolStatus(selectedCurrent.status) : 'unknown';
    if (!selected || previousSelectedStatusRef.current === status) return;
    if (previousSelectedStatusRef.current) announce(t(statusKey(status)), 'polite');
    previousSelectedStatusRef.current = status;
  }, [selected, selectedCurrent?.status, t]);
  const openDetails = useCallback(
    (invocation: InvocationForDetails, origin?: HTMLElement) => {
      detailRequestRef.current += 1;
      detailOwnerRef.current = userId;
      detailOriginRef.current = origin ?? null;
      setSelected(invocation);
      setDetail(null);
      setLoadError(false);
      setLoading(false);
    },
    [userId]
  );
  const openSearchResults = useCallback(
    async (invocation: InvocationForDetails, origin?: HTMLElement) => {
      const requestId = ++searchRequestRef.current;
      const requestUserId = userId;
      searchOwnerRef.current = requestUserId;
      searchOriginRef.current = origin ?? null;
      setSearchSelected(invocation);
      setSearchPresentation(null);
      setSearchLoadError(false);
      setSearchPage(0);
      if (!invocation.invocationId) return;
      setSearchLoading(true);
      try {
        const details = await loadToolInvocationDetails(requestUserId, [invocation.invocationId]);
        const loaded = details.get(invocation.invocationId);
        const presentation = loaded && parseSearchResultPresentation(loaded.metadata);
        if (!presentation) throw new Error('search presentation unavailable');
        if (
          requestId === searchRequestRef.current &&
          requestUserId === userId &&
          searchOwnerRef.current === userId
        )
          setSearchPresentation(presentation);
      } catch {
        if (requestId !== searchRequestRef.current || requestUserId !== userId) return;
        setSearchLoadError(true);
        announce(t('chat.searchResultsLoadError'), 'assertive');
      } finally {
        if (requestId === searchRequestRef.current && requestUserId === userId)
          setSearchLoading(false);
      }
    },
    [t, userId]
  );
  const closeDetails = () => {
    detailRequestRef.current += 1;
    setSelected(null);
    const origin = detailOriginRef.current;
    requestAnimationFrame(() => {
      if (isModalOpen()) return;
      if (origin?.isConnected) origin.focus();
      else restoreDefaultFocus();
    });
    detailOriginRef.current = null;
  };
  const closeSearchResults = () => {
    searchRequestRef.current += 1;
    setSearchSelected(null);
    const origin = searchOriginRef.current;
    requestAnimationFrame(() => {
      if (isModalOpen()) return;
      if (origin?.isConnected) origin.focus();
      else restoreDefaultFocus();
    });
    searchOriginRef.current = null;
  };
  const context = useMemo(
    () => ({ openDetails, openSearchResults }),
    [openDetails, openSearchResults]
  );
  const searchPageCount = searchPresentation
    ? Math.max(1, Math.ceil(searchPresentation.items.length / SEARCH_RESULTS_PAGE_SIZE))
    : 0;
  const visibleSearchItems =
    searchPresentation?.items.slice(
      searchPage * SEARCH_RESULTS_PAGE_SIZE,
      (searchPage + 1) * SEARCH_RESULTS_PAGE_SIZE
    ) ?? [];
  const openTarget = async (target: NonNullable<SearchResultTarget> | undefined) => {
    if (!target) return;
    try {
      await openToolNavigationTarget(target);
    } catch {
      announce(t('chat.toolTargetOpenFailed'), 'assertive');
    }
  };
  return (
    <ToolInvocationDialogsContext.Provider value={context}>
      <>
        {children}
        <Modal
          isOpen={!!selected && detailOwnerRef.current === userId}
          onClose={closeDetails}
          returnFocusOnClose={false}
          title={`${t('chat.technicalDetails')} — ${t(statusKey(displayToolStatus(selectedCurrent?.status)))}`}
          size="lg"
          readingMode
        >
          {selectedCurrent && (
            <>
              <p className="tool-calls-section__technical-name">{selectedCurrent.name}</p>
              <p className="tool-calls-section__state">
                {t(statusKey(displayToolStatus(selectedCurrent.status)))}
              </p>
              {loading && <p>{t('chat.loadingToolDetails')}</p>}
              {loadError && <p>{t('chat.toolDetailsLoadError')}</p>}
              {!loading && !loadError && (
                <>
                  <section className="tool-calls-section__section">
                    <h2 className="tool-calls-section__section-heading">{t('chat.parameters')}</h2>
                    <pre className="tool-calls-section__args">
                      {formatArgs(
                        detail
                          ? detailArguments(detail)
                          : sanitizeToolDetailArguments(
                              selectedCurrent.inputPreview ?? selectedCurrent.args ?? ''
                            )
                      )}
                    </pre>
                  </section>
                  <section className="tool-calls-section__section">
                    <h2 className="tool-calls-section__section-heading">{t('chat.response')}</h2>
                    <pre className="tool-calls-section__result-content">
                      {detail
                        ? detailResult(detail)
                        : (displayToolStatus(selectedCurrent.status) === 'running'
                            ? `${t('chat.partialOutput')}: `
                            : '') +
                          (selectedCurrent.outputPreview ??
                            selectedCurrent.summary ??
                            t('chat.toolDetailsUnavailable'))}
                    </pre>
                  </section>
                </>
              )}
            </>
          )}
        </Modal>
        <Modal
          isOpen={!!searchSelected && searchOwnerRef.current === userId}
          onClose={closeSearchResults}
          returnFocusOnClose={false}
          title={t('chat.searchResults')}
          size="lg"
          readingMode
        >
          {searchLoading && <p>{t('chat.loadingSearchResults')}</p>}
          {searchLoadError && <p>{t('chat.searchResultsLoadError')}</p>}
          {searchPresentation && !searchLoading && !searchLoadError && (
            <>
              <p className="tool-calls-section__search-summary">
                {t('chat.searchResultsSummary', {
                  count: searchPresentation.total,
                  shown: searchPresentation.items.length,
                })}
                {searchPresentation.truncated
                  ? ` ${t('chat.searchResultsTruncated', { shown: searchPresentation.items.length, total: searchPresentation.total })}`
                  : ''}
              </p>
              <ul className="tool-calls-section__search-results">
                {visibleSearchItems.map((item, index) => (
                  <li
                    key={`${item.title}-${searchPage * SEARCH_RESULTS_PAGE_SIZE + index}`}
                    className="tool-calls-section__search-result"
                  >
                    {item.target ? (
                      <button
                        type="button"
                        className="tool-calls-section__target"
                        onClick={() => void openTarget(item.target)}
                      >
                        {item.title}
                      </button>
                    ) : (
                      <span>{item.title}</span>
                    )}
                    {item.snippet && <p>{item.snippet}</p>}
                  </li>
                ))}
              </ul>
              {searchPageCount > 1 && (
                <nav
                  className="tool-calls-section__search-pagination"
                  aria-label={t('chat.searchResultsPagination')}
                >
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={searchPage === 0}
                    onClick={() => setSearchPage((page) => Math.max(0, page - 1))}
                  >
                    {t('chat.previousPage')}
                  </Button>
                  <span>
                    {t('chat.searchResultsPage', {
                      current: searchPage + 1,
                      total: searchPageCount,
                    })}
                  </span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={searchPage + 1 >= searchPageCount}
                    onClick={() => setSearchPage((page) => Math.min(searchPageCount - 1, page + 1))}
                  >
                    {t('chat.nextPage')}
                  </Button>
                </nav>
              )}
            </>
          )}
        </Modal>
      </>
    </ToolInvocationDialogsContext.Provider>
  );
}
