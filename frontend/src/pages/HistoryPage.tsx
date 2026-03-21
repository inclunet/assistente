import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { GetConversations, DeleteConversation, UpdateConversation, ExportConversations, ImportConversations, SearchConversationHistory } from '@wailsjs/go/main/App';
import { useTranslation } from 'react-i18next';
import { DataGrid, DataGridColumn } from '../components/ui/DataGrid';
import type { MenuItem as ContextMenuItem } from '../components/menu';
import { MenuButton } from '../components/layout/MenuButton';
import { Toolbar } from '../components/ui/Toolbar';
import { useGridFocus } from '../hooks/useGridFocus';
import { useGridPageLandmarks } from '../hooks/useGridPageLandmarks';
import { useWorkspaceStore } from '../store/workspaceStore';
import { formatRelativeTime } from '../lib/dateUtils';
import { downloadJSON, openFileDialog, generateFilename } from '../lib/exportImport';
import './HistoryPage.css';

interface Conversation {
  id: number;
  title: string;
  created_at: string;
  updated_at: string;
  message_count: number;
  snippet?: string;
}

export default function HistoryPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchTerm, setSearchTerm] = useState('');
  const [selectedIds, setSelectedIds] = useState<Set<string | number>>(new Set());
  const [searchResultIds, setSearchResultIds] = useState<Set<number> | null>(null);
  const [snippetsMap, setSnippetsMap] = useState<Map<number, string>>(new Map());
  const [searching, setSearching] = useState(false);
  const searchDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [focusedRow, setFocusedRow] = useState<Conversation | null>(null);
  const { handleGridReady } = useGridFocus();
  useGridPageLandmarks({ pageClass: 'history-page' });
  const addWorkspaceTab = useWorkspaceStore(state => state.addTab);

  useEffect(() => {
    loadConversations();
  }, []);

  const doSearch = useCallback(async (query: string) => {
    if (!query.trim()) {
      setSearchResultIds(null);
      setSnippetsMap(new Map());
      return;
    }
    setSearching(true);
    try {
      const results = await SearchConversationHistory(query, 50);
      if (!results || results.length === 0) {
        setSearchResultIds(new Set());
        setSnippetsMap(new Map());
      } else {
        const ids = new Set<number>();
        const snippets = new Map<number, string>();
        for (const r of results) {
          ids.add(r.conversation_id);
          if (!snippets.has(r.conversation_id)) {
            const snippet = (r.snippet || '')
              .replace(/>>>/g, '\u00AB').replace(/<<</g, '\u00BB');
            snippets.set(r.conversation_id, snippet);
          }
        }
        setSearchResultIds(ids);
        setSnippetsMap(snippets);
      }
    } catch (error) {
      console.error('Erro na busca:', error);
      setSearchResultIds(new Set());
      setSnippetsMap(new Map());
    } finally {
      setSearching(false);
    }
  }, []);

  useEffect(() => {
    if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current);
    if (!searchTerm.trim()) {
      setSearchResultIds(null);
      setSnippetsMap(new Map());
      return;
    }
    searchDebounceRef.current = setTimeout(() => doSearch(searchTerm), 300);
    return () => { if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current); };
  }, [searchTerm, doSearch]);

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
      const mapped = (result || []).map((c: Conversation) => ({
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

  const handleOpenConversation = async (conversationId: number, title?: string) => {
    try {
      await addWorkspaceTab('chat', String(conversationId), title || t('chat.newConversation', 'Nova conversa'));
      navigate('/');
    } catch (error) {
      console.error('Erro ao abrir conversa:', error);
    }
  };

  const handleNewConversation = () => {
    navigate('/');
  };

  const handleDeleteConversation = useCallback(async (conversationId: number) => {
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
  }, [t]);

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

  const handleDeleteAction = useCallback(() => {
    if (selectedIds.size > 0) {
      handleDeleteSelected();
      return;
    }
    if (focusedRow) {
      handleDeleteConversation(focusedRow.id);
    }
  }, [focusedRow, handleDeleteConversation, handleDeleteSelected, selectedIds.size]);

  const displayItems = useMemo(() => {
    if (searchResultIds === null) return conversations;
    return conversations.filter(c => searchResultIds.has(c.id));
  }, [conversations, searchResultIds]);

  const handleFocusChange = useCallback((item: Conversation | null) => {
    setFocusedRow(item);
  }, []);

  const handleDeleteRow = useCallback((item: Conversation) => {
    handleDeleteConversation(item.id);
  }, [handleDeleteConversation]);

  const getRowActions = useCallback(
    (item: Conversation): ContextMenuItem[] => [
      {
        id: 'open',
        label: t('history.openConversation', 'Abrir conversa'),
        icon: '📂',
        action: () => handleOpenConversation(item.id, item.title),
      },
      {
        id: 'delete',
        label: t('history.deleteConversation', 'Excluir conversa'),
        icon: '🗑️',
        action: () => handleDeleteConversation(item.id),
      },
    ],
    [handleDeleteConversation, handleOpenConversation, t]
  );

  const getMenuButtonItems = useCallback(
    (item: Conversation) =>
      getRowActions(item).map((action) => ({
        id: action.id,
        label: action.label ?? '',
        icon: action.icon ?? '',
        shortcut: action.shortcut,
        onClick: action.action,
        submenu: action.submenu
          ? action.submenu.map((submenuItem) => ({
              id: submenuItem.id,
              label: submenuItem.label ?? '',
              icon: submenuItem.icon ?? '',
              shortcut: submenuItem.shortcut,
              onClick: submenuItem.action,
            }))
          : undefined,
      })),
    [getRowActions]
  );

  const columns: DataGridColumn<Conversation>[] = [
    {
      key: 'title',
      label: t('history.title', 'Título'),
      width: '40%',
      editable: true,
      format: (_value, item) => {
        const snippet = snippetsMap.get(item.id);
        if (snippet) {
          return (
            <span className="history-page__title-cell">
              <span className="history-page__title-text">{item.title}</span>
              <span className="history-page__title-snippet">{snippet}</span>
            </span>
          );
        }
        return item.title;
      },
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
      format: (value) => {
        const dateValue = value instanceof Date || typeof value === 'string' || typeof value === 'number'
          ? value
          : undefined;
        const timestamp = dateValue ? new Date(dateValue).getTime() : 0;
        return formatRelativeTime(timestamp);
      },
    },
    {
      key: 'updated_at',
      label: t('history.updated', 'Atualizada em'),
      width: '20%',
      format: (value) => {
        const dateValue = value instanceof Date || typeof value === 'string' || typeof value === 'number'
          ? value
          : undefined;
        const timestamp = dateValue ? new Date(dateValue).getTime() : 0;
        return formatRelativeTime(timestamp);
      },
    },
    {
      key: 'actions',
      label: '',
      width: '5%',
      format: (_value, item) => (
        <MenuButton
          items={getMenuButtonItems(item)}
          buttonLabel={t('history.actions', 'Ações')}
        />
      ),
    },
  ];

  // handleCellAction não é mais necessário, pois as ações estão no MenuButton

  const handleCellEdit = useCallback(async (item: Conversation, column: DataGridColumn<Conversation>, newValue: string) => {
    if (column.key === 'title') {
      try {
        await UpdateConversation(item.id, newValue, '');
        setConversations(prev =>
          prev.map(conv =>
            conv.id === item.id ? { ...conv, title: newValue } : conv
          )
        );
      } catch (error) {
        console.error('Erro ao atualizar título:', error);
      }
    }
  }, []);

  if (loading) {
    return (
      <div className="history-page">
        <div className="loading">{t('history.loading', 'Carregando histórico...')}</div>
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
            key: 'open-conversation',
            label: t('history.openConversation', 'Abrir conversa'),
            icon: '📂',
            onClick: () => focusedRow && handleOpenConversation(focusedRow.id, focusedRow.title),
            disabled: !focusedRow,
          },
          {
            key: 'delete',
            label: selectedIds.size > 0
              ? t('history.deleteSelected', `Deletar (${selectedIds.size})`)
              : t('history.deleteConversation', 'Excluir conversa'),
            icon: '🗑️',
            onClick: handleDeleteAction,
            disabled: selectedIds.size === 0 && !focusedRow,
            variant: 'danger',
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
        ]}
      />

      {searching && (
        <div className="history-page__search-status">{t('history.searching', 'Buscando...')}</div>
      )}

      <DataGrid
        items={displayItems}
        columns={columns}
        onCellEdit={handleCellEdit}
        onDelete={handleDeleteRow}
        selectedIds={selectedIds}
        multiSelect={true}
        onSelectionChange={setSelectedIds}
        onGridReady={handleGridReady}
        onFocusChange={handleFocusChange}
        getRowActions={getRowActions}
      />
    </div>
  );
}
