import { describe, expect, it, vi } from 'vitest';
import { buildEditorDestinationSubmenu } from './editorSendMenu';

describe('editorSendMenu', () => {
  it('inclui destinos validos e novo documento com submenu de formatos', () => {
    const onSendToEditor = vi.fn();

    const items = buildEditorDestinationSubmenu({
      baseId: 'send-editor',
      editorTargets: [
        { id: 'doc-1', title: 'README.md' },
        { id: 'doc-2', title: 'Notas' },
      ],
      formats: [
        {
          id: 'markdown',
          label: 'Markdown',
          payload: {
            format: 'markdown' as const,
            title: 'Mensagem (Markdown)',
            content: 'ola',
          },
        },
        {
          id: 'plain',
          label: 'Texto',
          payload: {
            format: 'plain' as const,
            title: 'Mensagem (texto)',
            content: 'ola',
          },
        },
      ],
      onSendToEditor,
      newDocumentLabel: 'Novo documento',
      fallbackDocumentTitle: 'Editor',
    });

    expect(items.map((item) => item.label ?? item.id)).toEqual([
      'README.md',
      'Notas',
      'send-editor-separator',
      'Novo documento',
    ]);
    expect(items[0].submenu?.map((item) => item.label)).toEqual(['Markdown', 'Texto']);
    expect(items[3].submenu?.map((item) => item.label)).toEqual(['Markdown', 'Texto']);

    items[0].submenu?.[0].action?.();
    expect(onSendToEditor).toHaveBeenCalledWith({
      target: 'document',
      targetDocumentId: 'doc-1',
      format: 'markdown',
      title: 'Mensagem (Markdown)',
      content: 'ola',
    });

    items[3].submenu?.[1].action?.();
    expect(onSendToEditor).toHaveBeenLastCalledWith({
      target: 'new_document',
      format: 'plain',
      title: 'Mensagem (texto)',
      content: 'ola',
    });
  });

  it('filtra destinos com id invalido e nao adiciona separador sem abas validas', () => {
    const onSendToEditor = vi.fn();

    const items = buildEditorDestinationSubmenu({
      baseId: 'send-editor',
      editorTargets: [
        { id: '', title: 'Sem id' },
        { id: '   ', title: 'So espacos' },
      ],
      formats: [
        {
          id: 'markdown',
          label: 'Markdown',
          payload: {
            format: 'markdown' as const,
            title: 'Mensagem',
            content: 'ola',
          },
        },
      ],
      onSendToEditor,
      newDocumentLabel: 'Novo documento',
      fallbackDocumentTitle: 'Editor',
    });

    expect(items).toHaveLength(1);
    expect(items[0].label).toBe('Novo documento');
    expect(items.some((item) => item.separator)).toBe(false);
  });
});
