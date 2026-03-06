import { useState, useCallback } from 'react';
import { useAnnouncer } from './useAnnouncer';
import { useUIStore } from '../store/uiStore';

export interface EditableItem {
  id: string | number;
  [key: string]: any;
}

export interface EditableListOperations<T extends EditableItem, TCreate = T, TUpdate = T> {
  /**
   * Função para buscar lista de itens do backend
   */
  loadItems: () => Promise<T[]>;
  /**
   * Função para buscar item específico por ID (opcional, se null usa item da lista)
   */
  loadItem?: (id: string | number) => Promise<T>;
  /**
   * Função para criar novo item
   */
  createItem: (data: TCreate) => Promise<string | number>;
  /**
   * Função para atualizar item existente
   */
  updateItem: (id: string | number, data: TUpdate) => Promise<void>;
  /**
   * Função para deletar item
   */
  deleteItem: (id: string | number) => Promise<void>;
}

export interface EditableListMessages<T extends EditableItem = EditableItem> {
  loadError?: string;
  createSuccess?: string;
  createError?: string;
  updateSuccess?: string;
  updateError?: string;
  deleteSuccess?: string;
  deleteError?: string;
  deleteConfirm?: string | ((item: T) => string);
}

export interface EditableListOptions<T extends EditableItem> {
  /**
   * Nome da entidade para mensagens padrão
   */
  entityName: string;
  /**
   * Mensagens customizadas (opcional)
   */
  messages?: EditableListMessages<T>;
  /**
   * Callback de validação antes de salvar (retorna mensagem de erro ou null)
   */
  validate?: (item: T, isNew: boolean) => string | null;
  /**
   * Função para criar item default para novo registro
   */
  createDefault: () => T;
  /**
   * Callback após criar/atualizar com sucesso
   */
  onSuccess?: () => void;
  /**
   * Callback após deletar com sucesso
   */
  onDeleteSuccess?: () => void;
  /**
   * Verifica se o item pode ser deletado (ex: não deletar item ativo)
   */
  canDelete?: (item: T) => boolean | string;
}

export interface UseEditableListResult<T extends EditableItem> {
  // Estado da lista
  items: T[];
  loading: boolean;
  loadItems: () => Promise<void>;

  // Estado do editor
  editingItem: T | null;
  editingId: string | number | null;
  isNew: boolean;
  saving: boolean;

  // Ações do editor
  openNew: () => void;
  openEdit: (item: T) => Promise<void>;
  closeEditor: () => void;
  updateField: <K extends keyof T>(field: K, value: T[K]) => void;
  setEditingItem: (item: T | null) => void;
  save: () => Promise<void>;

  // Ações de item
  deleteItem: (item: T) => Promise<void>;
}

