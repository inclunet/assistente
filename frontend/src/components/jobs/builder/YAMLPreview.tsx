import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { JobDraft } from './JobBuilder';
import './YAMLPreview.css';

interface YAMLPreviewProps {
  draft: JobDraft;
}

function toYAML(obj: unknown, indent = 0): string {
  const pad = '  '.repeat(indent);

  if (obj === null || obj === undefined) return '';
  if (typeof obj === 'string') {
    if (obj.includes('{{') || obj.includes('\n') || obj.includes(':') || obj.includes('#')) {
      return `"${obj.replace(/"/g, '\\"')}"`;
    }
    return obj;
  }
  if (typeof obj === 'number' || typeof obj === 'boolean') return String(obj);

  if (Array.isArray(obj)) {
    if (obj.length === 0) return '[]';
    if (obj.every((v) => typeof v === 'string' || typeof v === 'number')) {
      return obj.map((v) => `\n${pad}- ${toYAML(v, indent + 1)}`).join('');
    }
    return obj
      .map((v) => {
        const inner = toYAML(v, indent + 1);
        if (typeof v === 'object' && v !== null) {
          const lines = inner.split('\n').filter(Boolean);
          return `\n${pad}- ${lines[0].trimStart()}${lines.slice(1).map((l) => `\n${pad}  ${l.trimStart()}`).join('')}`;
        }
        return `\n${pad}- ${inner}`;
      })
      .join('');
  }

  if (typeof obj === 'object') {
    const entries = Object.entries(obj as Record<string, unknown>).filter(
      ([, v]) => v !== undefined && v !== null && v !== '' && !(Array.isArray(v) && v.length === 0)
    );
    if (entries.length === 0) return '{}';
    return entries
      .map(([k, v]) => {
        const val = toYAML(v, indent + 1);
        if (typeof v === 'object' && v !== null && !Array.isArray(v)) {
          return `${pad}${k}:${val.startsWith('\n') ? val : `\n${val}`}`;
        }
        if (Array.isArray(v) && v.length > 0 && typeof v[0] === 'object') {
          return `${pad}${k}:${val}`;
        }
        if (Array.isArray(v)) {
          return `${pad}${k}:${val}`;
        }
        return `${pad}${k}: ${val}`;
      })
      .join('\n');
  }

  return String(obj);
}

function buildYAML(draft: JobDraft): string {
  const doc: Record<string, unknown> = {
    id: draft.id,
    name: draft.name,
    description: draft.description,
    enabled: draft.enabled,
  };

  if (draft.pipeline) doc.pipeline = draft.pipeline;
  if (draft.tags && draft.tags.length > 0) doc.tags = draft.tags;

  doc.triggers = draft.triggers;
  doc.tool = draft.tool;

  if (draft.inputs && Object.keys(draft.inputs).length > 0) {
    doc.inputs = draft.inputs;
  }

  const hasSuccessEvent = draft.events.emit_success && draft.events.on_success;
  const hasFailureEvent = draft.events.emit_failure && draft.events.on_failure;

  if (hasSuccessEvent || hasFailureEvent) {
    const events: Record<string, string> = {};
    if (hasSuccessEvent) events.on_success = draft.events.on_success;
    if (hasFailureEvent) events.on_failure = draft.events.on_failure;
    if (hasSuccessEvent && draft.events.mode === 'fanout' && draft.events.for_each) {
      events.for_each = draft.events.for_each;
    }
    if (hasSuccessEvent && draft.events.emit_when) {
      events.emit_when = draft.events.emit_when;
    }
    if (hasSuccessEvent && draft.events.payload_template) {
      events.payload_template = draft.events.payload_template;
    }
    doc.events = events;
  }

  if (draft.error_policy.strategy && draft.error_policy.strategy !== 'stop') {
    doc.error_policy = draft.error_policy;
  }

  return toYAML(doc);
}

export function YAMLPreview({ draft }: YAMLPreviewProps) {
  const { t } = useTranslation();

  const yaml = useMemo(() => buildYAML(draft), [draft]);

  return (
    <div className="yaml-preview">
      <div className="yaml-preview__header">
        <span className="yaml-preview__title">{t('jobs.builder.yamlPreview')}</span>
        <span className="yaml-preview__filename">{draft.id || 'untitled'}.yaml</span>
      </div>
      <pre className="yaml-preview__code" tabIndex={0} aria-label={t('jobs.builder.yamlPreview')}>
        <code>{yaml}</code>
      </pre>
    </div>
  );
}
