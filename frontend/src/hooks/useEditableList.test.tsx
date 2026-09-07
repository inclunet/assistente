/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';

import { useEditableList, type EditableListOperations } from './useEditableList';

type Item = { id: string; name: string };

let addToastMock = vi.fn();
let announceMock = vi.fn();
let confirmMock = vi.fn();

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { addToast: addToastMock };
    return selector ? selector(s) : s;
  },
}));

vi.mock('./useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceMock }),
}));

vi.mock('./useConfirm', () => ({
  useConfirm: () => confirmMock,
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, opts?: string | Record<string, unknown>) => {
      if (typeof opts === 'string') return opts;
      if (opts && typeof opts === 'object' && 'defaultValue' in opts && typeof opts.defaultValue === 'string') {
        return opts.defaultValue;
      }
      return key;
    },
  }),
}));

describe('useEditableList', () => {
  beforeEach(() => {
    addToastMock = vi.fn();
    announceMock = vi.fn();
    confirmMock = vi.fn().mockResolvedValue(true);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  const setup = (overrides?: Partial<EditableListOperations<Item>>) => {
    const operations: EditableListOperations<Item> = {
      loadItems: vi.fn().mockResolvedValue([
        { id: '1', name: 'Item 1' },
        { id: '2', name: 'Item 2' },
      ]),
      loadItem: vi.fn().mockResolvedValue({ id: '1', name: 'Item 1 (full)' }),
      createItem: vi.fn().mockResolvedValue('3'),
      updateItem: vi.fn().mockResolvedValue(undefined),
      deleteItem: vi.fn().mockResolvedValue(undefined),
      ...overrides,
    };

    const { result } = renderHook(() =>
      useEditableList<Item>(operations, {
        entityName: 'Item',
        createDefault: () => ({ id: 'new', name: 'Novo' }),
      })
    );

    return { result, operations };
  };

  it('carrega itens com loadItems', async () => {
    const { result } = setup();

    await act(async () => {
      await result.current.loadItems();
    });

    expect(result.current.items).toHaveLength(2);
    expect(result.current.items[0].name).toBe('Item 1');
  });

  it('openNew abre editor com item default', () => {
    const { result } = setup();

    act(() => {
      result.current.openNew();
    });

    expect(result.current.editingItem?.name).toBe('Novo');
    expect(result.current.isNew).toBe(true);
    expect(announceMock).toHaveBeenCalled();
  });

  it('openEdit usa loadItem quando fornecido', async () => {
    const { result, operations } = setup();

    await act(async () => {
      await result.current.openEdit({ id: '1', name: 'Item 1' });
    });

    expect(operations.loadItem).toHaveBeenCalledWith('1');
    expect(result.current.editingItem?.name).toBe('Item 1 (full)');
    expect(result.current.isNew).toBe(false);
  });

  it('save cria novo item quando isNew', async () => {
    const { result, operations } = setup();

    act(() => {
      result.current.openNew();
    });

    await act(async () => {
      await result.current.save();
    });

    expect(operations.createItem).toHaveBeenCalled();
    expect(addToastMock).toHaveBeenCalledWith('editableList.createSuccess', 'success', undefined, undefined, {
      suppressAnnounce: true,
    });
  });

  it('save respeita validação e não chama createItem', async () => {
    const createItemMock = vi.fn().mockResolvedValue('3');

    const { result } = renderHook(() =>
      useEditableList<Item>(
        {
          loadItems: vi.fn().mockResolvedValue([]),
          createItem: createItemMock,
          updateItem: vi.fn().mockResolvedValue(undefined),
          deleteItem: vi.fn().mockResolvedValue(undefined),
        },
        {
          entityName: 'Item',
          createDefault: () => ({ id: 'new', name: '' }),
          validate: () => 'Erro de validação',
        }
      )
    );

    act(() => {
      result.current.openNew();
    });

    await act(async () => {
      await result.current.save();
    });

    expect(createItemMock).not.toHaveBeenCalled();
    expect(addToastMock).toHaveBeenCalledWith('Erro de validação', 'error');
  });

  it('deleteItem respeita confirm e chama delete', async () => {
    const { result, operations } = setup();

    await act(async () => {
      await result.current.deleteItem({ id: '1', name: 'Item 1' });
    });

    expect(confirmMock).toHaveBeenCalled();
    expect(operations.deleteItem).toHaveBeenCalledWith('1');
    expect(addToastMock).toHaveBeenCalledWith('editableList.deleteSuccess', 'success', undefined, undefined, {
      suppressAnnounce: true,
    });
  });

  it('deleteItem com canDelete assíncrono e skipBuiltInDeleteConfirm não chama useConfirm', async () => {
    const deleteItemMock = vi.fn().mockResolvedValue(undefined);

    const { result } = renderHook(() =>
      useEditableList<Item>(
        {
          loadItems: vi.fn().mockResolvedValue([]),
          createItem: vi.fn().mockResolvedValue('3'),
          updateItem: vi.fn().mockResolvedValue(undefined),
          deleteItem: deleteItemMock,
        },
        {
          entityName: 'Item',
          createDefault: () => ({ id: 'new', name: '' }),
          skipBuiltInDeleteConfirm: true,
          canDelete: async () => true,
        }
      )
    );

    await act(async () => {
      await result.current.deleteItem({ id: '1', name: 'Item 1' });
    });

    expect(confirmMock).not.toHaveBeenCalled();
    expect(deleteItemMock).toHaveBeenCalledWith('1');
  });

  it('deleteItem bloqueia quando canDelete retorna string', async () => {
    const deleteItemMock = vi.fn().mockResolvedValue(undefined);

    const { result } = renderHook(() =>
      useEditableList<Item>(
        {
          loadItems: vi.fn().mockResolvedValue([]),
          createItem: vi.fn().mockResolvedValue('3'),
          updateItem: vi.fn().mockResolvedValue(undefined),
          deleteItem: deleteItemMock,
        },
        {
          entityName: 'Item',
          createDefault: () => ({ id: 'new', name: '' }),
          canDelete: () => 'Bloqueado',
        }
      )
    );

    await act(async () => {
      await result.current.deleteItem({ id: '1', name: 'Item 1' });
    });

    expect(deleteItemMock).not.toHaveBeenCalled();
    expect(addToastMock).toHaveBeenCalledWith('Bloqueado', 'error');
  });

  it('deleteItem não chama delete quando usuário cancela', async () => {
    confirmMock.mockResolvedValue(false);
    const { result, operations } = setup();

    await act(async () => {
      await result.current.deleteItem({ id: '1', name: 'Item 1' });
    });

    expect(operations.deleteItem).not.toHaveBeenCalled();
  });

  it('closeEditor limpa estado', async () => {
    const { result } = setup();

    act(() => {
      result.current.openNew();
    });

    act(() => {
      result.current.closeEditor();
    });

    await waitFor(() => {
      expect(result.current.editingItem).toBeNull();
      expect(result.current.isNew).toBe(false);
    });
  });
});
