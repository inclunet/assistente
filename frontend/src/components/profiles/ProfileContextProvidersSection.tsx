import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { contextprovider, profiles } from '@wailsjs/go/models';
import { CollapsibleSection } from '../ui/CollapsibleSection';
import { DataGrid, DataGridColumn } from '../ui/DataGrid';

type ContextProviderConfigMap = Record<string, profiles.ContextProviderProfileConfig>;

interface ProviderRow {
  id: string;
  name: string;
  displayName: string;
  description: string;
  defaultEnabled: boolean;
  effectiveEnabled: boolean;
  effectiveBudget: number;
  budgetOverride: number;
  supportsSettings: boolean;
}

export interface ProfileContextProvidersSectionProps {
  providers: contextprovider.ProviderMetadata[];
  value?: ContextProviderConfigMap | null;
  onChange: (value: ContextProviderConfigMap) => void;
  disabled?: boolean;
}

export function ProfileContextProvidersSection({
  providers,
  value,
  onChange,
  disabled = false,
}: ProfileContextProvidersSectionProps) {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(true);
  const config = value ?? {};

  const rows = useMemo<ProviderRow[]>(() => providers.map((provider) => {
    const providerConfig = config[provider.name];
    const hasEnabledOverride = typeof providerConfig?.enabled === 'boolean';
    const budgetOverride = providerConfig?.budget ?? 0;
    return {
      id: provider.name,
      name: provider.name,
      displayName: provider.display_name || provider.name,
      description: provider.description || '',
      defaultEnabled: provider.default_enabled,
      effectiveEnabled: hasEnabledOverride ? Boolean(providerConfig?.enabled) : provider.default_enabled,
      effectiveBudget: budgetOverride > 0 ? budgetOverride : provider.default_budget,
      budgetOverride,
      supportsSettings: provider.supports_settings,
    };
  }), [config, providers]);

  const selectedIds = useMemo(
    () => new Set<string | number>(rows.filter((row) => row.effectiveEnabled).map((row) => row.id)),
    [rows],
  );

  const updateProvider = useCallback((providerName: string, patch: Partial<profiles.ContextProviderProfileConfig>) => {
    const previous = config[providerName] ?? {};
    const nextProviderConfig = { ...previous, ...patch };
    const next = { ...config, [providerName]: nextProviderConfig };
    onChange(next);
  }, [config, onChange]);

  const handleSelectionChange = useCallback((newSelectedIds: Set<string | number>) => {
    let next: ContextProviderConfigMap | null = null;

    for (const row of rows) {
      const enabled = newSelectedIds.has(row.id);
      if (enabled !== row.effectiveEnabled) {
        const previous = (next ?? config)[row.name] ?? {};
        next = {
          ...(next ?? config),
          [row.name]: { ...previous, enabled },
        };
      }
    }

    if (next) {
      onChange(next);
    }
  }, [config, onChange, rows]);

  const handleBudgetChange = useCallback((row: ProviderRow, valueText: string) => {
    const parsed = Number.parseInt(valueText, 10);
    updateProvider(row.name, { budget: Number.isFinite(parsed) && parsed > 0 ? parsed : 0 });
  }, [updateProvider]);

  const columns: DataGridColumn<ProviderRow>[] = [
    {
      key: 'enabled',
      label: '',
      width: '44px',
      format: (_value: unknown, row: ProviderRow) => {
        const localizedName = t(`profiles.contextProviderNames.${row.name}`, row.displayName);
        return (
          <input
            type="checkbox"
            checked={row.effectiveEnabled}
            readOnly
            tabIndex={-1}
            aria-label={t('profiles.contextProviderEnabledAria', '{{name}}: {{state}}', {
              name: localizedName,
              state: row.effectiveEnabled ? t('common.enabled', 'Habilitado') : t('common.disabled', 'Desabilitado'),
            })}
            style={{ pointerEvents: 'none' }}
          />
        );
      },
    },
    {
      key: 'displayName',
      label: t('profiles.contextProviderColName', 'Provider'),
      width: '24%',
      format: (_value: unknown, row: ProviderRow) => (
        <div className="profiles-context-provider__name">
          <span>{t(`profiles.contextProviderNames.${row.name}`, row.displayName)}</span>
          <span className="profiles-context-provider__slug">{row.name}</span>
        </div>
      ),
    },
    {
      key: 'description',
      label: t('profiles.contextProviderColDescription', 'Descrição'),
      truncate: true,
      format: (_value: unknown, row: ProviderRow) => t(`profiles.contextProviderDescriptions.${row.name}`, row.description),
    },
    {
      key: 'state',
      label: t('profiles.contextProviderColState', 'Estado'),
      width: '120px',
      format: (_value: unknown, row: ProviderRow) => (
        row.effectiveEnabled
          ? t('profiles.contextProviderStateEnabled', 'habilitado')
          : t('profiles.contextProviderStateDisabled', 'desabilitado')
      ),
    },
    {
      key: 'budget',
      label: t('profiles.contextProviderColBudget', 'Budget efetivo'),
      width: '150px',
      format: (_value: unknown, row: ProviderRow) => {
        const localizedName = t(`profiles.contextProviderNames.${row.name}`, row.displayName);
        return (
          <label className="profiles-context-provider__budget">
            <span className="sr-only">
              {t('profiles.contextProviderBudgetLabel', 'Budget em caracteres para {{name}}', { name: localizedName })}
            </span>
            <input
              type="number"
              min={0}
              step={100}
              value={row.budgetOverride > 0 ? row.budgetOverride : ''}
              placeholder={String(row.effectiveBudget)}
              onChange={(event) => handleBudgetChange(row, event.target.value)}
              onClick={(event) => event.stopPropagation()}
              onMouseDown={(event) => event.stopPropagation()}
              disabled={disabled}
              aria-describedby="context-providers-budget-hint"
            />
          </label>
        );
      },
    },
  ];

  return (
    <CollapsibleSection
      title={t('profiles.collapseContextProviders', 'Context Providers')}
      isOpen={isOpen}
      onToggle={() => setIsOpen((current) => !current)}
      disabled={disabled}
    >
      <p className="profiles-field__hint">
        {t('profiles.contextProvidersHint', 'Controle quais providers podem inserir blocos automáticos no prompt e limite o budget em caracteres. Tools relacionadas podem continuar disponíveis por outros mecanismos.')}
      </p>
      <p id="context-providers-budget-hint" className="profiles-field__hint">
        {t('profiles.contextProvidersBudgetHint', 'Deixe o budget vazio ou 0 para usar o default do provider. O limite é em caracteres, não tokens.')}
      </p>
      {providers.length > 0 ? (
        <DataGrid<ProviderRow>
          items={rows}
          columns={columns}
          label={t('profiles.contextProvidersGridLabel', 'Lista de Context Providers')}
          getItemId={(item) => item.id}
          selectedIds={selectedIds}
          selectionMode="checkbox"
          onSelectionChange={handleSelectionChange}
          showHeader={true}
          autoFocusOnMount={false}
          className="profiles-context-providers-datagrid"
        />
      ) : (
        <p className="profiles-field__hint" role="status">
          {t('profiles.contextProvidersUnavailable', 'Nenhum Context Provider registrado foi informado pelo runtime.')}
        </p>
      )}
      <p className="profiles-field__hint">
        {t('profiles.contextProvidersSettingsNote', 'Settings específicas aparecerão aqui quando um provider declarar suporte no contrato de metadados.')}
      </p>
    </CollapsibleSection>
  );
}
