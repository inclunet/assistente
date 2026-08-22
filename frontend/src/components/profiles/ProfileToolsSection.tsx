import { useCallback, useMemo, useRef, useState } from 'react';
import { FilterOutlined } from '@ant-design/icons';
import { apidto, allowlist } from '@wailsjs/go/models';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { useMCPStore } from '../../store/mcpStore';
import { type DataGridColumn } from '../ui/DataGrid';
import { RangeSlider } from '../ui/RangeSlider';
import { Combobox, type ComboboxItem } from '../pickers/Combobox';
import { parseToolSource, extractMcpServers } from '../../utils/toolSource';
import { ResourceSelectionSection } from './ResourceSelectionSection';
import { useAnnouncer } from '../../hooks/useAnnouncer';

export type ToolFilter = 'all' | 'local' | 'mcp' | `mcp:${string}`;
type ToolPolicyState = 'disabled' | 'on_demand' | 'preloaded';

const TOOL_POLICY_DISABLED: ToolPolicyState = 'disabled';
const TOOL_POLICY_ON_DEMAND: ToolPolicyState = 'on_demand';
const TOOL_POLICY_PRELOADED: ToolPolicyState = 'preloaded';
const CONTROL_PLANE_TOOLS = new Set(['tool_catalog', 'load_skill']);

export interface ProfileToolsSectionProps {
  availableTools: apidto.ToolInfo[];
  enabledTools?: string[] | null;
  toolPolicy?: Record<string, string> | null;
  toolPolicyDefault?: string | null;
  toolsDisabled?: boolean;
  commandAllowlist?: string;
  availableAllowlists: allowlist.AllowlistInfo[];
  maxAgenticIterations?: number;
  responseTimeout?: number;
  /** Override tri-state de MCP nativo: true=força nativo, false=força adapter, null/undefined=auto. */
  nativeMcp?: boolean | null;
  onChange: (
    field: 'enabled_tools' | 'tool_policy' | 'command_allowlist' | 'disable_tools' | 'max_agentic_iterations' | 'response_timeout' | 'native_mcp',
    value: string[] | string | boolean | number | null | Record<string, string>
  ) => void;
  onPolicyChange?: (policy: Record<string, string>) => void;
  disabled?: boolean;
}

interface ToolRow {
  id: string;
  name: string;
  displayName: string;
  description: string;
  sourceType: string;
  sourceLabel: string;
}

