import { useState, useEffect } from 'react';
import { 
  GetAllMemories, 
  CreateMemory, 
  UpdateMemory, 
  DeleteMemory,
  ExportMemories,
  ImportMemories,
  RegenerateMemoryEmbeddings,
  GetMemoryEmbeddingStatus
} from '../../wailsjs/go/main/App';
import { useTranslation } from 'react-i18next';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar, ToolbarAction } from '../components/ui/Toolbar';
import { SimpleModal } from '../components/ui/SimpleModal';
import { Input } from '../components/ui/Input';
import { Textarea } from '../components/ui/Textarea';
import { Select } from '../components/ui/Select';
import { Button } from '../components/ui/Button';
import { useGridFocus } from '../hooks/useGridFocus';
import { formatRelativeTime } from '../lib/dateUtils';
import { downloadJSON, openFileDialog, generateFilename } from '../lib/exportImport';
import './MemoryPage.css';

interface Memory {
  id: number;
  title: string;
  content: string;
  category: string;
  created_at?: string;
}

const CATEGORIES = [
  { value: 'core', label: 'Core (sempre no contexto)' },
  { value: 'usuario', label: 'Usuário' },
  { value: 'preferencia', label: 'Preferência' },
  { value: 'projeto', label: 'Projeto' },
  { value: 'contexto', label: 'Contexto' },
  { value: 'geral', label: 'Geral' }
];

