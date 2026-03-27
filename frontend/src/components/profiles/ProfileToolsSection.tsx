import { useCallback, useMemo, useState } from 'react';
import { FilterOutlined } from '@ant-design/icons';
import { main, allowlist } from '@wailsjs/go/models';
import { useTranslation } from 'react-i18next';
import { useMCPStore } from '../../store/mcpStore';
import { CollapsibleSection } from '../ui/CollapsibleSection';
import { DataGrid, DataGridColumn } from '../ui/DataGrid';
import { RangeSlider } from '../ui/RangeSlider';
import { Combobox, type ComboboxItem } from '../pickers/Combobox';
import { useToolbarKeyboardNav } from '../../hooks/useToolbarKeyboardNav';
import { parseToolSource, extractMcpServers } from '../../utils/toolSource';

export type ToolFilter = 'all' | 'local' | 'mcp' | `mcp:${string}`;

export interface ProfileToolsSectionProps {
  availableTools: main.ToolInfo[];
  enabledTools?: string[] | null;
  toolsDisabled?: boolean;
  commandAllowlist?: string;
  availableAllowlists: allowlist.AllowlistInfo[];
  maxAgenticIterations?: number;
  responseTimeout?: number;
  onChange: (
    field: 'enabled_tools' | 'command_allowlist' | 'disable_tools' | 'max_agentic_iterations' | 'response_timeout',
    value: string[] | string | boolean | number | null
  ) => void;
  disabled?: boolean;
}

interface ToolRow {
  id: string;
  name: string;
  description: string;
}

