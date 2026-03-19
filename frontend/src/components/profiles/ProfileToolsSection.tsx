import { useCallback } from 'react';
import { main, allowlist } from '@wailsjs/go/models';
import { useTranslation } from 'react-i18next';
import { CollapsibleSection } from '../ui/CollapsibleSection';
import { DataGrid, DataGridColumn } from '../ui/DataGrid';
import { RangeSlider } from '../ui/RangeSlider';

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

  const allNames = availableTools.map(tool => tool.name);
  const allSelected = !enabledTools;
  const noneSelected = Array.isArray(enabledTools) && enabledTools.length === 0;
  const showSelectAll = !allSelected;
  const showDeselectAll = !noneSelected;

  const toolRows: ToolRow[] = availableTools.map(tool => ({
    id: tool.name,
    name: tool.name,
    description: tool.description || '',
  }));

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

  const isToolEnabled = useCallback((name: string) => {
    return !enabledTools || enabledTools.includes(name);
  }, [enabledTools]);

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
          <div
            className="profiles-field__tools-actions"
            role="toolbar"
            aria-label={t('profiles.toolsActionsLabel', 'Ações de seleção de ferramentas')}
            data-testid="tools-toolbar"
          >
            {showSelectAll && (
              <button
                type="button"
                className="profiles-field__tools-toggle"
                tabIndex={0}
                onClick={() => onChange('enabled_tools', null)}
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
                tabIndex={showSelectAll ? -1 : 0}
                onClick={() => onChange('enabled_tools', [])}
                disabled={disabled}
                data-testid="tools-deselect-all"
              >
                {t('profiles.toolsDeselectAll', 'Desmarcar todas')}
              </button>
            )}
          </div>
          <DataGrid<ToolRow>
            items={toolRows}
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
