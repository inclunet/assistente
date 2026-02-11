import { useState, useEffect, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import {
  GetSkills,
  GetSkill,
  CreateSkill,
  UpdateSkill,
  DeleteSkill,
  GetSkillSearchPaths,
} from '../../wailsjs/go/main/App';
import { skills, main } from '../../wailsjs/go/models';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar } from '../components/ui/Toolbar';
import { Button } from '../components';
import { useGridFocus } from '../hooks/useGridFocus';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useUIStore } from '../store/uiStore';
import './SkillsPage.css';

type SkillInfo = skills.SkillInfo;

interface SkillRow {
  id: string;
  slug: string;
  name: string;
  description: string;
  auto: boolean;
  source: string;
  tools: string[];
  globs: string[];
}

export default function SkillsPage() {
  const { t } = useTranslation();
  const { addToast } = useUIStore();
  const { announce } = useAnnouncer();
  const { focusFirstCell, handleGridReady } = useGridFocus();

  // Grid state
  const [rows, setRows] = useState<SkillRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [searchPaths, setSearchPaths] = useState<string[]>([]);

  // Editor state
  const [editingSkill, setEditingSkill] = useState<{
    name: string;
    description: string;
    auto: boolean;
    tools: string;
    globs: string;
    content: string;
  } | null>(null);
  const [editingSlug, setEditingSlug] = useState<string | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [saving, setSaving] = useState(false);

  // Refs for focus management
  const editorRef = useRef<HTMLDivElement>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const editorJustOpenedRef = useRef(false);

  const loadSkills = useCallback(async () => {
    setLoading(true);
    try {
      const [list, paths] = await Promise.all([
        GetSkills(),
        GetSkillSearchPaths(),
      ]);
      setSearchPaths(paths || []);

      const mapped: SkillRow[] = (list || []).map((s: SkillInfo) => ({
        id: s.slug,
        slug: s.slug,
        name: s.name,
        description: s.description || '',
        auto: s.auto,
        source: s.source,
        tools: s.tools || [],
        globs: s.globs || [],
      }));
      setRows(mapped);
    } catch (error) {
      console.error('Erro ao carregar skills:', error);
      addToast(t('skills.loadError', 'Erro ao carregar skills'), 'error');
    } finally {
      setLoading(false);
    }
  }, [addToast, t]);

  useEffect(() => {
    loadSkills();
  }, [loadSkills]);

  // Focus editor only when it first opens
  useEffect(() => {
    if (editingSkill && editorRef.current && editorJustOpenedRef.current) {
      editorJustOpenedRef.current = false;
      previousFocusRef.current = document.activeElement as HTMLElement;
      const firstInput = editorRef.current.querySelector<HTMLElement>(
        'input, select, textarea, [tabindex]'
      );
      setTimeout(() => firstInput?.focus(), 50);
    }
  }, [editingSkill]);

  // ESC to close editor
  useEffect(() => {
    if (!editingSkill) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && editingSkill) {
        e.preventDefault();
        handleCloseEditor();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [editingSkill]);

  // --- Grid actions ---

  const handleEditSkill = async (row: SkillRow) => {
    try {
      const skill = await GetSkill(row.slug);
      setEditingSlug(row.slug);
      setIsNew(false);
      editorJustOpenedRef.current = true;
      setEditingSkill({
        name: skill.name,
        description: skill.description,
        auto: skill.auto,
        tools: (skill.tools || []).join(', '),
        globs: (skill.globs || []).join(', '),
        content: skill.content,
      });
      announce(t('skills.editorOpened', `Editor aberto para ${row.name}`));
    } catch (error) {
      console.error('Erro ao carregar skill:', error);
      addToast(t('skills.loadOneError', 'Erro ao carregar skill'), 'error');
    }
  };

  const handleNewSkill = () => {
    setEditingSlug(null);
    setIsNew(true);
    editorJustOpenedRef.current = true;
    setEditingSkill({
      name: '',
      description: '',
      auto: false,
      tools: '',
      globs: '',
      content: '',
    });
    announce(t('skills.newSkillAnnounce', 'Editor aberto para novo skill'));
  };

  const handleDeleteSkill = async (row: SkillRow) => {
    if (!confirm(t('skills.confirmDelete', `Tem certeza que deseja excluir o skill "${row.name}"?`))) return;

    try {
      await DeleteSkill(row.slug);
      addToast(t('skills.deleted', 'Skill excluído!'), 'success');
      announce(t('skills.deletedAnnounce', 'Skill excluído'));

      if (editingSlug === row.slug) {
        setEditingSlug(null);
        setEditingSkill(null);
      }
      await loadSkills();
    } catch (error: any) {
      console.error('Erro ao excluir skill:', error);
      addToast(error.message || t('skills.deleteError', 'Erro ao excluir skill'), 'error');
    }
  };

  // --- Editor actions ---

  const handleSave = async () => {
    if (!editingSkill) return;

    if (!editingSkill.name.trim()) {
      addToast(t('skills.nameRequired', 'Nome é obrigatório'), 'error');
      return;
    }
    if (!editingSkill.description.trim()) {
      addToast(t('skills.descriptionRequired', 'Descrição é obrigatória'), 'error');
      return;
    }

    setSaving(true);
    try {
      const toolsList = editingSkill.tools.split(',').map(s => s.trim()).filter(Boolean);
      const globsList = editingSkill.globs.split(',').map(s => s.trim()).filter(Boolean);

      const req = main.SkillCreateRequest.createFrom({
        name: editingSkill.name.trim(),
        description: editingSkill.description.trim(),
        auto: editingSkill.auto,
        tools: toolsList.length > 0 ? toolsList : undefined,
        globs: globsList.length > 0 ? globsList : undefined,
        content: editingSkill.content,
      });

      if (isNew) {
        const slug = await CreateSkill(req);
        addToast(t('skills.created', `Skill "${editingSkill.name}" criado!`), 'success');
        announce(t('skills.createdAnnounce', `Skill ${editingSkill.name} criado`));
        setIsNew(false);
        setEditingSlug(slug);
      } else if (editingSlug) {
        await UpdateSkill(editingSlug, req);
        addToast(t('skills.updated', `Skill "${editingSkill.name}" atualizado!`), 'success');
        announce(t('skills.updatedAnnounce', `Skill ${editingSkill.name} atualizado`));
      }
      await loadSkills();
    } catch (error: any) {
      console.error('Erro ao salvar skill:', error);
      addToast(error.message || t('skills.saveError', 'Erro ao salvar skill'), 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleCloseEditor = () => {
    setEditingSkill(null);
    setEditingSlug(null);
    setIsNew(false);
    announce(t('skills.editorClosed', 'Editor fechado'));
    setTimeout(() => previousFocusRef.current?.focus(), 50);
  };

  const updateField = (field: string, value: any) => {
    if (!editingSkill) return;
    setEditingSkill({ ...editingSkill, [field]: value });
  };

  // --- Grid columns ---

  const columns: DataGridColumn<SkillRow>[] = [
    {
      key: 'name',
      label: t('skills.colName', 'Nome'),
      width: '25%',
    },
    {
      key: 'description',
      label: t('skills.colDescription', 'Descrição'),
      width: '35%',
      truncate: true,
    },
    {
      key: 'auto',
      label: t('skills.colMode', 'Modo'),
      width: '12%',
      format: (value: any) =>
        value ? (
          <span className="skills-badge skills-badge--auto">
            {t('skills.auto', 'Auto')}
          </span>
        ) : (
          <span className="skills-badge skills-badge--manual">
            {t('skills.manual', 'Manual')}
          </span>
        ),
    },
    {
      key: 'source',
      label: t('skills.colSource', 'Origem'),
      width: '13%',
      format: (value: any) => {
        switch (value) {
          case 'workdir': return t('skills.sourceWorkdir', 'Projeto');
          case 'home': return t('skills.sourceHome', 'Global');
          case 'exe': return t('skills.sourceExe', 'Embutido');
          default: return value;
        }
      },
    },
    {
      key: 'edit',
      label: '',
      width: '5%',
      action: true,
      actionIcon: '✏️',
      actionLabel: t('skills.edit', 'Editar skill'),
    },
    {
      key: 'delete',
      label: '',
      width: '5%',
      action: true,
      actionIcon: '🗑️',
      actionLabel: t('skills.delete', 'Excluir skill'),
    },
  ];

  const handleCellAction = (item: SkillRow, column: DataGridColumn<SkillRow>) => {
    if (column.key === 'edit') {
      handleEditSkill(item);
    } else if (column.key === 'delete') {
      handleDeleteSkill(item);
    }
  };

  // --- Filtering ---

  const filteredRows = rows.filter(row =>
    row.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
    row.description.toLowerCase().includes(searchTerm.toLowerCase()) ||
    row.slug.toLowerCase().includes(searchTerm.toLowerCase())
  );

  // --- Loading ---

  if (loading) {
    return (
      <div className="skills-page">
        <div className="loading" role="status">{t('skills.loading', 'Carregando skills...')}</div>
      </div>
    );
  }

  // --- Render ---

  const editorTitle = isNew
    ? t('skills.newSkillTitle', 'Novo Skill')
    : editingSkill?.name || '';

  return (
    <div className="skills-page">
      <Toolbar
        left={
          <h1 className="page-toolbar__title">
            {t('skills.pageTitle', 'Skills')}
          </h1>
        }
        searchPlaceholder={t('skills.search', 'Buscar skills...')}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        onFocusGrid={focusFirstCell}
        actions={[
          {
            key: 'new-skill',
            label: t('skills.newSkill', 'Novo Skill'),
            icon: '➕',
            onClick: handleNewSkill,
            variant: 'primary',
          },
        ]}
      />

      <DataGrid
        items={filteredRows}
        columns={columns}
        label={t('skills.gridLabel', 'Lista de skills')}
        getItemId={(item) => item.id}
        selectedIds={selectedIds}
        onSelectionChange={setSelectedIds}
        onActivate={(item) => handleEditSkill(item)}
        onDelete={(item) => handleDeleteSkill(item)}
        onCellAction={handleCellAction}
        onGridReady={handleGridReady}
      />

      {/* Editor Panel */}
      {editingSkill && (
        <div
          ref={editorRef}
          className="skills-editor"
          role="region"
          aria-label={t('skills.editorLabel', `Editor de skill: ${editorTitle}`)}
          aria-live="polite"
        >
          <div className="skills-editor__header">
            <h2 id="skills-editor-title">{editorTitle}</h2>
            <div className="skills-editor__actions">
              {editingSlug && (
                <Button
                  variant="danger"
                  onClick={() => {
                    const row = rows.find(r => r.slug === editingSlug);
                    if (row) handleDeleteSkill(row);
                  }}
                  aria-label={t('skills.deleteBtnLabel', `Excluir skill ${editingSkill.name}`)}
                >
                  {t('skills.deleteBtn', 'Excluir')}
                </Button>
              )}
              <Button onClick={handleSave} loading={saving}>
                {t('skills.saveBtn', 'Salvar')}
              </Button>
              <Button
                variant="ghost"
                onClick={handleCloseEditor}
                aria-label={t('skills.closeBtnLabel', 'Fechar editor, Escape')}
              >
                {t('skills.closeBtn', 'Fechar')}
              </Button>
            </div>
          </div>

          {/* General Section */}
          <section className="skills-section" aria-labelledby="section-general">
            <h3 id="section-general">{t('skills.sectionGeneral', 'Geral')}</h3>
            <div className="skills-fields">
              <div className="skills-field">
                <label htmlFor="sk-name" className="skills-field__label">
                  {t('skills.fieldName', 'Nome')}
                </label>
                <input
                  id="sk-name"
                  type="text"
                  className="skills-field__input"
                  value={editingSkill.name}
                  onChange={(e) => updateField('name', e.target.value)}
                  placeholder={t('skills.namePlaceholder', 'Ex: Criar Componente React')}
                />
              </div>
              <div className="skills-field">
                <label htmlFor="sk-description" className="skills-field__label">
                  {t('skills.fieldDescription', 'Descrição')}
                </label>
                <input
                  id="sk-description"
                  type="text"
                  className="skills-field__input"
                  value={editingSkill.description}
                  onChange={(e) => updateField('description', e.target.value)}
                  placeholder={t('skills.descriptionPlaceholder', 'Quando este skill deve ser usado')}
                />
              </div>
              <div className="skills-field skills-field--checkbox">
                <input
                  id="sk-auto"
                  type="checkbox"
                  checked={editingSkill.auto}
                  onChange={(e) => updateField('auto', e.target.checked)}
                />
                <label htmlFor="sk-auto" className="skills-field__label">
                  {t('skills.fieldAuto', 'Auto — injetar automaticamente no system prompt')}
                </label>
              </div>
              <p className="skills-field__hint">
                {editingSkill.auto
                  ? t('skills.autoHint', 'O conteúdo deste skill será incluído em toda conversa.')
                  : t('skills.manualHint', 'O assistente lerá este skill sob demanda quando for relevante.')}
              </p>
            </div>
          </section>

          {/* Metadata Section */}
          <section className="skills-section" aria-labelledby="section-metadata">
            <h3 id="section-metadata">{t('skills.sectionMetadata', 'Metadados')}</h3>
            <div className="skills-fields">
              <div className="skills-field">
                <label htmlFor="sk-tools" className="skills-field__label">
                  {t('skills.fieldTools', 'Ferramentas associadas')}
                </label>
                <input
                  id="sk-tools"
                  type="text"
                  className="skills-field__input"
                  value={editingSkill.tools}
                  onChange={(e) => updateField('tools', e.target.value)}
                  placeholder={t('skills.toolsPlaceholder', 'read_file, write_file (separar por vírgula)')}
                />
                <span className="skills-field__hint">
                  {t('skills.toolsHint', 'Informativo — indica quais tools o skill utiliza.')}
                </span>
              </div>
              <div className="skills-field">
                <label htmlFor="sk-globs" className="skills-field__label">
                  {t('skills.fieldGlobs', 'Padrões de arquivo')}
                </label>
                <input
                  id="sk-globs"
                  type="text"
                  className="skills-field__input"
                  value={editingSkill.globs}
                  onChange={(e) => updateField('globs', e.target.value)}
                  placeholder={t('skills.globsPlaceholder', '*.tsx, *.css (separar por vírgula)')}
                />
                <span className="skills-field__hint">
                  {t('skills.globsHint', 'Padrões de arquivo para ativação contextual futura.')}
                </span>
              </div>
            </div>
          </section>

          {/* Content Section */}
          <section className="skills-section" aria-labelledby="section-content">
            <h3 id="section-content">{t('skills.sectionContent', 'Conteúdo')}</h3>
            <div className="skills-fields">
              <div className="skills-field">
                <label htmlFor="sk-content" className="skills-field__label">
                  {t('skills.fieldContent', 'Instruções (Markdown)')}
                </label>
                <textarea
                  id="sk-content"
                  className="skills-field__textarea"
                  value={editingSkill.content}
                  onChange={(e) => updateField('content', e.target.value)}
                  placeholder={t('skills.contentPlaceholder', '# Instruções\n\nDescreva aqui o que o assistente deve fazer...')}
                  rows={12}
                />
              </div>
            </div>
          </section>
        </div>
      )}

      {/* Empty state when no skill is being edited */}
      {!editingSkill && rows.length > 0 && (
        <div className="skills-empty" role="status">
          <p>{t('skills.selectHint', 'Pressione Enter ou clique ✏️ para editar um skill.')}</p>
        </div>
      )}

      {/* Empty state when no skills exist */}
      {!editingSkill && rows.length === 0 && (
        <div className="skills-empty" role="status">
          <p>
            {t('skills.noSkills', 'Nenhum skill encontrado. Crie arquivos .md em')}{' '}
            <code>{searchPaths.length > 0 ? searchPaths[searchPaths.length - 1] : '.assistente/skills/'}</code>{' '}
            {t('skills.orUseButton', 'ou use o botão "Novo Skill".')}
          </p>
        </div>
      )}

      {/* Search paths footer */}
      {searchPaths.length > 0 && (
        <div className="skills-search-paths" role="contentinfo" aria-label={t('skills.searchPathsLabel', 'Caminhos de busca de skills')}>
          <p className="skills-search-paths__title">
            {t('skills.searchPaths', 'Caminhos de busca:')}
          </p>
          {searchPaths.map((path, i) => (
            <p key={i} className="skills-search-paths__item">{path}</p>
          ))}
        </div>
      )}
    </div>
  );
}
