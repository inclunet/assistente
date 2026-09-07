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
import {
  normalizeToolPolicyMap,
  normalizeToolPolicyState,
  resolveToolPolicy,
  type ToolPolicyState,
} from '../../utils/toolPolicyMatcher';
import { ResourceSelectionSection } from './ResourceSelectionSection';
import { useAnnouncer } from '../../hooks/useAnnouncer';

export type ToolFilter = 'all' | 'local' | 'mcp' | `mcp:${string}`;

const TOOL_POLICY_DISABLED: ToolPolicyState = 'disabled';
const TOOL_POLICY_ON_DEMAND: ToolPolicyState = 'on_demand';
const TOOL_POLICY_PRELOADED: ToolPolicyState = 'preloaded';
const TOOL_CATALOG_NAME = 'tool_catalog';
const CONTROL_PLANE_TOOLS = new Set([TOOL_CATALOG_NAME, 'load_skill']);

// Espelha ensureCatalogForOnDemandTools do backend: sem o catálogo carregado
// nenhuma tool sob demanda pode ser descoberta, então ele sobe para preloaded a
// menos que o perfil o bloqueie de propósito.
function withCatalogForOnDemandTools(
  policy: Record<string, ToolPolicyState>,
  options: { explicitlyDisabled: boolean; hasOnDemandOutsideGrid: boolean },
): Record<string, ToolPolicyState> {
  // Sem tool_catalog na lista o backend degradaria as sob demanda para
  // preloaded, mas isso só acontece em registry sem catálogo — estado que não
  // existe com o editor acessível, e espelhá-lo aqui congelaria a degradação
  // dentro do perfil.
  if (!(TOOL_CATALOG_NAME in policy)) return policy;
  if (policy[TOOL_CATALOG_NAME] === TOOL_POLICY_PRELOADED) return policy;
  if (options.explicitlyDisabled) return policy;
  const needsCatalog = policy[TOOL_CATALOG_NAME] === TOOL_POLICY_ON_DEMAND
    || options.hasOnDemandOutsideGrid
    || Object.entries(policy).some(([name, state]) => (
      name !== TOOL_CATALOG_NAME && state === TOOL_POLICY_ON_DEMAND
    ));
  if (!needsCatalog) return policy;
  return { ...policy, [TOOL_CATALOG_NAME]: TOOL_POLICY_PRELOADED };
}

export interface ProfileToolsSectionProps {
  availableTools: apidto.ToolInfo[];
  enabledTools?: string[] | null;
  toolPolicy?: Record<string, string> | null;
  toolPolicyDefault?: string | null;
  runtimeTools?: string[];
  toolsDisabled?: boolean;
  commandAllowlist?: string;
  availableAllowlists: allowlist.AllowlistInfo[];
  maxAgenticIterations?: number;
  responseTimeout?: number;
  /** Override tri-state de MCP nativo: true=força nativo, false=força adapter, null/undefined=auto. */
  nativeMcp?: boolean | null;
  onChange: (
    field: 'enabled_tools' | 'tool_policy' | 'tool_policy_default' | 'command_allowlist' | 'disable_tools' | 'max_agentic_iterations' | 'response_timeout' | 'native_mcp',
    value: string[] | string | boolean | number | null | Record<string, string>
  ) => void;
  onPolicyChange?: (
    policy: Record<string, string>,
    extras?: { toolPolicyDefault?: string },
  ) => void;
  disabled?: boolean;
}

interface ToolRow {
  id: string;
  name: string;
  displayName: string;
  description: string;
  sourceType: string;
  sourceLabel: string;
  package?: string;
  optIn: boolean;
}