export function useEditableList<T extends EditableItem, TCreate = T, TUpdate = T>(
  operations: EditableListOperations<T, TCreate, TUpdate>,
  options: EditableListOptions<T>
): UseEditableListResult<T> {
  const { addToast } = useUIStore();
  const { announce } = useAnnouncer();

  // Estado da lista
  const [items, setItems] = useState<T[]>([]);
  const [loading, setLoading] = useState(false);

  // Estado do editor
  const [editingItem, setEditingItem] = useState<T | null>(null);
  const [editingId, setEditingId] = useState<string | number | null>(null);
  const [isNew, setIsNew] = useState(false);
  const [saving, setSaving] = useState(false);

  const messages = options.messages || {};

  // --- Lista ---

  const loadItems = useCallback(async () => {
    setLoading(true);
    try {
      const list = await operations.loadItems();
      setItems(list);
    } catch (error) {
      console.error(`Erro ao carregar ${options.entityName}:`, error);
      addToast(
        messages.loadError || `Erro ao carregar ${options.entityName}`,
        'error'
      );
    } finally {
      setLoading(false);
    }
  }, [operations, options.entityName, messages.loadError, addToast]);

  // --- Editor ---

  const openNew = useCallback(() => {
    const defaultItem = options.createDefault();
    setEditingItem(defaultItem);
    setEditingId(null);
    setIsNew(true);
    announce(`Editor aberto para novo ${options.entityName}`);
  }, [options, announce]);

  const openEdit = useCallback(async (item: T) => {
    try {
      // Se há função para carregar item completo, usa ela
      const fullItem = operations.loadItem
        ? await operations.loadItem(item.id)
        : item;

      setEditingItem(fullItem);
      setEditingId(item.id);
      setIsNew(false);
      announce(`Editor aberto para ${getName(item)}`);
    } catch (error) {
      console.error(`Erro ao carregar ${options.entityName}:`, error);
      addToast(
        messages.loadError || `Erro ao carregar ${options.entityName}`,
        'error'
      );
    }
  }, [operations, options.entityName, messages.loadError, addToast, announce]);

  const closeEditor = useCallback(() => {
    setEditingItem(null);
    setEditingId(null);
    setIsNew(false);
    announce('Editor fechado');
  }, [announce]);

  const updateField = useCallback(<K extends keyof T>(field: K, value: T[K]) => {
    setEditingItem(prev => {
      if (!prev) return null;
      return { ...prev, [field]: value };
    });
  }, []);

  const save = useCallback(async () => {
    if (!editingItem) return;

    // Validação
    if (options.validate) {
      const error = options.validate(editingItem, isNew);
      if (error) {
        addToast(error, 'error');
        return;
      }
    }

    setSaving(true);
    try {
      if (isNew) {
        const newId = await operations.createItem(editingItem as unknown as TCreate);
        addToast(
          messages.createSuccess || `${options.entityName} criado com sucesso!`,
          'success'
        );
        announce(`${getName(editingItem)} criado`);
        setIsNew(false);
        setEditingId(newId);
      } else if (editingId !== null) {
        await operations.updateItem(editingId, editingItem as unknown as TUpdate);
        addToast(
          messages.updateSuccess || `${options.entityName} atualizado com sucesso!`,
          'success'
        );
        announce(`${getName(editingItem)} atualizado`);
      }

      await loadItems();
      closeEditor();
      options.onSuccess?.();
    } catch (error: any) {
      console.error(`Erro ao salvar ${options.entityName}:`, error);
      const errorMessage = isNew
        ? messages.createError || `Erro ao criar ${options.entityName}`
        : messages.updateError || `Erro ao atualizar ${options.entityName}`;
      addToast(error.message || errorMessage, 'error');
    } finally {
      setSaving(false);
    }
  }, [
    editingItem,
    editingId,
    isNew,
    operations,
    options,
    messages,
    addToast,
    announce,
    loadItems,
    closeEditor,
  ]);

  const deleteItem = useCallback(async (item: T) => {
    // Verifica se pode deletar
    if (options.canDelete) {
      const canDelete = options.canDelete(item);
      if (typeof canDelete === 'string') {
        addToast(canDelete, 'error');
        return;
      }
      if (!canDelete) {
        addToast(`Não é possível excluir este ${options.entityName}`, 'error');
        return;
      }
    }

    // Confirmação
    const confirmMessage =
      typeof messages.deleteConfirm === 'function'
        ? messages.deleteConfirm(item)
        : messages.deleteConfirm || `Tem certeza que deseja excluir "${getName(item)}"?`;
    if (!confirm(confirmMessage)) return;

    try {
      await operations.deleteItem(item.id);
      addToast(
        messages.deleteSuccess || `${options.entityName} excluído com sucesso!`,
        'success'
      );
      announce(`${options.entityName} excluído`);

      // Fecha editor se estava editando este item
      if (editingId === item.id) {
        closeEditor();
      }

      await loadItems();
      options.onDeleteSuccess?.();
    } catch (error: any) {
      console.error(`Erro ao excluir ${options.entityName}:`, error);
      addToast(
        error.message || messages.deleteError || `Erro ao excluir ${options.entityName}`,
        'error'
      );
    }
  }, [operations, options, messages, addToast, announce, editingId, closeEditor, loadItems]);

  return {
    // Lista
    items,
    loading,
    loadItems,

    // Editor
    editingItem,
    editingId,
    isNew,
    saving,

    // Ações
    openNew,
    openEdit,
    closeEditor,
    updateField,
    setEditingItem,
    save,
    deleteItem,
  };
}

// Helper para obter nome do item
function getName(item: any): string {
  return item.name || item.title || item.slug || item.id || 'Item';
}
