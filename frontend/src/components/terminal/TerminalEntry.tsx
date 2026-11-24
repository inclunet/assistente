/**
 * TerminalCommandNode + TerminalOutputNode — exibem comando e saída como
 * nós focáveis separados, análogos ao MessageNode do chat.
 * 
 * Cada nó tem seu próprio aria-label com o conteúdo para leitura por
 * screen readers (NVDA, JAWS, etc.) ao navegar com setas.
 */

import {
  forwardRef,
  useCallback,
  useImperativeHandle,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { isManagedCommandRunning, sessionOutputSource } from '../../lib/terminalHistory';
import type { HistoryEntry } from '../../store/terminalStore';
import { playBumpSound } from '../../services/audioFeedback';
import { codePointPrefix, formatDuration } from '../../utils/format';
import { useVirtualModal } from '../../hooks/useVirtualModal';
import './TerminalEntry.css';

// ─── Shared helpers ────────────────────────────────────────────

/** Trunca texto para o aria-label (screen readers não lidam bem com textos enormes) */
function truncateForAria(text: string, maxLen: number, truncatedLabel: string): string {
  if (!text) return '';
  const clean = text.trim();
  const prefix = codePointPrefix(clean, maxLen);
  return prefix.length < clean.length ? prefix + '… ' + truncatedLabel : prefix;
}

interface NavigationProps {
  sessionBlocks?: HistoryEntry[];
  onNavigatePrev?: () => void;
  onNavigateNext?: () => void;
  onNavigateFirst?: () => void;
  onNavigateLast?: () => void;
  onReachEnd?: () => void;
}

function SessionOutputText({ blocks, label }: { blocks: HistoryEntry[]; label: string }) {
  return (
    <div>
      <span className="terminal-node__label">{label}</span>
      <pre className="terminal-node__text">{blocks.map(block => block.output)}</pre>
    </div>
  );
}

function sessionOutputAnnouncement(blocks: HistoryEntry[] | undefined, label: string, truncated: string): string {
  if (!blocks?.length) return '';
  let preview = '';
  let remaining = 500;
  for (const block of blocks) {
    const text = preview ? block.output : block.output.trimStart();
    const prefix = codePointPrefix(text, remaining);
    preview += prefix;
    if (text.slice(prefix.length).trim()) return `${label}: ${preview.trimEnd()}… ${truncated}`;
    remaining -= Array.from(prefix).length;
  }
  return `${label}: ${preview.trimEnd()}`;
}

function useNodeKeyboard(
  nav: NavigationProps,
  isReading: boolean,
  onStartReading: () => void,
) {
  return useCallback((e: React.KeyboardEvent) => {
    if (isReading) return;

    switch (e.key) {
      case 'Enter':
        e.preventDefault();
        e.stopPropagation();
        onStartReading();
        break;
      case 'ArrowUp':
        e.preventDefault();
        nav.onNavigatePrev ? nav.onNavigatePrev() : playBumpSound();
        break;
      case 'ArrowDown':
        e.preventDefault();
        nav.onNavigateNext ? nav.onNavigateNext() : (nav.onReachEnd ? nav.onReachEnd() : playBumpSound());
        break;
      case 'Home':
        e.preventDefault();
        nav.onNavigateFirst ? nav.onNavigateFirst() : playBumpSound();
        break;
      case 'End':
        e.preventDefault();
        nav.onNavigateLast ? nav.onNavigateLast() : (nav.onReachEnd ? nav.onReachEnd() : playBumpSound());
        break;
      case 'PageUp':
        e.preventDefault();
        nav.onNavigateFirst ? nav.onNavigateFirst() : playBumpSound();
        break;
      case 'PageDown':
        e.preventDefault();
        nav.onNavigateLast ? nav.onNavigateLast() : (nav.onReachEnd ? nav.onReachEnd() : playBumpSound());
        break;
      case 'Escape':
        e.preventDefault();
        if (nav.onReachEnd) nav.onReachEnd();
        break;
    }
  }, [
    isReading,
    nav.onNavigatePrev,
    nav.onNavigateNext,
    nav.onNavigateFirst,
    nav.onNavigateLast,
    nav.onReachEnd,
    onStartReading,
  ]);
}

function useTerminalReadingMode(
  forwardedRef: React.ForwardedRef<HTMLDivElement>,
  labels: { dialog: string; open: string; close: string },
) {
  const nodeRef = useRef<HTMLDivElement>(null);
  const [isReading, setIsReading] = useState(false);
  useImperativeHandle(forwardedRef, () => nodeRef.current!, []);
  useVirtualModal({
    elementRef: nodeRef,
    isActive: isReading,
    onClose: () => setIsReading(false),
    openAnnouncement: labels.open,
    closeAnnouncement: labels.close,
    contentSelector: '.terminal-node__content',
    dialogLabel: labels.dialog,
  });
  return { nodeRef, isReading, startReading: () => setIsReading(true) };
}

// ─── Command Node ──────────────────────────────────────────────

export interface TerminalCommandNodeProps extends NavigationProps {
  entry: HistoryEntry;
}

export const TerminalCommandNode = forwardRef<HTMLDivElement, TerminalCommandNodeProps>(
  function TerminalCommandNode({ entry, sessionBlocks, ...nav }, ref) {
    const { t } = useTranslation();
    const { nodeRef, isReading, startReading } = useTerminalReadingMode(ref, {
      dialog: t('terminal.entry.readingDialog'),
      open: t('terminal.entry.readingOpen'),
      close: t('terminal.entry.readingClose'),
    });
    const handleKeyDown = useNodeKeyboard(nav, isReading, startReading);

    const sourceLabel = entry.source === 'llm' ? t('terminal.entry.llmCommand') : t('terminal.entry.command');
    const sessionLabel = t('terminal.entry.sessionOutput');
    const truncated = t('terminal.entry.truncated');
    const ariaLabel = [`${sourceLabel}: ${truncateForAria(entry.command, 500, truncated)}`,
      sessionOutputAnnouncement(sessionBlocks, sessionLabel, truncated)].filter(Boolean).join('. ');

    return (
      <div
        ref={nodeRef}
        className={`terminal-node terminal-node--command ${isReading ? 'terminal-node--reading' : ''}`}
        tabIndex={-1}
        role="listitem"
        aria-label={ariaLabel}
        onKeyDown={handleKeyDown}
      >
        <div className="terminal-node__content">
          {isReading && (
            <div className="terminal-node__reading-badge">{t('terminal.entry.reading')}</div>
          )}
          <div className="terminal-node__header">
            <span className="terminal-node__icon" aria-hidden="true">&gt;_</span>
            <span className="terminal-node__label">
              {entry.source === 'llm' ? t('terminal.entry.commandLLM') : t('terminal.entry.command')}
            </span>
            {entry.source === 'llm' && (
              <span className="terminal-node__source-badge">{t('terminal.entry.llm')}</span>
            )}
          </div>
          <pre className="terminal-node__text"><code>{entry.command}</code></pre>
          {sessionBlocks && <SessionOutputText blocks={sessionBlocks} label={sessionLabel} />}
        </div>
      </div>
    );
  }
);

// ─── Output Node ───────────────────────────────────────────────

export interface TerminalOutputNodeProps extends NavigationProps {
  entry: HistoryEntry;
}

export const TerminalOutputNode = forwardRef<HTMLDivElement, TerminalOutputNodeProps>(
  function TerminalOutputNode({ entry, sessionBlocks, ...nav }, ref) {
    const { t } = useTranslation();
    const { nodeRef, isReading, startReading } = useTerminalReadingMode(ref, {
      dialog: t('terminal.entry.readingDialog'),
      open: t('terminal.entry.readingOpen'),
      close: t('terminal.entry.readingClose'),
    });
    const handleKeyDown = useNodeKeyboard(nav, isReading, startReading);

    const isRaw = entry.source === 'user-raw' || entry.source === sessionOutputSource;
    const isRunning = isManagedCommandRunning(entry);
    const hasExitCode = !isRaw && !isRunning;

    const duration = entry.endedAt && entry.startedAt && !isRaw
      ? formatDuration(new Date(entry.endedAt).getTime() - new Date(entry.startedAt).getTime())
      : null;

    const sessionLabel = t('terminal.entry.sessionOutput');
    const standalone = entry.source === sessionOutputSource ? [entry, ...(sessionBlocks ?? [])] : undefined;
    const outputLabel = standalone ? sessionLabel : t('terminal.entry.output');
    const truncatedText = t('terminal.entry.truncated');
    // Constrói aria-label com informações relevantes
    let ariaLabel = outputLabel;
    if (standalone) {
      ariaLabel = sessionOutputAnnouncement(standalone, sessionLabel, truncatedText);
    } else if (entry.output) {
      ariaLabel += `: ${truncateForAria(entry.output, 500, truncatedText)}`;
    } else if (isRunning) {
      ariaLabel += `: ${t('terminal.entry.running')}`;
    } else {
      ariaLabel += `: ${t('terminal.entry.empty')}`;
    }
    if (hasExitCode) {
      ariaLabel += `. ${t('terminal.entry.exitCode')} ${entry.exitCode}`;
    }
    if (duration) {
      ariaLabel += `. ${t('terminal.entry.duration')} ${duration}`;
    }
    if (!standalone && sessionBlocks) ariaLabel += '. ' + sessionOutputAnnouncement(sessionBlocks, sessionLabel, truncatedText);

    const exitBadgeClass = entry.exitCode === 0
      ? 'terminal-node__exit-badge--success'
      : !hasExitCode
        ? ''
        : 'terminal-node__exit-badge--error';

    return (
      <div
        ref={nodeRef}
        className={`terminal-node terminal-node--output ${isRunning ? 'terminal-node--running' : ''} ${isReading ? 'terminal-node--reading' : ''}`}
        tabIndex={-1}
        role="listitem"
        aria-label={ariaLabel}
        onKeyDown={handleKeyDown}
      >
        <div className="terminal-node__content">
          {isReading && (
            <div className="terminal-node__reading-badge">{t('terminal.entry.reading')}</div>
          )}
          <div className="terminal-node__header">
            <span className="terminal-node__icon" aria-hidden="true">&#9638;</span>
            <span className="terminal-node__label">{outputLabel}</span>
            {hasExitCode && (
              <span className={`terminal-node__exit-badge ${exitBadgeClass}`}>
                {t('terminal.entry.exit')} {entry.exitCode}
              </span>
            )}
            {duration && (
              <span className="terminal-node__duration">{duration}</span>
            )}
            {isRunning && (
              <span className="terminal-node__running-indicator" aria-hidden="true">
                <span className="terminal-node__pulse" />
                {t('terminal.entry.executing')}
              </span>
            )}
          </div>
          {entry.output && (
            <pre className="terminal-node__text">{standalone ? standalone.map(block => block.output) : entry.output}</pre>
          )}
          {!standalone && sessionBlocks && <SessionOutputText blocks={sessionBlocks} label={sessionLabel} />}
        </div>
      </div>
    );
  }
);
