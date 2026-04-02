import { useState, useCallback, useMemo } from 'react';
import { FilterOutlined, UpOutlined, DownOutlined } from '@ant-design/icons';
import { skills } from '@wailsjs/go/models';
import { useTranslation } from 'react-i18next';
import { CollapsibleSection } from '../ui/CollapsibleSection';
import { DataGrid, DataGridColumn } from '../ui/DataGrid';
import { Combobox, type ComboboxItem } from '../pickers/Combobox';
import { useToolbarKeyboardNav } from '../../hooks/useToolbarKeyboardNav';

export type SkillFilter = 'all' | 'exe' | 'home' | 'workdir';

const SKILL_SOURCE_LABELS: Record<string, string> = {
  exe: 'Builtin',
  home: 'Home',
  workdir: 'Workspace',
};

export interface ProfileSkillsSectionProps {
  availableSkills: Array<
    | skills.SkillInfo
    | { slug: string; name: string; description?: string; version?: string; source?: string }
  >;
  enabledSkills?: string[];
  disableOnDemand?: boolean;
  skillsDisabled?: boolean;
  onChange: (
    field: 'enabled_skills' | 'disable_on_demand_skills' | 'disable_skills',
    value: string[] | boolean
  ) => void;
  disabled?: boolean;
}

interface SkillRow {
  id: string;
  slug: string;
  name: string;
  description: string;
  source: string;
}

