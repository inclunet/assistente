/**
 * TerminalHistory — renderiza entries de terminal como uma lista plana de nós
 * focáveis: cada entry gera um nó "comando" e (se houver output) um nó "saída".
 * 
 * A navegação por teclado (ArrowUp/Down, Home/End) percorre todos os nós
 * individualmente, permitindo ao leitor de tela ler cada parte separadamente.
 */

import { useEffect, useRef, useCallback, forwardRef, useImperativeHandle, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { HistoryEntry } from '../../store/terminalStore';
import { TerminalCommandNode, TerminalOutputNode } from './TerminalEntry';
import { announce } from '../../hooks/useAnnouncer';
import './TerminalHistory.css';

interface TerminalHistoryProps {
  entries: HistoryEntry[];
  runningCommandId: string | null;
  isLoading?: boolean;
  /** Chamado quando ArrowDown no último nó — foco volta ao input */
  onReachEnd?: () => void;
}

/** Tipo de nó na lista plana */
interface FlatNode {
  /** ID único do nó (para refs e keys) */
  nodeId: string;
  /** Tipo: comando ou saída */
  type: 'command' | 'output';
  /** Entry original */
  entry: HistoryEntry;
}

export const TerminalHistory = forwardRef<HTMLDivElement, TerminalHistoryProps>(
  function TerminalHistory({ entries, runningCommandId: _runningCommandId, isLoading, onReachEnd }, ref) {
    const { t } = useTranslation();
    const containerRef = useRef<HTMLDivElement>(null);
    const bottomRef = useRef<HTMLDivElement>(null);
    const nodeRefs = useRef<Map<string, HTMLDivElement>>(new Map());

    // Encaminha o ref para o container
    useImperativeHandle(ref, () => containerRef.current!, []);

    // Monta lista plana de nós a partir dos entries
    const flatNodes = useMemo<FlatNode[]>(() => {
      const nodes: FlatNode[] = [];
      for (const entry of entries) {
        // Sempre adiciona nó de comando
        nodes.push({
          nodeId: `cmd-${entry.id}`,
          type: 'command',
          entry,
        });
        // Adiciona nó de saída se tem output ou está em execução (raw/marker)
        if (entry.output || entry.exitCode === -999) {
          nodes.push({
            nodeId: `out-${entry.id}`,
            type: 'output',
            entry,
          });
        }
      }
      return nodes;
    }, [entries]);

    // Auto-scroll para o final quando novo conteúdo chega
    const lastNode = flatNodes[flatNodes.length - 1];
    const scrollKey = lastNode ? `${lastNode.nodeId}-${lastNode.entry.output?.length ?? 0}` : '';

    useEffect(() => {
      if (bottomRef.current && containerRef.current) {
        const container = containerRef.current;
        const isNearBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 150;
        if (isNearBottom) {
          bottomRef.current.scrollIntoView({ behavior: 'smooth' });
        }
      }
    }, [scrollKey]);

    // Anuncia novo comando quando aparece
    const prevEntryCount = useRef(entries.length);
    useEffect(() => {
      if (entries.length > prevEntryCount.current) {
        const lastEntry = entries[entries.length - 1];
        if (lastEntry) {
          announce(`${t('terminal.history.commandPrefix')} ${lastEntry.command}`);
        }
      }
      prevEntryCount.current = entries.length;
    }, [entries, t]);

    /** Registra/remove ref de nó */
    const setNodeRef = useCallback((nodeId: string, el: HTMLDivElement | null) => {
      if (el) {
        nodeRefs.current.set(nodeId, el);
      } else {
        nodeRefs.current.delete(nodeId);
      }
    }, []);

    /** Foca um nó pelo índice na lista plana */
    const focusNode = useCallback((index: number) => {
      const target = flatNodes[index];
      if (!target) return;
      const el = nodeRefs.current.get(target.nodeId);
      if (el) {
        el.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        el.focus();
      }
    }, [flatNodes]);

    if (isLoading) {
      return (
        <div className="terminal-history terminal-history--loading" role="region" aria-label={t('terminal.history.label')}>
          <p className="terminal-history__loading-text">{t('terminal.history.loading')}</p>
        </div>
      );
    }

    if (entries.length === 0) {
      return (
        <div ref={containerRef} className="terminal-history terminal-history--empty" role="region" aria-label={t('terminal.history.label')}>
          <div className="terminal-history__empty-state">
            <span className="terminal-history__empty-icon" aria-hidden="true">&gt;_</span>
            <p className="terminal-history__empty-text">
              {t('terminal.history.emptyTitle')}
              <br />
              {t('terminal.history.emptyHint')}
            </p>
          </div>
        </div>
      );
    }

    return (
      <div className="terminal-history" ref={containerRef} role="list" aria-label={t('terminal.history.label')}>
        {flatNodes.map((node, index) => {
          const navProps = {
            onNavigatePrev: index > 0 ? () => focusNode(index - 1) : undefined,
            onNavigateNext: index < flatNodes.length - 1 ? () => focusNode(index + 1) : onReachEnd,
            onNavigateFirst: () => focusNode(0),
            onNavigateLast: index < flatNodes.length - 1
              ? () => focusNode(flatNodes.length - 1)
              : onReachEnd,
            onReachEnd,
          };

          if (node.type === 'command') {
            return (
              <TerminalCommandNode
                key={node.nodeId}
                ref={(el) => setNodeRef(node.nodeId, el)}
                entry={node.entry}
                {...navProps}
              />
            );
          } else {
            return (
              <TerminalOutputNode
                key={node.nodeId}
                ref={(el) => setNodeRef(node.nodeId, el)}
                entry={node.entry}
                {...navProps}
              />
            );
          }
        })}
        <div ref={bottomRef} />
      </div>
    );
  }
);
