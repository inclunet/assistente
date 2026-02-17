import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { GetConversations, DeleteConversation, UpdateConversation, ExportConversations, ImportConversations } from '../../wailsjs/go/main/App';
import { useTranslation } from 'react-i18next';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import { Toolbar } from '../components/ui/Toolbar';
import { useGridFocus } from '../hooks/useGridFocus';
import { formatRelativeTime } from '../lib/dateUtils';
import { downloadJSON, openFileDialog, generateFilename } from '../lib/exportImport';
import './HistoryPage.css';

interface Conversation {
  id: number;
  title: string;
  created_at: string;
  updated_at: string;
  message_count: number;
}

export default function HistoryPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const { focusFirstCell, handleGridReady } = useGridFocus();

  useEffect(() => {
    loadConversations();
  }, []);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.key === 'n') {
        e.preventDefault();
        handleNewConversation();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const loadConversations = async () => {
    setLoading(true);
    try {
      const result = await GetConversations();
      const mapped = result.map((c: any) => ({
        id: c.id,
        title: c.title || t('history.untitled', 'Sem título'),
        created_at: c.created_at,
        updated_at: c.updated_at,
        message_count: c.message_count || 0
      }));
      setConversations(mapped || []);
    } catch (error) {
      console.error('Erro ao carregar conversas:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleOpenConversation = (conversationId: number) => {
    navigate(`/?conversation=${conversationId}`);
  };

  const handleNewConversation = () => {
    // Navega para o chat sem ID de conversa (cria nova)
    navigate('/');
  };

  const handleDeleteConversation = async (conversationId: number) => {
    if (!confirm(t('history.confirmDelete', 'Tem certeza que deseja deletar esta conversa?'))) return;
    
    try {
      await DeleteConversation(conversationId);
      setConversations(prev => prev.filter(c => c.id !== conversationId));
      setSelectedIds(prev => {
        const newSet = new Set(prev);
        newSet.delete(conversationId);
        return newSet;
      });
    } catch (error) {
      console.error('Erro ao deletar conversa:', error);
    }
  };

  const handleDeleteSelected = async () => {
    if (selectedIds.size === 0) return;
    if (!confirm(t('history.confirmDeleteMultiple', `Tem certeza que deseja deletar ${selectedIds.size} conversa(s)?`))) return;

    try {
      await Promise.all(Array.from(selectedIds).map(id => DeleteConversation(Number(id))));
      setConversations(prev => prev.filter(c => !selectedIds.has(c.id)));
      setSelectedIds(new Set());
    } catch (error) {
      console.error('Erro ao deletar conversas:', error);
    }
  };

  const handleExport = async () => {
    const idsToExport = selectedIds.size > 0 
      ? Array.from(selectedIds).map(id => Number(id))
      : conversations.map(c => c.id);
    
    if (idsToExport.length === 0) {
      alert(t('history.noConversationsToExport', 'Nenhuma conversa para exportar'));
      return;
    }

    try {
      const jsonData = await ExportConversations(idsToExport);
      const filename = generateFilename('conversas');
      downloadJSON(jsonData, filename);
    } catch (error) {
      console.error('Erro ao exportar conversas:', error);
      alert(t('history.exportError', 'Erro ao exportar conversas'));
    }
  };

  const handleImport = async () => {
    try {
      const jsonData = await openFileDialog('.json');
      const result = await ImportConversations(jsonData);
      
      if (result.success) {
        alert(t('history.importSuccess', `Importação concluída: ${result.message}`));
        loadConversations();
      } else {
        alert(t('history.importPartial', `Importação parcial: ${result.message}\nErros: ${result.errors?.join(', ')}`));
        loadConversations();
      }
    } catch (error) {
      console.error('Erro ao importar conversas:', error);
      alert(t('history.importError', 'Erro ao importar conversas'));
    }
  };

  const filteredConversations = conversations.filter(conv =>
    conv.title.toLowerCase().includes(searchTerm.toLowerCase())
  );

  const columns: DataGridColumn<Conversation>[] = [
    {
      key: 'title',
      label: t('history.title', 'Título'),
      width: '40%',
      editable: true,
    },
    {
      key: 'message_count',
      label: t('history.messages', 'Mensagens'),
      width: '15%',
      format: (value) => String(value || 0),
    },
    {
      key: 'created_at',
      label: t('history.created', 'Criada em'),
      width: '20%',
      format: (value) => formatRelativeTime(new Date(value).getTime()),
    },
    {
      key: 'updated_at',
      label: t('history.updated', 'Atualizada em'),
      width: '20%',
      format: (value) => formatRelativeTime(new Date(value).getTime()),
    },
    {
      key: 'open',
      label: '',
      width: '2.5%',
      action: true,
      actionIcon: '📂',
      actionLabel: 'Abrir conversa',
    },
    {
      key: 'delete',
      label: '',
      width: '2.5%',
      action: true,
      actionIcon: '🗑️',
      actionLabel: 'Excluir conversa',
    },
  ];

  const handleCellAction = (item: Conversation, column: DataGridColumn<Conversation>) => {
    if (column.key === 'open') {
      handleOpenConversation(item.id);
    } else if (column.key === 'delete') {
      handleDeleteConversation(item.id);
    }
  };

  const handleCellEdit = async (item: Conversation, column: DataGridColumn<Conversation>, newValue: string) => {
    if (column.key === 'title') {
      try {
        // Atualiza no backend
        await UpdateConversation(item.id, newValue, '');
        // Atualiza no estado local
        setConversations(prev => 
          prev.map(conv => 
            conv.id === item.id ? { ...conv, title: newValue } : conv
          )
        );
      } catch (error) {
        console.error('Erro ao atualizar título:', error);
      }
    }
  };

  if (loading) {
    return (
      <div className="history-page">
        <div className="loading">Carregando histórico...</div>
      </div>
    );
  }

  return (
    <div className="history-page">
      <Toolbar
        left={<h1 className="page-toolbar__title">{t('history.pageTitle', 'Histórico de Conversas')}</h1>}
        searchPlaceholder={t('history.search', 'Buscar conversas...')}
        searchValue={searchTerm}
        onSearchChange={setSearchTerm}
        onFocusGrid={focusFirstCell}
        actions={[
          {
            key: 'new-conversation',
            label: t('history.newConversation', 'Nova Conversa'),
            icon: '➕',
            onClick: handleNewConversation,
            variant: 'primary',
            shortcut: 'Ctrl+N',
          },
          {
            key: 'export',
            label: selectedIds.size > 0 
              ? t('history.exportSelected', `Exportar (${selectedIds.size})`)
              : t('history.exportAll', 'Exportar Tudo'),
            icon: '📤',
            onClick: handleExport,
            variant: 'secondary',
          },
          {
            key: 'import',
            label: t('history.import', 'Importar'),
            icon: '📥',
            onClick: handleImport,
            variant: 'secondary',
          },
          ...(selectedIds.size > 0
            ? [
                {
                  key: 'delete-selected',
                  label: t('history.deleteSelected', `Deletar (${selectedIds.size})`),
                  onClick: handleDeleteSelected,
                  variant: 'danger' as const,
                },
              ]
            : []),
        ]}
      />

      <DataGrid
        items={filteredConversations}
        columns={columns}
        onCellAction={handleCellAction}
        onCellEdit={handleCellEdit}
        onDelete={handleDeleteConversation}
        selectedIds={selectedIds}
        multiSelect={true}
        onSelectionChange={setSelectedIds}
        onGridReady={handleGridReady}
      />
    </div>
  );
}