export function ProfileToolsSection({
  availableTools,
  enabledTools = null,
  toolsDisabled = false,
  commandAllowlist = '',
  availableAllowlists = [],
  maxAgenticIterations = 0,
  responseTimeout = 180,
  onChange,
  disabled = false,
}: ProfileToolsSectionProps) {
  const { t } = useTranslation();
  const mcpServers = useMCPStore((s) => s.servers);

  const [filter, setFilter] = useState<ToolFilter>('all');
  const [search, setSearch] = useState('');

  const allNames = availableTools.map(tool => tool.name);

  const mcpServerEntries = useMemo(
    () => extractMcpServers(allNames, mcpServers.map((s) => ({ slug: s.slug, name: s.name }))),
    [allNames, mcpServers],
  );

  const filterItems: ComboboxItem[] = useMemo(() => [
    { value: 'all', label: t('profiles.filterAll', 'Todas') },
    { value: 'local', label: t('profiles.filterLocal', 'Locais') },
    { value: 'mcp', label: t('profiles.filterAllMcp', 'Todas MCP') },
    ...mcpServerEntries.map((srv) => ({ value: `mcp:${srv.slug}`, label: srv.name })),
  ], [t, mcpServerEntries]);

  const toolRows: ToolRow[] = useMemo(
    () => availableTools.map(tool => ({ id: tool.name, name: tool.name, description: tool.description || '' })),
    [availableTools],
  );

  const filteredRows = useMemo(() => {
    const term = search.toLowerCase().trim();
    return toolRows.filter((row) => {
      if (filter !== 'all') {
        const src = parseToolSource(row.name);
        if (filter === 'local' && src.type !== 'local') return false;
        if (filter === 'mcp' && src.type !== 'mcp') return false;
        if (filter.startsWith('mcp:') && (src.type !== 'mcp' || src.serverSlug !== filter.slice(4))) return false;
      }
      if (term && !row.name.toLowerCase().includes(term) && !row.description.toLowerCase().includes(term)) return false;
      return true;
    });
  }, [toolRows, filter, search]);

  const filteredNames = useMemo(() => new Set(filteredRows.map((r) => r.name)), [filteredRows]);
  const isFiltered = filter !== 'all' || search.trim() !== '';

  const selectedIds: Set<string | number> = !enabledTools
    ? new Set<string | number>(allNames)
    : new Set<string | number>(enabledTools);

  const handleSelectionChange = useCallback((newSelectedIds: Set<string | number>) => {
    if (newSelectedIds.size === allNames.length) {
      onChange('enabled_tools', null);
    } else if (newSelectedIds.size === 0) {
      onChange('enabled_tools', []);
    } else {
      onChange('enabled_tools', Array.from(newSelectedIds) as string[]);
    }
  }, [allNames.length, onChange]);

  const handleSelectFiltered = useCallback(() => {
    if (!isFiltered) {
      onChange('enabled_tools', null);
      return;
    }
    const current = new Set<string>(enabledTools ?? allNames);
    for (const name of filteredNames) current.add(name);
    if (current.size === allNames.length) {
      onChange('enabled_tools', null);
    } else {
      onChange('enabled_tools', Array.from(current));
    }
  }, [isFiltered, enabledTools, allNames, filteredNames, onChange]);

  const handleDeselectFiltered = useCallback(() => {
    if (!isFiltered) {
      onChange('enabled_tools', []);
      return;
    }
    const current = new Set<string>(enabledTools ?? allNames);
    for (const name of filteredNames) current.delete(name);
    if (current.size === 0) {
      onChange('enabled_tools', []);
    } else if (current.size === allNames.length) {
      onChange('enabled_tools', null);
    } else {
      onChange('enabled_tools', Array.from(current));
    }
  }, [isFiltered, enabledTools, allNames, filteredNames, onChange]);

  const allFilteredSelected = [...filteredNames].every((n) => selectedIds.has(n));
  const noneFilteredSelected = [...filteredNames].every((n) => !selectedIds.has(n));
  const showSelectAll = !allFilteredSelected;
  const showDeselectAll = !noneFilteredSelected;

  const isToolEnabled = useCallback((name: string) => {
    return !enabledTools || enabledTools.includes(name);
  }, [enabledTools]);

  const toolbarRef = useToolbarKeyboardNav();

  const columns: DataGridColumn<ToolRow>[] = [
    {
      key: 'checked',
      label: '',
      width: '40px',
      format: (_value: unknown, item: ToolRow) => {
        const checked = isToolEnabled(item.name);
        return (
          <input
            type="checkbox"
            checked={checked}
            readOnly
            tabIndex={-1}
            aria-label={checked
              ? t('profiles.toolEnabled', `${item.name} ativada`)
              : t('profiles.toolDisabled', `${item.name} desativada`)}
            style={{ pointerEvents: 'none' }}
          />
        );
      },
    },
    {
      key: 'name',
      label: t('profiles.toolColName', 'Nome'),
      width: '30%',
    },
    {
      key: 'description',
      label: t('profiles.toolColDesc', 'Descrição'),
      truncate: true,
    },
  ];

  return (
    <CollapsibleSection
      title={t('profiles.collapseTools', 'Ferramentas (Tool Calling)')}
      isOpen={!toolsDisabled}
      onToggle={() => onChange('disable_tools', !toolsDisabled)}
      disabled={disabled}
      badge={toolsDisabled ? 'off' : 'on'}
    >
      {availableTools.length > 0 ? (
        <>
          <p className="profiles-field__hint">
            {t('profiles.toolsHint', 'Selecione quais ferramentas este perfil pode usar. Nenhuma seleção = todas habilitadas.')}
          </p>
          <input
            type="text"
            className="profiles-field__filter-search"
            placeholder={t('profiles.toolsSearchPlaceholder', 'Buscar ferramenta…')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            aria-label={t('profiles.toolsSearchLabel', 'Filtrar ferramentas por nome')}
            data-testid="tools-search"
          />
          <div
            ref={toolbarRef}
            className="profiles-field__tools-actions"
            role="toolbar"
            aria-label={t('profiles.toolsActionsLabel', 'Ações de seleção de ferramentas')}
            data-testid="tools-toolbar"
          >
            <div data-testid="tools-filter">
              <Combobox
                items={filterItems}
                selected={filter}
                onSelect={(value) => setFilter(value as ToolFilter)}
                label={t('profiles.toolsFilterLabel', 'Filtrar por origem')}
                icon={<FilterOutlined />}
                maxWidth="200px"
                disabled={disabled}
              />
            </div>
            {showSelectAll && (
              <button
                type="button"
                className="profiles-field__tools-toggle"
                onClick={handleSelectFiltered}
                disabled={disabled}
                data-testid="tools-select-all"
              >
                {t('profiles.toolsSelectAll', 'Selecionar todas')}
              </button>
            )}
            {showDeselectAll && (
              <button
                type="button"
                className="profiles-field__tools-toggle"
                onClick={handleDeselectFiltered}
                disabled={disabled}
                data-testid="tools-deselect-all"
              >
                {t('profiles.toolsDeselectAll', 'Desmarcar todas')}
              </button>
            )}
          </div>
          {filteredRows.length > 0 ? (
            <DataGrid<ToolRow>
              items={filteredRows}
              columns={columns}
              label={t('profiles.toolsGridLabel', 'Lista de ferramentas disponíveis')}
              getItemId={(item) => item.name}
              selectedIds={selectedIds}
              selectionMode="checkbox"
              onSelectionChange={handleSelectionChange}
              showHeader={true}
              autoFocusOnMount={false}
              className="profiles-tools-datagrid"
            />
          ) : (
            <p className="profiles-field__hint profiles-field__no-results">
              {t('profiles.toolsNoResults', 'Nenhuma ferramenta corresponde ao filtro.')}
            </p>
          )}

          <div className="profiles-field">
            <RangeSlider
              id="agentic-max-iterations"
              label={t('profiles.agenticMaxIterations', 'Máximo de Iterações')}
              value={maxAgenticIterations}
              min={0}
              max={1000}
              step={1}
              onChange={(value) => onChange('max_agentic_iterations', value)}
              formatValue={(value) => {
                if (value === 0) return t('profiles.agenticIterationsDefault', 'Padrão (25)');
                if (value <= 25) return `${value} (Conversacional)`;
                if (value <= 100) return `${value} (Moderado)`;
                return `${value} (Agressivo)`;
              }}
              disabled={disabled}
            />
            <span className="profiles-field__hint">
              {maxAgenticIterations === 0
                ? t('profiles.agenticIterationsDefault', 'Usar padrão de 25 iterações')
                : maxAgenticIterations <= 25
                  ? t('profiles.agenticIterationsConversational', 'Modo conversacional - limite baixo')
                  : maxAgenticIterations <= 100
                    ? t('profiles.agenticIterationsModerate', 'Modo moderado - respostas mais detalhadas')
                    : t('profiles.agenticIterationsAggressive', 'Modo agressivo - análise profunda')}
            </span>
          </div>

          <div className="profiles-field">
            <label htmlFor="pf-response-timeout" className="profiles-field__label">
              {t('profiles.responseTimeout', 'Timeout de Resposta (segundos)')}
            </label>
            <input
              id="pf-response-timeout"
              type="number"
              className="profiles-field__input"
              min={5}
              max={600}
              value={responseTimeout}
              onChange={(e) => onChange('response_timeout', parseInt(e.target.value) || 180)}
              disabled={disabled}
              data-testid="response-timeout-input"
            />
            <span className="profiles-field__hint">
              {t('profiles.responseTimeoutHint', '2ª camada de proteção contra loops. Respostas acima desse tempo são interrompidas.')}
            </span>
          </div>

          <div className="profiles-field">
            <label htmlFor="pf-command-allowlist" className="profiles-field__label">
              {t('profiles.fieldCommandAllowlist', 'Allowlist de Comandos')}
            </label>
            <select
              id="pf-command-allowlist"
              className="profiles-field__select"
              value={commandAllowlist}
              onChange={(e) => onChange('command_allowlist', e.target.value || '')}
              disabled={disabled}
              data-testid="allowlist-select"
            >
              <option value="">{t('profiles.allowlistDefault', 'Padrão')}</option>
              {availableAllowlists.map((al) => (
                <option key={al.slug} value={al.slug}>
                  {al.name} ({al.ruleCount} regras)
                </option>
              ))}
            </select>
            <span className="profiles-field__hint">
              {t('profiles.allowlistHint', 'Define quais comandos shell são executados automaticamente, bloqueados ou pedem confirmação.')}
            </span>
          </div>
        </>
      ) : (
        <p className="profiles-field__hint" style={{ margin: 0 }}>
          {t('profiles.noToolsAvailable', 'Nenhuma ferramenta encontrada.')}
        </p>
      )}
    </CollapsibleSection>
  );
}