export function ProfileSkillsSection({
  availableSkills,
  enabledSkills = [],
  disableOnDemand = false,
  skillsDisabled = false,
  onChange,
  disabled = false,
}: ProfileSkillsSectionProps) {
  const { t } = useTranslation();
  const [focusedSlug, setFocusedSlug] = useState<string | null>(null);
  const [filter, setFilter] = useState<SkillFilter>('all');
  const [search, setSearch] = useState('');

  const autoloadSet = new Set(enabledSkills);

  const autoloadSkills = enabledSkills
    .map(slug => availableSkills.find(s => s.slug === slug))
    .filter(Boolean) as Array<{ slug: string; name: string; description?: string; source?: string }>;
  const onDemandSkills = availableSkills.filter(s => !autoloadSet.has(s.slug));
  const sortedSkills: SkillRow[] = useMemo(
    () => [...autoloadSkills, ...onDemandSkills].map(s => ({
      id: s.slug,
      slug: s.slug,
      name: s.name,
      description: s.description || '',
      source: s.source || 'exe',
    })),
    [availableSkills, enabledSkills],
  );

  const availableSources = useMemo(() => {
    const sources = new Set(sortedSkills.map((s) => s.source));
    return Array.from(sources).sort();
  }, [sortedSkills]);

  const filterItems: ComboboxItem[] = useMemo(() => [
    { value: 'all', label: t('profiles.filterAll', 'Todas') },
    ...availableSources.map((src) => ({
      value: src,
      label: t(`profiles.filterSkillSource.${src}`, SKILL_SOURCE_LABELS[src] ?? src),
    })),
  ], [t, availableSources]);

  const filteredSkills = useMemo(() => {
    const term = search.toLowerCase().trim();
    return sortedSkills.filter((row) => {
      if (filter !== 'all' && row.source !== filter) return false;
      if (term && !row.name.toLowerCase().includes(term) && !row.description.toLowerCase().includes(term)) return false;
      return true;
    });
  }, [sortedSkills, filter, search]);

  const filteredSlugs = useMemo(() => new Set(filteredSkills.map((r) => r.slug)), [filteredSkills]);
  const isFiltered = filter !== 'all' || search.trim() !== '';

  const selectedIds = new Set<string | number>(enabledSkills);

  const allSlugs = availableSkills.map(s => s.slug);
  const allFilteredSelected = [...filteredSlugs].every((s) => selectedIds.has(s));
  const noneFilteredSelected = [...filteredSlugs].every((s) => !selectedIds.has(s));
  const showSelectAll = !allFilteredSelected;
  const showDeselectAll = !noneFilteredSelected;

  const handleSelectionChange = useCallback((newSelectedIds: Set<string | number>) => {
    const prevSet = new Set(enabledSkills);
    const newSet = newSelectedIds as Set<string>;

    let added: string | null = null;
    let removed: string | null = null;
    for (const id of newSet) {
      if (!prevSet.has(id)) { added = id; break; }
    }
    for (const id of prevSet) {
      if (!newSet.has(id as string)) { removed = id; break; }
    }

    if (added) {
      const newList = [...enabledSkills, added];
      onChange('enabled_skills', newList.length === allSlugs.length ? allSlugs : newList);
    } else if (removed) {
      onChange('enabled_skills', enabledSkills.filter(s => s !== removed));
    }
  }, [enabledSkills, allSlugs, onChange]);

  const handleSelectFiltered = useCallback(() => {
    if (!isFiltered) {
      onChange('enabled_skills', allSlugs);
      return;
    }
    const current = new Set(enabledSkills);
    for (const slug of filteredSlugs) current.add(slug);
    const result = allSlugs.filter((s) => current.has(s));
    onChange('enabled_skills', result.length === allSlugs.length ? allSlugs : result);
  }, [isFiltered, enabledSkills, allSlugs, filteredSlugs, onChange]);

  const handleDeselectFiltered = useCallback(() => {
    if (!isFiltered) {
      onChange('enabled_skills', []);
      return;
    }
    const result = enabledSkills.filter((s) => !filteredSlugs.has(s));
    onChange('enabled_skills', result);
  }, [isFiltered, enabledSkills, filteredSlugs, onChange]);

  const handleMoveItem = useCallback((fromIndex: number, toIndex: number) => {
    const item = filteredSkills[fromIndex];
    const target = filteredSkills[toIndex];
    if (!item || !target) return;
    if (!autoloadSet.has(item.slug) || !autoloadSet.has(target.slug)) return;

    const fromEnabledIdx = enabledSkills.indexOf(item.slug);
    const toEnabledIdx = enabledSkills.indexOf(target.slug);
    if (fromEnabledIdx < 0 || toEnabledIdx < 0) return;

    const newList = [...enabledSkills];
    [newList[fromEnabledIdx], newList[toEnabledIdx]] = [newList[toEnabledIdx], newList[fromEnabledIdx]];
    onChange('enabled_skills', newList);
  }, [filteredSkills, autoloadSet, enabledSkills, onChange]);

  const handleMoveButton = useCallback((direction: 'up' | 'down') => {
    if (!focusedSlug || !autoloadSet.has(focusedSlug)) return;
    const idx = enabledSkills.indexOf(focusedSlug);
    if (idx < 0) return;
    const newIdx = direction === 'up' ? idx - 1 : idx + 1;
    if (newIdx < 0 || newIdx >= enabledSkills.length) return;

    const newList = [...enabledSkills];
    [newList[idx], newList[newIdx]] = [newList[newIdx], newList[idx]];
    onChange('enabled_skills', newList);
  }, [focusedSlug, autoloadSet, enabledSkills, onChange]);

  const handleFocusChange = useCallback((item: SkillRow | null) => {
    setFocusedSlug(item?.slug ?? null);
  }, []);

  const focusedIsEnabled = focusedSlug ? autoloadSet.has(focusedSlug) : false;
  const focusedEnabledIdx = focusedSlug ? enabledSkills.indexOf(focusedSlug) : -1;
  const canMoveUp = focusedIsEnabled && focusedEnabledIdx > 0;
  const canMoveDown = focusedIsEnabled && focusedEnabledIdx >= 0 && focusedEnabledIdx < enabledSkills.length - 1;

  const toolbarRef = useToolbarKeyboardNav();

  const columns: DataGridColumn<SkillRow>[] = [
    {
      key: 'checked',
      label: '',
      width: '40px',
      format: (_value: unknown, item: SkillRow) => {
        const idx = enabledSkills.indexOf(item.slug);
        const checked = idx >= 0;
        return (
          <input
            type="checkbox"
            checked={checked}
            readOnly
            tabIndex={-1}
            aria-label={checked
              ? t('profiles.skillAutoload', `${item.name} autoload #${idx + 1}`)
              : t('profiles.skillOnDemand', `${item.name} sob demanda`)}
            style={{ pointerEvents: 'none' }}
          />
        );
      },
    },
    {
      key: 'order',
      label: '#',
      width: '40px',
      format: (_value: unknown, item: SkillRow) => {
        const idx = enabledSkills.indexOf(item.slug);
        return idx >= 0 ? `${idx + 1}` : '';
      },
    },
    {
      key: 'name',
      label: t('profiles.skillColName', 'Nome'),
      width: '28%',
    },
    {
      key: 'description',
      label: t('profiles.skillColDesc', 'Descrição'),
      truncate: true,
    },
  ];

  return (
    <CollapsibleSection
      title={t('profiles.collapseSkills', 'Skills')}
      isOpen={!skillsDisabled}
      onToggle={() => onChange('disable_skills', !skillsDisabled)}
      disabled={disabled}
      badge={skillsDisabled ? 'off' : 'on'}
    >
      {availableSkills.length > 0 ? (
        <>
          <p className="profiles-field__hint">
            {t('profiles.skillsHint', 'Marque skills para autoload (injetados no system prompt em ordem). Desmarcados ficam disponíveis sob demanda.')}
          </p>
          <input
            type="text"
            className="profiles-field__filter-search"
            placeholder={t('profiles.skillsSearchPlaceholder', 'Buscar skill…')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            aria-label={t('profiles.skillsSearchLabel', 'Filtrar skills por nome')}
            data-testid="skills-search"
          />
          <div
            ref={toolbarRef}
            className="profiles-field__tools-actions"
            role="toolbar"
            aria-label={t('profiles.skillsActionsLabel', 'Ações de seleção de skills')}
            data-testid="skills-toolbar"
          >
            {availableSources.length > 1 && (
              <div data-testid="skills-filter">
                <Combobox
                  items={filterItems}
                  selected={filter}
                  onSelect={(value) => setFilter(value as SkillFilter)}
                  label={t('profiles.skillsFilterLabel', 'Filtrar por origem')}
                  icon={<FilterOutlined aria-hidden="true" />}
                  maxWidth="180px"
                  disabled={disabled}
                />
              </div>
            )}
            {showSelectAll && (
              <button
                type="button"
                className="profiles-field__tools-toggle"
                onClick={handleSelectFiltered}
                disabled={disabled}
                data-testid="skills-select-all"
              >
                {t('profiles.skillsSelectAll', 'Selecionar todas')}
              </button>
            )}
            {showDeselectAll && (
              <button
                type="button"
                className="profiles-field__tools-toggle"
                onClick={handleDeselectFiltered}
                disabled={disabled}
                data-testid="skills-deselect-all"
              >
                {t('profiles.skillsDeselectAll', 'Desmarcar todas')}
              </button>
            )}
            <button
              type="button"
              className="profiles-field__tools-toggle"
              tabIndex={-1}
              onClick={() => handleMoveButton('up')}
              disabled={disabled || !canMoveUp}
              aria-label={t('profiles.skillMoveUp', 'Subir skill')}
              data-testid="skills-move-up"
            >
              <UpOutlined aria-hidden="true" />
            </button>
            <button
              type="button"
              className="profiles-field__tools-toggle"
              tabIndex={-1}
              onClick={() => handleMoveButton('down')}
              disabled={disabled || !canMoveDown}
              aria-label={t('profiles.skillMoveDown', 'Descer skill')}
              data-testid="skills-move-down"
            >
              <DownOutlined aria-hidden="true" />
            </button>
            <button
              type="button"
              className={`profiles-field__tools-toggle ${disableOnDemand ? 'profiles-field__tools-toggle--active' : ''}`}
              tabIndex={-1}
              onClick={() => onChange('disable_on_demand_skills', !disableOnDemand)}
              disabled={disabled}
              aria-pressed={disableOnDemand}
              data-testid="skills-toggle-on-demand"
            >
              {disableOnDemand
                ? t('profiles.skillsOnDemandOff', 'Sob demanda: desativado')
                : t('profiles.skillsOnDemandOn', 'Sob demanda: ativado')}
            </button>
          </div>
          {filteredSkills.length > 0 ? (
            <DataGrid<SkillRow>
              items={filteredSkills}
              columns={columns}
              label={t('profiles.skillsGridLabel', 'Lista de skills')}
              getItemId={(item) => item.slug}
              selectedIds={selectedIds}
              selectionMode="checkbox"
              onSelectionChange={handleSelectionChange}
              onMoveItem={handleMoveItem}
              onFocusChange={handleFocusChange}
              showHeader={true}
              autoFocusOnMount={false}
              className="profiles-skills-datagrid"
            />
          ) : (
            <p className="profiles-field__hint profiles-field__no-results">
              {t('profiles.skillsNoResults', 'Nenhum skill corresponde ao filtro.')}
            </p>
          )}
        </>
      ) : (
        <p className="profiles-field__hint" style={{ margin: 0 }}>
          {t('profiles.noSkillsAvailable', 'Nenhum skill encontrado.')}
        </p>
      )}
    </CollapsibleSection>
  );
}
