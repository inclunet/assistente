import { useState, useCallback, useRef, useMemo, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useAnnouncer } from '../../../hooks/useAnnouncer';
import './OutputExplorer.css';

interface OutputExplorerProps {
  data: Record<string, unknown> | null;
  onSelectPath?: (path: string) => void;
  autoFocus?: boolean;
  highlightArrays?: boolean;
}

interface FlatNode {
  path: string;
  name: string;
  value: unknown;
  depth: number;
  isExpandable: boolean;
  isArray: boolean;
  isObject: boolean;
  childCount: number;
  setSize: number;
  posInSet: number;
  typeLabel: string;
}

function computeTypeLabel(value: unknown, isArray: boolean, isObject: boolean): string {
  if (isArray) return `[${(value as unknown[]).length}]`;
  if (isObject) return `{${Object.keys(value as Record<string, unknown>).length}}`;
  if (typeof value === 'string') {
    const s = value as string;
    return `"${s.length > 60 ? s.slice(0, 60) + '…' : s}"`;
  }
  return String(value);
}

function flattenData(
  data: Record<string, unknown>,
  expandedSet: Set<string>,
): FlatNode[] {
  const result: FlatNode[] = [];

  function walk(entries: [string, unknown][], parentPath: string, depth: number) {
    for (let i = 0; i < entries.length; i++) {
      const [name, value] = entries[i];
      const path = parentPath ? `${parentPath}.${name}` : name;
      const isObject = value !== null && typeof value === 'object' && !Array.isArray(value);
      const isArray = Array.isArray(value);
      const isExpandable = isObject || isArray;

      const childEntries = isObject
        ? Object.entries(value as Record<string, unknown>)
        : isArray
          ? (value as unknown[]).map((v, idx) => [String(idx), v] as [string, unknown])
          : [];

      result.push({
        path,
        name,
        value,
        depth,
        isExpandable,
        isArray,
        isObject,
        childCount: childEntries.length,
        setSize: entries.length,
        posInSet: i + 1,
        typeLabel: computeTypeLabel(value, isArray, isObject),
      });

      if (isExpandable && expandedSet.has(path)) {
        walk(childEntries, path, depth + 1);
      }
    }
  }

  walk(Object.entries(data), '', 0);
  return result;
}

function buildInitialExpanded(data: Record<string, unknown>): Set<string> {
  const set = new Set<string>();
  function walk(entries: [string, unknown][], parentPath: string, depth: number) {
    if (depth >= 2) return;
    for (const [name, value] of entries) {
      const path = parentPath ? `${parentPath}.${name}` : name;
      const isObj = value !== null && typeof value === 'object' && !Array.isArray(value);
      const isArr = Array.isArray(value);
      if (isObj || isArr) {
        set.add(path);
        const children = isObj
          ? Object.entries(value as Record<string, unknown>)
          : (value as unknown[]).map((v, i) => [String(i), v] as [string, unknown]);
        walk(children, path, depth + 1);
      }
    }
  }
  walk(Object.entries(data), '', 0);
  return set;
}