export function ProfileToolsSection({
  availableTools,
  enabledTools = null,
  toolPolicy = null,
  toolPolicyDefault = null,
  runtimeTools = [],
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
      package: tool.package || undefined,
      optIn: tool.opt_in || false,
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

  // O backend apara os nomes antes de aplicar a política. Normalizar aqui uma
  // vez só evita que uma chave com espaços seja aceita lá e ignorada aqui.
  const normalizedToolPolicy = useMemo(
    () => normalizeToolPolicyMap(toolPolicy),
    [toolPolicy],
  );

  const hasExplicitToolPolicy = Object.keys(normalizedToolPolicy).length > 0;

  // O grid só enxerga as tools discoverable, mas o perfil pode configurar tools
  // que não aparecem aqui (text_edit, por exemplo, é opt-in não discoverable e
  // vem preloaded no Editor de Texto). Salvar só o que a tela mostra apagaria
  // essa configuração a cada toggle.
  const policyEntriesOutsideGrid = useMemo(() => {
    const preserved: Record<string, string> = {};
    for (const [name, state] of Object.entries(normalizedToolPolicy)) {
      if (allNames.includes(name)) continue;
      preserved[name] = state;
    }
    if (hasExplicitToolPolicy) return preserved;
    // Salvar a política zera a allowlist legada no perfil. Sem carregar junto os
    // nomes que o grid não mostra, eles sumiriam no primeiro toggle.
    for (const name of enabledTools ?? []) {
      const key = name.trim();
      if (key === '' || allNames.includes(key)) continue;
      preserved[key] = TOOL_POLICY_PRELOADED;
    }
    return preserved;
  }, [allNames, enabledTools, hasExplicitToolPolicy, normalizedToolPolicy]);

  // Uma tool sob demanda que o grid não mostra também precisa do catálogo, e o
  // backend a enxerga ao decidir a promoção. Aqui não dá para separar a opt-in
  // oculta de um nome que saiu do registry (MCP fora do ar, tool removida), e a
  // escolha é assumir que existe: promover o catálogo não concede capability
  // nenhuma, enquanto ignorá-la gravaria tool_catalog desabilitado e mataria a
  // descoberta de uma tool legitimamente configurada.
  const hasOnDemandOutsideGrid = useMemo(
    () => Object.values(policyEntriesOutsideGrid)
      .some((state) => normalizeToolPolicyState(state) === TOOL_POLICY_ON_DEMAND),
    [policyEntriesOutsideGrid],
  );

  const catalogExplicitlyDisabled = isToolExplicitlyDisabled(normalizedToolPolicy, TOOL_CATALOG_NAME);

  // O runtime informa load_skill enquanto o perfil tem skill sob demanda, e o
  // backend a promove sozinho a cada turno. Gravar essa promoção no mapa a
  // congelaria: desligadas as skills, a entrada explícita continuaria expondo
  // uma tool cujas chamadas já não têm o que carregar. Só sai do mapa a
  // promoção que ninguém pediu — o que o perfil declarava e o que o usuário
  // acabou de escolher permanecem.
  const dropRuntimeOnlyPromotions = useCallback((policy: Record<string, ToolPolicyState>) => {
    const declared = new Set(enabledTools?.map((name) => name.trim()) ?? []);
    let next = policy;
    for (const raw of runtimeTools) {
      const name = raw.trim();
      if (name === '' || name in normalizedToolPolicy || declared.has(name)) continue;
      if (next[name] !== TOOL_POLICY_PRELOADED) continue;
      if (next === policy) next = { ...policy };
      delete next[name];
    }
    return next;
  }, [enabledTools, normalizedToolPolicy, runtimeTools]);

  // Nem allowlist, nem mapa, nem default: o perfil ainda é o legado aberto que o
  // backend resolve como catalog-first.
  const isLegacyCatalogFirst = enabledTools == null
    && !hasExplicitToolPolicy
    && (toolPolicyDefault?.trim() ?? '') === '';

  const effectiveToolPolicy = useMemo(() => {
    const policy: Record<string, ToolPolicyState> = {};
    const normalizedDefault = toolPolicyDefault?.trim() ?? '';
    // Espelha o backend: enquanto o perfil descreve as tools pela allowlist
    // legada, o default sozinho não rege nada (AEP-0096).
    if (hasExplicitToolPolicy || (normalizedDefault !== '' && enabledTools == null)) {
      const defaultState = normalizedDefault === TOOL_POLICY_ON_DEMAND
        ? TOOL_POLICY_ON_DEMAND
        : TOOL_POLICY_DISABLED;
      for (const item of toolRows) {
        policy[item.name] = resolveToolPolicy(normalizedToolPolicy, defaultState, {
          name: item.name,
          package: item.package,
          optIn: item.optIn,
        }).state;
      }
      for (const rawName of runtimeTools) {
        const name = rawName.trim();
        if (!name) continue;
        const item = toolRows.find((row) => row.name === name);
        if (!item) continue;
        const match = resolveToolPolicy(normalizedToolPolicy, defaultState, {
          name: item.name,
          package: item.package,
          optIn: item.optIn,
        });
        // RuntimeTools espelha a autorização explícita do control-plane no
        // backend (AEP-0081 D8); não é uma elevação causada pelo wildcard.
        if (match.explicit && match.state === TOOL_POLICY_DISABLED && !match.deniedOptIn) continue;
        policy[name] = TOOL_POLICY_PRELOADED;
      }
      return withCatalogForOnDemandTools(policy, {
        explicitlyDisabled: catalogExplicitlyDisabled,
        hasOnDemandOutsideGrid,
      });
    }
    if (enabledTools == null) {
      for (const item of toolRows) {
        policy[item.name] = item.optIn ? TOOL_POLICY_DISABLED : TOOL_POLICY_ON_DEMAND;
      }
      if (allNames.includes(TOOL_CATALOG_NAME)) {
        policy[TOOL_CATALOG_NAME] = TOOL_POLICY_PRELOADED;
      }
      for (const name of runtimeTools) {
        if (allNames.includes(name)) policy[name] = TOOL_POLICY_PRELOADED;
      }
      return policy;
    }
    const enabledSet = new Set(enabledTools.map((name) => name.trim()));
    for (const name of allNames) {
      policy[name] = enabledSet.has(name) ? TOOL_POLICY_PRELOADED : TOOL_POLICY_DISABLED;
    }
    if (enabledTools.length > 0) {
      for (const name of runtimeTools) {
        if (allNames.includes(name)) policy[name] = TOOL_POLICY_PRELOADED;
      }
    }
    return policy;
  }, [
    allNames,
    catalogExplicitlyDisabled,
    enabledTools,
    hasExplicitToolPolicy,
    hasOnDemandOutsideGrid,
    normalizedToolPolicy,
    runtimeTools,
    toolPolicyDefault,
    toolRows,
  ]);
  const effectiveToolPolicyRef = useRef(effectiveToolPolicy);
  effectiveToolPolicyRef.current = effectiveToolPolicy;

  const selectedIds: Set<string | number> = useMemo(
    () => new Set<string | number>(
      allNames.filter((name) => effectiveToolPolicy[name] !== TOOL_POLICY_DISABLED)
    ),
    [allNames, effectiveToolPolicy],
  );

  const commitToolPolicy = useCallback((nextPolicy: Record<string, ToolPolicyState>) => {
    // Herdar o disabled do default não é escolha de ninguém, e materializar
    // esse estado ao lado de uma tool sob demanda a deixaria inalcançável,
    // porque o backend trata bloqueio explícito do catálogo como definitivo. Só
    // vale como escolha o bloqueio que já estava no perfil ou o que o usuário
    // acabou de fazer — e enquanto ele seguir desabilitado nesta gravação.
    const catalogChosen = nextPolicy[TOOL_CATALOG_NAME] === TOOL_POLICY_DISABLED
      && (catalogExplicitlyDisabled
        || effectiveToolPolicyRef.current[TOOL_CATALOG_NAME] !== TOOL_POLICY_DISABLED);
    const resolved = withCatalogForOnDemandTools(nextPolicy, {
      explicitlyDisabled: catalogChosen,
      hasOnDemandOutsideGrid,
    });
    effectiveToolPolicyRef.current = resolved;
    const merged = { ...policyEntriesOutsideGrid, ...dropRuntimeOnlyPromotions(resolved) };
    if (onPolicyChange) {
      // Perfil legado aberto (sem allowlist, sem mapa e sem default) é
      // catalog-first: toda tool futura nasce sob demanda. Gravar só o mapa
      // zeraria o enabled_tools e o backend passaria a reger as não listadas
      // pelo default vazio, virando fail-closed sem ninguém pedir (D3 da
      // AEP-0096).
      if (isLegacyCatalogFirst) {
        onPolicyChange(merged, { toolPolicyDefault: TOOL_POLICY_ON_DEMAND });
        return;
      }
      onPolicyChange(merged);
      return;
    }
    onChange('tool_policy', merged);
  }, [
    catalogExplicitlyDisabled,
    dropRuntimeOnlyPromotions,
    hasOnDemandOutsideGrid,
    isLegacyCatalogFirst,
    onChange,
    onPolicyChange,
    policyEntriesOutsideGrid,
  ]);

  // Trocar o padrão num perfil legado precisa materializar a allowlist como
  // tool_policy no mesmo salvamento. Sem isso o backend mantém a allowlist e a
  // escolha não teria efeito nenhum.
  const handleToolPolicyDefaultChange = useCallback((value: string) => {
    if (enabledTools != null && !hasExplicitToolPolicy && onPolicyChange) {
      const migrated = withCatalogForOnDemandTools(effectiveToolPolicy, {
        explicitlyDisabled: catalogExplicitlyDisabled,
        // O novo default rege as tools que a allowlist não citava; com
        // on_demand elas passam a depender do catálogo para serem carregadas.
        hasOnDemandOutsideGrid: hasOnDemandOutsideGrid
          || value.trim() === TOOL_POLICY_ON_DEMAND,
      });
      onPolicyChange(
        { ...policyEntriesOutsideGrid, ...dropRuntimeOnlyPromotions(migrated) },
        { toolPolicyDefault: value },
      );
      return;
    }
    onChange('tool_policy_default', value);
  }, [
    catalogExplicitlyDisabled,
    dropRuntimeOnlyPromotions,
    effectiveToolPolicy,
    enabledTools,
    hasExplicitToolPolicy,
    hasOnDemandOutsideGrid,
    onChange,
    onPolicyChange,
    policyEntriesOutsideGrid,
  ]);

  const isExplicitlyDisabled = useCallback(
    (name: string) => isToolExplicitlyDisabled(normalizedToolPolicy, name),
    [normalizedToolPolicy],
  );

  const shouldPreserveControlPlane = useCallback((name: string) => (
    CONTROL_PLANE_TOOLS.has(name)
    && effectiveToolPolicy[name] === TOOL_POLICY_PRELOADED
    && !isExplicitlyDisabled(name)
  ), [effectiveToolPolicy, isExplicitlyDisabled]);

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
        next[name] = shouldPreserveControlPlane(name)
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
  }, [allNames, commitToolPolicy, effectiveToolPolicy, filteredNames, isExplicitlyDisabled, isFiltered, selectedIds.size, setToolsState, shouldPreserveControlPlane]);

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
        next[name] = shouldPreserveControlPlane(name)
          ? TOOL_POLICY_PRELOADED
          : TOOL_POLICY_DISABLED;
      }
      commitToolPolicy(next);
      return;
    }
    setToolsState(filteredNames, TOOL_POLICY_DISABLED);
  }, [isFiltered, allNames, commitToolPolicy, effectiveToolPolicy, filteredNames, setToolsState, shouldPreserveControlPlane]);

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
            <label htmlFor="pf-tool-policy-default" className="profiles-field__label">
              {t('profiles.toolPolicyDefaultLabel', 'Estado padrão de novas ferramentas')}
            </label>
            <select
              id="pf-tool-policy-default"
              className="profiles-field__select"
              value={toolPolicyDefault?.trim() ?? ''}
              onChange={(e) => handleToolPolicyDefaultChange(e.target.value)}
              disabled={disabled}
              data-testid="tool-policy-default-select"
            >
              <option value="">{t('profiles.toolPolicyDefaultLegacy', 'Compatibilidade com perfil legado')}</option>
              <option value={TOOL_POLICY_DISABLED}>{t('profiles.toolPolicyDefaultDisabled', 'Desabilitadas')}</option>
              <option value={TOOL_POLICY_ON_DEMAND}>{t('profiles.toolPolicyDefaultOnDemand', 'Sob demanda')}</option>
            </select>
            <span className="profiles-field__hint">
              {t('profiles.toolPolicyDefaultHint', 'Define o estado de ferramentas futuras e integrações não listadas explicitamente. Ferramentas opt-in continuam bloqueadas.')}
            </span>
      </div>

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

function isToolExplicitlyDisabled(toolPolicy: Record<string, string> | null, name: string): boolean {
  return toolPolicy != null
    && Object.prototype.hasOwnProperty.call(toolPolicy, name)
    && normalizeToolPolicyState(toolPolicy[name]) === TOOL_POLICY_DISABLED;
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