export default function MemoryPage() {
  const { t } = useTranslation();
  const [memories, setMemories] = useState<Memory[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [filterCategory, setFilterCategory] = useState('all');
  const [showModal, setShowModal] = useState(false);
  const [editingMemory, setEditingMemory] = useState<Memory | null>(null);
  const [saving, setSaving] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [embeddingStatus, setEmbeddingStatus] = useState<any>(null);
  const { focusFirstCell, handleGridReady } = useGridFocus();
  
  // Form state
  const [formTitle, setFormTitle] = useState('');
  const [formContent, setFormContent] = useState('');
  const [formCategory, setFormCategory] = useState('geral');
  const [formError, setFormError] = useState('');

  useEffect(() => {
    loadMemories();
    loadEmbeddingStatus();
  }, []);

  const loadEmbeddingStatus = async () => {
    try {
      const status = await GetMemoryEmbeddingStatus();
      setEmbeddingStatus(status);
    } catch (error) {
      console.error('Erro ao carregar status de embeddings:', error);
    }
  };

  const handleRegenerateEmbeddings = async () => {
    if (!confirm('Deseja regenerar os embeddings de todas as memórias? Isso pode levar alguns minutos.')) return;

    try {
      await RegenerateMemoryEmbeddings();
      await loadEmbeddingStatus();
      alert(t('memory.embeddingsRegenerated', 'Embeddings regenerados com sucesso!'));
    } catch (error) {
      console.error('Erro ao regenerar embeddings:', error);
      alert('Erro ao regenerar embeddings');
    }
  };

  // Atalho Ctrl+N para nova memória
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.ctrlKey && event.key.toLowerCase() === 'n') {
        event.preventDefault();
        openNewForm();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const loadMemories = async () => {
    setLoading(true);
    try {
      const result = await GetAllMemories();
      const mapped = result.map((m: any) => ({
        id: m.id,
        title: m.title || 'Sem título',
        content: m.content,
        category: m.category || 'geral',
        created_at: m.created_at
      }));
      setMemories(mapped || []);
    } catch (error) {
      console.error('Erro ao carregar memórias:', error);
    } finally {
      setLoading(false);
    }
  };

  const openNewForm = () => {
    setEditingMemory(null);
    setFormTitle('');
    setFormContent('');
    setFormCategory('geral');
    setFormError('');
    setShowModal(true);
  };

  const openEditForm = (memory: Memory) => {
    setEditingMemory(memory);
    setFormTitle(memory.title);
    setFormContent(memory.content);
    setFormCategory(memory.category);
    setFormError('');
    setShowModal(true);
  };

  const handleSave = async () => {
    if (!formContent.trim()) {
      setFormError('Conteúdo é obrigatório');
      return;
    }

    setSaving(true);
    setFormError('');

    try {
      if (editingMemory) {
        await UpdateMemory(editingMemory.id, formContent, formCategory, formTitle);
      } else {
        await CreateMemory(formContent, formCategory, formTitle);
      }
      await loadMemories();
      setShowModal(false);
    } catch (error: any) {
      setFormError('Erro ao salvar: ' + (error.message || error));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (memory: Memory) => {
    if (!confirm(t('memory.confirmDelete', 'Tem certeza que deseja excluir esta memória?'))) return;
    
    try {
      await DeleteMemory(memory.id);
      setMemories(prev => prev.filter(m => m.id !== memory.id));
    } catch (error) {
      console.error('Erro ao deletar memória:', error);
      alert('Erro ao deletar memória');
    }
  };

  const handleDeleteSelected = async () => {
    if (selectedIds.size === 0) return;
    if (!confirm(t('memory.confirmDeleteMultiple', `Tem certeza que deseja excluir ${selectedIds.size} memória(s)?`))) return;

    try {
      await Promise.all(Array.from(selectedIds).map(id => DeleteMemory(Number(id))));
      setMemories(prev => prev.filter(m => !selectedIds.has(m.id)));
      setSelectedIds(new Set());
    } catch (error) {
      console.error('Erro ao deletar memórias:', error);
      alert('Erro ao deletar memórias');
    }
  };

  const handleExport = async () => {
    const idsToExport = selectedIds.size > 0 
      ? Array.from(selectedIds).map(id => Number(id))
      : memories.map(m => m.id);
    
    if (idsToExport.length === 0) {
      alert(t('memory.noMemoriesToExport', 'Nenhuma memória para exportar'));
      return;
    }

    try {
      const jsonData = await ExportMemories(idsToExport);
      const filename = generateFilename('memorias');
      downloadJSON(jsonData, filename);
    } catch (error) {
      console.error('Erro ao exportar memórias:', error);
      alert(t('memory.exportError', 'Erro ao exportar memórias'));
    }
  };

  const handleImport = async () => {
    try {
      const jsonData = await openFileDialog('.json');
      const result = await ImportMemories(jsonData);
      
      if (result.success) {
        alert(t('memory.importSuccess', `Importação concluída: ${result.message}`));
        loadMemories();
      } else {
        alert(t('memory.importPartial', `Importação parcial: ${result.message}\nErros: ${result.errors?.join(', ')}`));
        loadMemories();
      }
    } catch (error) {
      console.error('Erro ao importar memórias:', error);
      alert(t('memory.importError', 'Erro ao importar memórias'));
    }
  };

  const getCategoryLabel = (category: string) => {
    const cat = CATEGORIES.find(c => c.value === category);
    return cat ? cat.label : category;
  };

  const filteredMemories = memories
    .filter(mem => filterCategory === 'all' || mem.category === filterCategory)
    .filter(mem =>
      mem.title.toLowerCase().includes(searchTerm.toLowerCase()) ||
      mem.content.toLowerCase().includes(searchTerm.toLowerCase())
    );

  const columns: DataGridColumn<Memory>[] = [
    { 
      key: 'title', 
      label: 'Título',
      truncate: true,
      width: '200px',
    },
    { 
      key: 'content', 
      label: 'Conteúdo',
      truncate: true,
      format: (value) => value?.length > 100 ? value.substring(0, 100) + '...' : value
    },
    { 
      key: 'category', 
      label: 'Categoria',
      width: '180px',
      format: (value) => getCategoryLabel(value)
    },
    { 
      key: 'created_at', 
      label: 'Criada',
      width: '120px',
      format: (value) => value ? formatRelativeTime(new Date(value).getTime()) : ''
    },
    { 
      key: 'edit', 
      label: 'Editar',
      width: '80px',
      action: true,
      actionIcon: '✏️',
      actionLabel: 'Editar memória',
    },
    { 
      key: 'delete', 
      label: 'Excluir',
      width: '80px',
      action: true,
      actionIcon: '🗑️',
      actionLabel: 'Excluir memória',
    }
  ];

  const toolbarActions: ToolbarAction[] = [
    {
      key: 'new',
      label: 'Nova Memória',
      icon: '➕',
      onClick: openNewForm,
      variant: 'primary',
      shortcut: 'Ctrl+N',
    },
    {
      key: 'export',
      label: selectedIds.size > 0 
        ? `Exportar (${selectedIds.size})`
        : 'Exportar Tudo',
      icon: '📤',
      onClick: handleExport,
      variant: 'secondary',
    },
    {
      key: 'import',
      label: 'Importar',
      icon: '📥',
      onClick: handleImport,
      variant: 'secondary',
    },
    {
      key: 'regenerate',
      label: 'Regenerar Embeddings',
      icon: '🔄',
      onClick: handleRegenerateEmbeddings,
      variant: 'secondary',
      disabled: !embeddingStatus || embeddingStatus.total === 0,
    },
    ...(selectedIds.size > 0
      ? [
          {
            key: 'delete-selected',
            label: `Deletar (${selectedIds.size})`,
            icon: '🗑️',
            onClick: handleDeleteSelected,
            variant: 'danger' as const,
          },
        ]
      : []),
  ];

  if (loading) {
    return (
      <div className="memory-page">
        <Toolbar left={<h1 className="page-toolbar__title">Gerenciar Memórias</h1>} />
        <div className="page-content">
          <div className="loading-message">Carregando memórias...</div>
        </div>
      </div>
    );
  }

  return (
    <div className="memory-page">
      <Toolbar 
        left={<h1 className="page-toolbar__title">Memórias Salvas</h1>}
        actions={toolbarActions}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        searchPlaceholder="Buscar memórias..."
        onFocusGrid={focusFirstCell}
        center={
          <select 
            value={filterCategory} 
            onChange={(e) => setFilterCategory(e.target.value)}
            className="category-filter"
          >
            <option value="all">Todas categorias</option>
            {CATEGORIES.map(cat => (
              <option key={cat.value} value={cat.value}>{cat.label}</option>
            ))}
          </select>
        }
      />

      <div className="page-content">
        {embeddingStatus && (
          <div className="embedding-status">
            <span>📊 Embeddings: {embeddingStatus.with_embeddings || 0}/{embeddingStatus.total || 0}</span>
          </div>
        )}

        <DataGrid
          items={filteredMemories}
          columns={columns}
          label="Lista de memórias"
          getItemId={(memory) => memory.id}
          onActivate={(memory) => openEditForm(memory)}
          onDelete={(memory) => handleDelete(memory)}
          onCellAction={(memory, column) => {
            if (column.key === 'edit') {
              openEditForm(memory);
            } else if (column.key === 'delete') {
              handleDelete(memory);
            }
          }}
          multiSelect={true}
          selectedIds={selectedIds}
          onSelectionChange={setSelectedIds}
          onGridReady={handleGridReady}
        />
      </div>

      {showModal && (
        <SimpleModal
          isOpen={showModal}
          onClose={() => setShowModal(false)}
          title={editingMemory ? 'Editar Memória' : 'Nova Memória'}
        >
          <div className="modal-form">
            {formError && (
              <div className="form-error">{formError}</div>
            )}

            <div className="form-group">
              <label htmlFor="title">Título</label>
              <Input
                id="title"
                value={formTitle}
                onChange={(e) => setFormTitle(e.target.value)}
                placeholder="Digite um título (opcional)..."
                autoFocus
              />
            </div>

            <div className="form-group">
              <label htmlFor="content">Conteúdo *</label>
              <Textarea
                id="content"
                value={formContent}
                onChange={(e) => setFormContent(e.target.value)}
                placeholder="Digite o conteúdo da memória..."
                rows={6}
              />
            </div>

            <div className="form-group">
              <label htmlFor="category">Categoria *</label>
              <Select
                id="category"
                value={formCategory}
                onChange={(e) => setFormCategory(e.target.value)}
                options={CATEGORIES}
              />
            </div>

            <div className="modal-actions">
              <Button variant="secondary" onClick={() => setShowModal(false)}>
                Cancelar
              </Button>
              <Button variant="primary" onClick={handleSave} disabled={saving}>
                {saving ? 'Salvando...' : 'Salvar'}
              </Button>
            </div>
          </div>
        </SimpleModal>
      )}
    </div>
  );
}