export function OutputExplorer({ data, onSelectPath, autoFocus = false, highlightArrays = false }: OutputExplorerProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();

  const [expandedSet, setExpandedSet] = useState<Set<string>>(() =>
    data ? buildInitialExpanded(data) : new Set(),
  );
  const [focusedPath, setFocusedPath] = useState<string | null>(null);
  const [copiedPath, setCopiedPath] = useState<string | null>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const copiedTimerRef = useRef<ReturnType<typeof setTimeout>>();

  useEffect(() => {
    if (data) {
      const expanded = buildInitialExpanded(data);
      setExpandedSet(expanded);
      setCopiedPath(null);
      if (autoFocus) {
        const entries = Object.entries(data);
        if (entries.length > 0) {
          setFocusedPath(entries[0][0]);
        }
      } else {
        setFocusedPath(null);
      }
    }
  }, [data, autoFocus]);

  const flatNodes = useMemo(
    () => (data ? flattenData(data, expandedSet) : []),
    [data, expandedSet],
  );

  const pathToIndex = useMemo(() => {
    const map = new Map<string, number>();
    flatNodes.forEach((n, i) => map.set(n.path, i));
    return map;
  }, [flatNodes]);

  const focusedIndex = focusedPath !== null ? (pathToIndex.get(focusedPath) ?? 0) : -1;

  useEffect(() => {
    if (focusedPath === null) return;
    const idx = pathToIndex.get(focusedPath);
    if (idx === undefined) return;
    const el = listRef.current?.querySelector<HTMLLIElement>(`[data-path="${CSS.escape(focusedPath)}"]`);
    el?.focus();
  }, [focusedPath, pathToIndex]);

  const toggleExpand = useCallback((path: string) => {
    setExpandedSet((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
        for (const p of prev) {
          if (p.startsWith(path + '.')) next.delete(p);
        }
      } else {
        next.add(path);
      }
      return next;
    });
  }, []);

  const getParentPath = useCallback((nodePath: string): string | null => {
    const dot = nodePath.lastIndexOf('.');
    if (dot < 0) return null;
    return nodePath.substring(0, dot);
  }, []);

  const onSelectPathRef = useRef(onSelectPath);
  onSelectPathRef.current = onSelectPath;

  const copyTemplatePath = useCallback((path: string) => {
    const template = `{{ .output.${path} }}`;
    navigator.clipboard.writeText(template).then(() => {
      setCopiedPath(path);
      announce(t('jobs.builder.treeCopiedTemplate', { path: template }));
      clearTimeout(copiedTimerRef.current);
      copiedTimerRef.current = setTimeout(() => setCopiedPath(null), 1500);
    }).catch(() => {
      // fallback: noop
    });
  }, [announce, t]);

  const activateNode = useCallback((node: FlatNode) => {
    if (onSelectPathRef.current) {
      onSelectPathRef.current(node.path);
      return;
    }
    if (!node.isExpandable) {
      copyTemplatePath(node.path);
    }
  }, [copyTemplatePath]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLLIElement>) => {
    const path = e.currentTarget.dataset.path;
    if (!path) return;

    const idx = pathToIndex.get(path);
    if (idx === undefined) return;
    const node = flatNodes[idx];
    if (!node) return;

    let handled = true;
    switch (e.key) {
      case 'ArrowDown':
        if (idx < flatNodes.length - 1) setFocusedPath(flatNodes[idx + 1].path);
        break;
      case 'ArrowUp':
        if (idx > 0) setFocusedPath(flatNodes[idx - 1].path);
        break;
      case 'ArrowRight':
        if (node.isExpandable && !expandedSet.has(node.path)) {
          toggleExpand(node.path);
        } else if (node.isExpandable && expandedSet.has(node.path) && node.childCount > 0) {
          const nextIdx = idx + 1;
          if (nextIdx < flatNodes.length) setFocusedPath(flatNodes[nextIdx].path);
        }
        break;
      case 'ArrowLeft':
        if (node.isExpandable && expandedSet.has(node.path)) {
          toggleExpand(node.path);
        } else {
          const parent = getParentPath(node.path);
          if (parent && pathToIndex.has(parent)) setFocusedPath(parent);
        }
        break;
      case 'Home':
        if (flatNodes.length > 0) setFocusedPath(flatNodes[0].path);
        break;
      case 'End':
        if (flatNodes.length > 0) setFocusedPath(flatNodes[flatNodes.length - 1].path);
        break;
      case 'Enter':
        activateNode(node);
        break;
      case ' ':
        if (node.isExpandable) {
          toggleExpand(node.path);
        } else {
          activateNode(node);
        }
        break;
      case 'c':
        if (e.ctrlKey || e.metaKey) {
          copyTemplatePath(node.path);
        } else {
          handled = false;
        }
        break;
      default: {
        if (e.key.length === 1 && !e.ctrlKey && !e.altKey && !e.metaKey) {
          const char = e.key.toLowerCase();
          for (let offset = 1; offset < flatNodes.length; offset++) {
            const candidate = flatNodes[(idx + offset) % flatNodes.length];
            if (candidate.name.toLowerCase().startsWith(char)) {
              setFocusedPath(candidate.path);
              break;
            }
          }
        } else {
          handled = false;
        }
        break;
      }
    }

    if (handled) {
      e.preventDefault();
      e.stopPropagation();
    }
  }, [flatNodes, expandedSet, pathToIndex, toggleExpand, getParentPath, activateNode, copyTemplatePath]);

  const handleClick = useCallback((node: FlatNode) => {
    setFocusedPath(node.path);
    if (node.isExpandable) {
      toggleExpand(node.path);
    }
    activateNode(node);
  }, [toggleExpand, activateNode]);

  useEffect(() => {
    return () => clearTimeout(copiedTimerRef.current);
  }, []);

  if (!data || Object.keys(data).length === 0) {
    return (
      <div className="output-explorer__empty">
        {t('jobs.builder.noOutput')}
      </div>
    );
  }

  const hasAction = Boolean(onSelectPath);

  return (
    <div className="output-explorer__wrapper">
      {!hasAction && (
        <p className="output-explorer__hint">
          {t('jobs.builder.treeClickToCopy')}
        </p>
      )}
      <ul
        ref={listRef}
        className="output-explorer"
        role="tree"
        aria-label={t('jobs.builder.outputTree')}
      >
        {flatNodes.map((node, index) => {
          const isExpanded = expandedSet.has(node.path);
          const isFocused = index === focusedIndex;
          const isCopied = copiedPath === node.path;

          const accessibleLabel = node.isExpandable
            ? `${node.name}, ${node.isArray
                ? t('jobs.builder.treeArrayLabel', { count: node.childCount })
                : t('jobs.builder.treeObjectLabel', { count: node.childCount })
              }, ${isExpanded ? t('jobs.builder.treeExpanded') : t('jobs.builder.treeCollapsed')}`
            : `${node.name}: ${node.typeLabel}`;

          return (
            <li
              key={node.path}
              data-path={node.path}
              role="treeitem"
              aria-level={node.depth + 1}
              aria-setsize={node.setSize}
              aria-posinset={node.posInSet}
              aria-expanded={node.isExpandable ? isExpanded : undefined}
              aria-label={accessibleLabel}
              tabIndex={isFocused || (focusedIndex === -1 && index === 0) ? 0 : -1}
              className={`tree-node${isFocused ? ' tree-node--focused' : ''}${!node.isExpandable ? ' tree-node--leaf' : ''}${isCopied ? ' tree-node--copied' : ''}${highlightArrays && node.isArray ? ' tree-node--array-highlight' : ''}${highlightArrays && !node.isArray && !node.isObject ? ' tree-node--dimmed' : ''}`}
              style={{ '--depth': node.depth } as React.CSSProperties}
              onClick={() => handleClick(node)}
              onKeyDown={handleKeyDown}
            >
              {node.isExpandable && (
                <span className="tree-node__arrow" aria-hidden="true">
                  {isExpanded ? '▼' : '▶'}
                </span>
              )}
              {!node.isExpandable && (
                <span className="tree-node__arrow-spacer" aria-hidden="true" />
              )}
              <span className="tree-node__key" aria-hidden="true">{node.name}</span>
              {!node.isExpandable && <span className="tree-node__sep" aria-hidden="true">:</span>}
              <span
                className={`tree-node__value tree-node__value--${typeof node.value}`}
                aria-hidden="true"
              >
                {node.typeLabel}
              </span>
              <span className="tree-node__action" aria-hidden="true">
                {isCopied
                  ? t('jobs.builder.treeCopied')
                  : hasAction ? '⎘' : !node.isExpandable ? '📋' : ''}
              </span>
            </li>
          );
        })}
      </ul>
      {copiedPath && (
        <div className="output-explorer__toast">
          {t('jobs.builder.treeCopiedTemplate', { path: `{{ .output.${copiedPath} }}` })}
        </div>
      )}
    </div>
  );
}
