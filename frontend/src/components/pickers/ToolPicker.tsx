import { useState, useEffect, forwardRef, useImperativeHandle, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { ComboboxItem } from './Combobox';
import { BasePicker } from './BasePicker';
import { useJobStore } from '../../store/jobStore';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { jobs } from '@wailsjs/go/models';

export interface ToolPickerProps {
  onChange?: (toolName: string, entry: jobs.CatalogEntry | null) => void;
  variant?: 'toolbar' | 'form';
  label?: string;
  description?: string;
  icon?: string;
  maxWidth?: string;
  onAnnounce?: (message: string) => void;
  onAfterSelect?: () => void;
  value?: string;
}

export interface ToolPickerRef {
  reload: () => Promise<void>;
}

function rawMessageToString(raw: unknown): string | undefined {
  if (raw == null) return undefined;
  if (typeof raw === 'string') return raw;

  // Wails mapeia Go json.RawMessage como number[] (bytes)
  if (Array.isArray(raw) && raw.every((v) => typeof v === 'number')) {
    if (typeof TextDecoder !== 'undefined') {
      return new TextDecoder().decode(Uint8Array.from(raw));
    }
    return String.fromCharCode(...raw);
  }

  if (raw instanceof Uint8Array) {
    if (typeof TextDecoder !== 'undefined') {
      return new TextDecoder().decode(raw);
    }
    return String.fromCharCode(...Array.from(raw));
  }

  // Fallback: preserva a forma serializável
  return JSON.stringify(raw);
}

function normalizeCatalogEntry(entry: jobs.CatalogEntry): jobs.CatalogEntry {
  const normalized = Object.assign(Object.create(Object.getPrototypeOf(entry)), entry) as jobs.CatalogEntry;
  const schema = rawMessageToString((entry as unknown as { schema?: unknown }).schema);
  if (schema !== undefined) {
    (normalized as unknown as { schema?: unknown }).schema = schema;
  }
  return normalized;
}

function localizedUnavailableReason(
  reason: string | undefined,
  t: (key: string, options?: Record<string, unknown>) => string,
): string {
  const normalized = reason?.trim() ?? '';
  const deletedServer = normalized.match(/^MCP server "([^"]+)" was deleted$/);
  if (deletedServer) {
    return t('jobs.builder.toolUnavailableServerDeleted', { server: deletedServer[1] });
  }
  if (normalized === 'server disconnected') {
    return t('jobs.builder.toolUnavailableServerDisconnected');
  }
  if (normalized === 'not discovered' || normalized === 'not discovered yet') {
    return t('jobs.builder.toolUnavailableNotDiscovered');
  }
  return t('jobs.builder.toolUnavailable');
}

export const ToolPicker = forwardRef<ToolPickerRef, ToolPickerProps>(
  (
    {
      onChange,
      variant = 'form',
      label,
      description,
      icon = '🔧',
      maxWidth,
      onAnnounce,
      onAfterSelect,
      value = '',
    },
    ref
  ) => {
    const { t } = useTranslation();
    const fetchToolCatalog = useJobStore((s) => s.fetchToolCatalog);

    const effectiveLabel = label ?? t('jobs.builder.step_tool');

    const [tools, setTools] = useState<jobs.CatalogEntry[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const loadTools = useCallback(async () => {
      setLoading(true);
      setError(null);
      try {
        const result = await fetchToolCatalog();
        setTools(result);
      } catch (err) {
        setError(err instanceof Error ? err.message : t('common.error'));
      } finally {
        setLoading(false);
      }
    }, [fetchToolCatalog, t]);

    useEffect(() => {
      loadTools();
    }, [loadTools]);

    useEffect(() => {
      const unsub = EventsOn('mcp:tools_changed', () => {
        loadTools();
      });
      return () => { unsub(); };
    }, [loadTools]);

    useImperativeHandle(ref, () => ({
      reload: loadTools,
    }));

    const buildItems = (): ComboboxItem[] => {
      return tools.map((tool) => {
        const unavailable = tool.availability_status === 'unavailable';
        const status = unavailable ? ` ${localizedUnavailableReason(tool.availability_reason, t)}` : '';
        const description = unavailable && tool.description === tool.availability_reason ? '' : tool.description;
        return {
          value: tool.name,
          label: tool.name,
          sublabel: description
            ? `[${tool.source}]${status} ${description}`
            : `[${tool.source}]${status}`,
          disabled: unavailable,
        };
      });
    };

    const handleSelect = (toolName: string) => {
      const entry = tools.find((t) => t.name === toolName) ?? null;
      if (entry?.availability_status === 'unavailable') {
        return;
      }
      onAnnounce?.(t('jobs.builder.selectedTool') + ': ' + toolName);
      onChange?.(toolName, entry ? normalizeCatalogEntry(entry) : null);
    };

    return (
      <BasePicker
        variant={variant}
        items={buildItems()}
        selected={value}
        onSelect={handleSelect}
        label={effectiveLabel}
        description={description}
        icon={icon}
        maxWidth={maxWidth}
        placeholder={t('jobs.builder.searchTools')}
        onAnnounce={onAnnounce}
        loading={loading}
        error={error}
        onRetry={loadTools}
        showFormLabel
        onAfterSelect={onAfterSelect}
      />
    );
  }
);

ToolPicker.displayName = 'ToolPicker';