export function ProfileToolsSection({
  availableTools,
  enabledTools = null,
  toolPolicy = null,
  toolPolicyDefault = null,
  toolsDisabled = false,
  commandAllowlist = '',
  availableAllowlists = [],
  maxAgenticIterations = 0,
  responseTimeout = 180,
  nativeMcp = null,
  onChange,
  onPolicyChange,
  disabled = false,
}: ProfileToolsSectionProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const mcpServers = useMCPStore((s) => s.servers);

  const [filter, setFilter] = useState<ToolFilter>('all');
  const [search, setSearch] = useState('');

  const allNames = useMemo(() => availableTools.map(tool => tool.name), [availableTools]);

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
    () => availableTools.map(tool => ({
      id: tool.name,
      name: tool.name,
      displayName: tool.display_name || tool.name,
      description: tool.description || '',
      sourceType: tool.source_type || 'local',
      sourceLabel: tool.source_label || 'Local',
    })),
    [availableTools],
  );

  const filteredRows = useMemo(() => {
    const term = search.toLowerCase().trim();
    return toolRows.filter((row) => {
      if (filter !== 'all') {
        if (filter === 'local' && row.sourceType !== 'local') return false;
        if (filter === 'mcp' && row.sourceType !== 'mcp') return false;
        if (filter.startsWith('mcp:')) {
          const src = parseToolSource(row.name);
          if (src.type !== 'mcp' || src.serverSlug !== filter.slice(4)) return false;
        }
      }
      if (term
        && !row.displayName.toLowerCase().includes(term)
        && !row.name.toLowerCase().includes(term)
        && !row.description.toLowerCase().includes(term)
        && !row.sourceLabel.toLowerCase().includes(term)
      ) return false;
      return true;
    });
  }, [toolRows, filter, search]);

  const filteredNames = useMemo(() => new Set(filteredRows.map((r) => r.name)), [filteredRows]);
  const isFiltered = filter !== 'all' || search.trim() !== '';

  const effectiveToolPolicy = useMemo(() => {
    const policy: Record<string, ToolPolicyState> = {};
    if ((toolPolicy && Object.keys(toolPolicy).length > 0) || toolPolicyDefault != null) {
      const defaultState = toolPolicyDefault === TOOL_POLICY_ON_DEMAND
        ? TOOL_POLICY_ON_DEMAND
        : TOOL_POLICY_DISABLED;
      for (const name of allNames) policy[name] = defaultState;
      for (const [name, state] of Object.entries(toolPolicy ?? {})) {
        if (!allNames.includes(name)) continue;
        policy[name] = normalizeToolPolicyState(state);
      }
      if (
        allNames.includes('tool_catalog')
        && !Object.prototype.hasOwnProperty.call(toolPolicy ?? {}, 'tool_catalog')
        && Object.values(policy).some((state) => state === TOOL_POLICY_ON_DEMAND)
      ) {
        policy.tool_catalog = TOOL_POLICY_PRELOADED;
      }
      return policy;
    }
    if (enabledTools == null) {
      for (const name of allNames) {
        policy[name] = CONTROL_PLANE_TOOLS.has(name) ? TOOL_POLICY_PRELOADED : TOOL_POLICY_ON_DEMAND;
      }
      return policy;
    }
    const enabledSet = new Set(enabledTools);
    for (const name of allNames) {
      policy[name] = enabledSet.has(name) ? TOOL_POLICY_PRELOADED : TOOL_POLICY_DISABLED;
    }
    return policy;
  }, [allNames, enabledTools, toolPolicy, toolPolicyDefault]);
  const effectiveToolPolicyRef = useRef(effectiveToolPolicy);
  effectiveToolPolicyRef.current = effectiveToolPolicy;

  const selectedIds: Set<string | number> = useMemo(
    () => new Set<string | number>(
      allNames.filter((name) => effectiveToolPolicy[name] !== TOOL_POLICY_DISABLED)
    ),
    [allNames, effectiveToolPolicy],
  );

  const commitToolPolicy = useCallback((nextPolicy: Record<string, ToolPolicyState>) => {
    effectiveToolPolicyRef.current = nextPolicy;
    if (onPolicyChange) {
      onPolicyChange(nextPolicy);
      return;
    }
    onChange('tool_policy', nextPolicy);
  }, [onChange, onPolicyChange]);

  const isExplicitlyDisabled = useCallback((name: string) => (
    toolPolicy != null
    && Object.prototype.hasOwnProperty.call(toolPolicy, name)
    && toolPolicy[name] === TOOL_POLICY_DISABLED
  ), [toolPolicy]);

  const setToolsState = useCallback((names: Iterable<string>, state: ToolPolicyState) => {
    const next = { ...effectiveToolPolicy };
    for (const name of names) {
      if (!allNames.includes(name)) continue;
      if (state === TOOL_POLICY_PRELOADED && isExplicitlyDisabled(name)) continue;
      next[name] = state;
    }
    commitToolPolicy(next);
  }, [allNames, commitToolPolicy, effectiveToolPolicy, isExplicitlyDisabled]);

  const handleSelectionChange = useCallback((newSelectedIds: Set<string | number>) => {
    if (newSelectedIds.size === allNames.length) {
      if (selectedIds.size === allNames.length) return;
      setToolsState(allNames, TOOL_POLICY_PRELOADED);
      return;
    }
    const scopeNames = isFiltered ? filteredNames : allNames;
    if (newSelectedIds.size === 0) {
      const next = { ...effectiveToolPolicy };
      for (const name of scopeNames) {
        next[name] = CONTROL_PLANE_TOOLS.has(name) && !isExplicitlyDisabled(name)
          ? TOOL_POLICY_PRELOADED
          : TOOL_POLICY_DISABLED;
      }
      commitToolPolicy(next);
      return;
    }
    const next = { ...effectiveToolPolicy };
    for (const name of scopeNames) {
      next[name] = newSelectedIds.has(name) && !isExplicitlyDisabled(name)
        ? TOOL_POLICY_PRELOADED
        : TOOL_POLICY_DISABLED;
    }
    commitToolPolicy(next);
  }, [allNames, commitToolPolicy, effectiveToolPolicy, filteredNames, isExplicitlyDisabled, isFiltered, selectedIds.size, setToolsState]);

  const handleSelectFiltered = useCallback(() => {
    if (!isFiltered) {
      setToolsState(allNames, TOOL_POLICY_PRELOADED);
      return;
    }
    setToolsState(filteredNames, TOOL_POLICY_PRELOADED);
  }, [isFiltered, allNames, filteredNames, setToolsState]);

  const handleDeselectFiltered = useCallback(() => {
    if (!isFiltered) {
      const next = { ...effectiveToolPolicy };
      for (const name of allNames) {
        next[name] = CONTROL_PLANE_TOOLS.has(name) && !isExplicitlyDisabled(name)
          ? TOOL_POLICY_PRELOADED
          : TOOL_POLICY_DISABLED;
      }
      commitToolPolicy(next);
      return;
    }
    setToolsState(filteredNames, TOOL_POLICY_DISABLED);
  }, [isFiltered, allNames, commitToolPolicy, effectiveToolPolicy, filteredNames, isExplicitlyDisabled, setToolsState]);

  const filteredToolNames = [...filteredNames];
  const allFilteredPreloaded = filteredToolNames.every(
    (name) => effectiveToolPolicy[name] === TOOL_POLICY_PRELOADED,
  );
  const noneFilteredAvailable = filteredToolNames.every(
    (name) => effectiveToolPolicy[name] === TOOL_POLICY_DISABLED,
  );
  const showSelectAll = !allFilteredPreloaded;
  const showDeselectAll = !noneFilteredAvailable;

  const handleToolToggle = useCallback((item: ToolRow) => {
    const currentPolicy = effectiveToolPolicyRef.current;
    const current = currentPolicy[item.name] ?? TOOL_POLICY_DISABLED;
    const nextState = nextToolPolicyState(current);
    commitToolPolicy({ ...currentPolicy, [item.name]: nextState });
    announce(t('profiles.toolPolicyChanged', '{{tool}} agora está {{state}}', {
      tool: item.displayName,
      state: toolPolicyStateLabel(t, nextState).toLowerCase(),
    }));
  }, [announce, commitToolPolicy, t]);

  const columns: DataGridColumn<ToolRow>[] = [
    {
      key: 'checked',
      label: t('profiles.toolColState', 'Estado'),
      width: '130px',
      format: (_value: unknown, item: ToolRow) => {
        const state = effectiveToolPolicy[item.name] ?? TOOL_POLICY_DISABLED;
        const label = item.sourceType === 'mcp'
          ? `${item.displayName} (${item.sourceLabel})`
          : item.displayName;
        return (
          <span
            aria-label={t('profiles.toolStateAria', '{{tool}}: {{state}}', {
              tool: label,
              state: toolPolicyStateLabel(t, state),
            })}
          >
            {toolPolicyStateLabel(t, state)}
          </span>
        );
      },
    },
    {
      key: 'displayName',
      label: t('profiles.toolColName', 'Nome'),
      width: '30%',
    },
    {
      key: 'sourceLabel',
      label: t('profiles.toolColSource', 'Origem'),
      width: '120px',
      format: (value: unknown) => (
        <span className={`tool-source-badge tool-source-badge--${value === 'Local' ? 'local' : 'mcp'}`}>
          {value as string}
        </span>
      ),
    },
    {
      key: 'description',
      label: t('profiles.toolColDesc', 'Descrição'),
      truncate: true,
    },
  ];

  return (
    <ResourceSelectionSection<ToolRow>
      title={t('profiles.collapseTools', 'Ferramentas (Tool Calling)')}
      isOpen={!toolsDisabled}
      onToggle={() => onChange('disable_tools', !toolsDisabled)}
      disabled={disabled}
      badge={toolsDisabled ? 'off' : 'on'}
      hasItems={availableTools.length > 0}
      hint={t('profiles.toolsHint', 'Defina o estado de cada ferramenta: desabilitada, sob demanda ou pré-carregada. Use Espaço para alternar o estado da linha focada.')}
      searchValue={search}
      onSearchChange={setSearch}
      searchPlaceholder={t('profiles.toolsSearchPlaceholder', 'Buscar ferramenta…')}
      searchLabel={t('profiles.toolsSearchLabel', 'Filtrar ferramentas por nome')}
      searchTestId="tools-search"
      toolbarLabel={t('profiles.toolsActionsLabel', 'Ações de seleção de ferramentas')}
      toolbarTestId="tools-toolbar"
      filterNode={(
        <div data-testid="tools-filter">
          <Combobox
            items={filterItems}
            selected={filter}
            onSelect={(value) => setFilter(value as ToolFilter)}
            label={t('profiles.toolsFilterLabel', 'Filtrar por origem')}
            icon={<FilterOutlined aria-hidden="true" />}
            maxWidth="200px"
            disabled={disabled}
          />
        </div>
      )}
      showSelectAll={showSelectAll}
      showDeselectAll={showDeselectAll}
      onSelectFiltered={handleSelectFiltered}
      onDeselectFiltered={handleDeselectFiltered}
      selectAllLabel={t('profiles.toolsSelectAll', 'Pré-carregar filtradas')}
      deselectAllLabel={t('profiles.toolsDeselectAll', 'Desabilitar filtradas')}
      selectAllTestId="tools-select-all"
      deselectAllTestId="tools-deselect-all"
      rows={filteredRows}
      columns={columns}
      gridLabel={t('profiles.toolsGridLabel', 'Lista de ferramentas disponíveis')}
      getItemId={(item) => item.name}
      selectedIds={selectedIds}
      onSelectionChange={handleSelectionChange}
      onItemToggle={handleToolToggle}
      gridClassName="profiles-tools-datagrid"
      noResultsMessage={t('profiles.toolsNoResults', 'Nenhuma ferramenta corresponde ao filtro.')}
      emptyMessage={t('profiles.noToolsAvailable', 'Nenhuma ferramenta encontrada.')}
    >
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

      <div className="profiles-field">
            <label htmlFor="pf-native-mcp" className="profiles-field__label">
              {t('profiles.nativeMcpLabel', 'MCP nativo (Responses/Anthropic)')}
            </label>
            <select
              id="pf-native-mcp"
              className="profiles-field__select"
              value={nativeMcp === true ? 'on' : nativeMcp === false ? 'off' : 'auto'}
              onChange={(e) => {
                const v = e.target.value;
                onChange('native_mcp', v === 'on' ? true : v === 'off' ? false : null);
              }}
              disabled={disabled}
              data-testid="native-mcp-select"
            >
              <option value="auto">{t('profiles.nativeMcpAuto', 'Automático')}</option>
              <option value="on">{t('profiles.nativeMcpOn', 'Forçar nativo')}</option>
              <option value="off">{t('profiles.nativeMcpOff', 'Forçar adapter (function)')}</option>
            </select>
            <span className="profiles-field__hint">
              {t('profiles.nativeMcpHint', "Como os servidores MCP são entregues ao provider. O modo nativo usa o formato do provider (OpenAI Responses → type:mcp; Anthropic → mcp_servers). Automático = tenta MCP nativo e, se o modelo não suportar, o app ajusta este perfil para adapter automaticamente. 'Forçar nativo' só tem efeito em providers fisicamente capazes (Responses/Anthropic).")}
            </span>
      </div>
    </ResourceSelectionSection>
  );
}

function normalizeToolPolicyState(state: string): ToolPolicyState {
  if (state === TOOL_POLICY_ON_DEMAND || state === TOOL_POLICY_PRELOADED) return state;
  return TOOL_POLICY_DISABLED;
}

function nextToolPolicyState(state: ToolPolicyState): ToolPolicyState {
  if (state === TOOL_POLICY_DISABLED) return TOOL_POLICY_ON_DEMAND;
  if (state === TOOL_POLICY_ON_DEMAND) return TOOL_POLICY_PRELOADED;
  return TOOL_POLICY_DISABLED;
}

function toolPolicyStateLabel(t: TFunction, state: ToolPolicyState): string {
  if (state === TOOL_POLICY_PRELOADED) return t('profiles.toolStatePreloaded', 'Pré-carregada');
  if (state === TOOL_POLICY_ON_DEMAND) return t('profiles.toolStateOnDemand', 'Sob demanda');
  return t('profiles.toolStateDisabled', 'Desabilitada');
}
