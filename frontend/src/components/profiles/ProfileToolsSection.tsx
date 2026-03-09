import { main, allowlist } from '@wailsjs/go/models';
import { useTranslation } from 'react-i18next';
import { CollapsibleSection } from '../ui/CollapsibleSection';

export interface ProfileToolsSectionProps {
  availableTools: main.ToolInfo[];
  enabledTools?: string[] | null;
  toolsDisabled?: boolean;
  commandAllowlist?: string;
  availableAllowlists: allowlist.AllowlistInfo[];
  onChange: (field: 'enabled_tools' | 'command_allowlist' | 'disable_tools', value: any) => void;
  disabled?: boolean;
}

export function ProfileToolsSection({
  availableTools,
  enabledTools = null,
  toolsDisabled = false,
  commandAllowlist = '',
  availableAllowlists = [],
  onChange,
  disabled = false,
}: ProfileToolsSectionProps) {
  const { t } = useTranslation();

  const allNames = availableTools.map(tool => tool.name);
  const allSelected = !enabledTools;
  const noneSelected = Array.isArray(enabledTools) && enabledTools.length === 0;
  const showSelectAll = !allSelected;
  const showDeselectAll = !noneSelected;

  const handleToggleTool = (toolName: string) => {
    if (!enabledTools) {
      // All enabled, so disable this one
      onChange('enabled_tools', allNames.filter(n => n !== toolName));
    } else if (enabledTools.includes(toolName)) {
      // Enabled, so disable it
      onChange('enabled_tools', enabledTools.filter(n => n !== toolName));
    } else {
      // Disabled, so enable it
      const newList = [...enabledTools, toolName];
      // If all tools selected, send null (means all enabled)
      onChange('enabled_tools', newList.length === allNames.length ? null : newList);
    }
  };

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
          <div
            className="profiles-field__tools-grid"
            role="group"
            aria-label={t('profiles.toolsGridLabel', 'Lista de ferramentas disponíveis')}
            data-testid="tools-grid"
          >
            {availableTools.map((tool) => {
              const isEnabled = !enabledTools || enabledTools.includes(tool.name);
              return (
                <div key={tool.name} className="profiles-field__tool-item">
                  <input
                    type="checkbox"
                    id={`pf-tool-${tool.name}`}
                    checked={isEnabled}
                    onChange={() => handleToggleTool(tool.name)}
                    disabled={disabled}
                    aria-labelledby={`pf-tool-name-${tool.name}`}
                    aria-describedby={`pf-tool-desc-${tool.name}`}
                    data-testid={`tool-checkbox-${tool.name}`}
                  />
                  <label htmlFor={`pf-tool-${tool.name}`} className="profiles-field__tool-label">
                    <span id={`pf-tool-name-${tool.name}`} className="profiles-field__tool-name">
                      {tool.name}
                    </span>
                    <span id={`pf-tool-desc-${tool.name}`} className="profiles-field__tool-desc">
                      {tool.description}
                    </span>
                  </label>
                </div>
              );
            })}
          </div>

          {/* Allowlist de Comandos */}
          <div className="profiles-field" style={{ marginTop: '0.75rem' }}>
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
