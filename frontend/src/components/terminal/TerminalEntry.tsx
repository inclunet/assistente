/**
 * TerminalCommandNode + TerminalOutputNode — exibem comando e saída como
 * nós focáveis separados, análogos ao MessageNode do chat.
 * 
 * Cada nó tem seu próprio aria-label com o conteúdo para leitura por
 * screen readers (NVDA, JAWS, etc.) ao navegar com setas.
 */

import { forwardRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import type { HistoryEntry } from '../../store/terminalStore';
import { playBumpSound } from '../../services/audioFeedback';
import './TerminalEntry.css';

// ─── Shared helpers ────────────────────────────────────────────

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  const min = Math.floor(ms / 60000);
  const sec = Math.floor((ms % 60000) / 1000);
  return `${min}m ${sec}s`;
}

/** Trunca texto para o aria-label (screen readers não lidam bem com textos enormes) */
function truncateForAria(text: string, maxLen: number, truncatedLabel: string): string {
  if (!text) return '';
  const clean = text.trim();
  if (clean.length <= maxLen) return clean;
  return clean.slice(0, maxLen) + '… ' + truncatedLabel;
}

interface NavigationProps {
  onNavigatePrev?: () => void;
  onNavigateNext?: () => void;
  onNavigateFirst?: () => void;
  onNavigateLast?: () => void;
  onReachEnd?: () => void;
}

function useNodeKeyboard(nav: NavigationProps) {
  return useCallback((e: React.KeyboardEvent) => {
    switch (e.key) {
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
  }, [nav.onNavigatePrev, nav.onNavigateNext, nav.onNavigateFirst, nav.onNavigateLast, nav.onReachEnd]);
}

// ─── Command Node ──────────────────────────────────────────────

export interface TerminalCommandNodeProps extends NavigationProps {
  entry: HistoryEntry;
}

export const TerminalCommandNode = forwardRef<HTMLDivElement, TerminalCommandNodeProps>(
  function TerminalCommandNode({ entry, ...nav }, ref) {
    const { t } = useTranslation();
    const handleKeyDown = useNodeKeyboard(nav);

    const sourceLabel = entry.source === 'llm' ? t('terminal.entry.llmCommand') : t('terminal.entry.command');
    const ariaLabel = `${sourceLabel}: ${truncateForAria(entry.command, 500, t('terminal.entry.truncated'))}`;

    return (
      <div
        ref={ref}
        className="terminal-node terminal-node--command"
        tabIndex={-1}
        role="listitem"
        aria-label={ariaLabel}
        onKeyDown={handleKeyDown}
      >
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
      </div>
    );
  }
);

// ─── Output Node ───────────────────────────────────────────────

export interface TerminalOutputNodeProps extends NavigationProps {
  entry: HistoryEntry;
}

export const TerminalOutputNode = forwardRef<HTMLDivElement, TerminalOutputNodeProps>(
  function TerminalOutputNode({ entry, ...nav }, ref) {
    const { t } = useTranslation();
    const handleKeyDown = useNodeKeyboard(nav);

    const isRaw = entry.source === 'user-raw';
    const hasExitCode = !isRaw && entry.exitCode !== -999;
    const isRunning = entry.exitCode === -999 && !isRaw;

    const duration = entry.endedAt && entry.startedAt && !isRaw
      ? formatDuration(new Date(entry.endedAt).getTime() - new Date(entry.startedAt).getTime())
      : null;

    const outputLabel = t('terminal.entry.output');
    const truncatedText = t('terminal.entry.truncated');
    // Constrói aria-label com informações relevantes
    let ariaLabel = outputLabel;
    if (entry.output) {
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

    const exitBadgeClass = entry.exitCode === 0
      ? 'terminal-node__exit-badge--success'
      : entry.exitCode === -999
        ? ''
        : 'terminal-node__exit-badge--error';

    return (
      <div
        ref={ref}
        className={`terminal-node terminal-node--output ${isRunning ? 'terminal-node--running' : ''}`}
        tabIndex={-1}
        role="listitem"
        aria-label={ariaLabel}
        onKeyDown={handleKeyDown}
      >
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
          <pre className="terminal-node__text">{entry.output}</pre>
        )}
      </div>
    );
  }
);
