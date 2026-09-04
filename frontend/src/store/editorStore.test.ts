import { describe, it, expect, beforeEach } from 'vitest';
import {
  normalizeEditorMode,
  preferLiveEditorDocument,
  resolveEditorDisplayMode,
  useEditorStore,
} from './editorStore';

function resetStore() {
  useEditorStore.setState({ documents: {}, pendingInsert: null });
}

describe('editorStore — filePath lifecycle', () => {
  beforeEach(resetStore);

  it('createDocument armazena filePath quando fornecido', () => {
    const id = useEditorStore.getState().createDocument({
      id: 'doc-1',
      title: 'readme.md',
      filePath: '/home/user/readme.md',
    });

    const doc = useEditorStore.getState().documents[id];
    expect(doc).toBeDefined();
    expect(doc.filePath).toBe('/home/user/readme.md');
    expect(doc.draftId).toBeNull(); // tem filePath → sem draft
  });

  it('createDocument sem filePath gera draftId', () => {
    const id = useEditorStore.getState().createDocument({ id: 'doc-2', title: 'Novo' });

    const doc = useEditorStore.getState().documents[id];
    expect(doc.filePath).toBeNull();
    expect(doc.draftId).toBe('doc-2');
  });

  it('setDocFilePath atualiza filePath de documento existente', () => {
    useEditorStore.getState().createDocument({ id: 'doc-3', title: 'Sem arquivo' });
    expect(useEditorStore.getState().documents['doc-3'].filePath).toBeNull();

    useEditorStore.getState().setDocFilePath('doc-3', '/tmp/saved.md');
    expect(useEditorStore.getState().documents['doc-3'].filePath).toBe('/tmp/saved.md');
  });

  it('setDocFilePath com null limpa filePath', () => {
    useEditorStore.getState().createDocument({ id: 'doc-4', title: 'Arquivo', filePath: '/a/b.md' });
    useEditorStore.getState().setDocFilePath('doc-4', null);
    expect(useEditorStore.getState().documents['doc-4'].filePath).toBeNull();
  });

  it('setDocFilePath em documento inexistente não altera estado', () => {
    const before = useEditorStore.getState().documents;
    useEditorStore.getState().setDocFilePath('nao-existe', '/x.md');
    expect(useEditorStore.getState().documents).toBe(before);
  });

  it('removeDocument limpa documento sem coordenar singleton ativo', () => {
    useEditorStore.getState().createDocument({ id: 'doc-5', title: 'A', filePath: '/a.md' });

    useEditorStore.getState().removeDocument('doc-5');
    expect(useEditorStore.getState().documents['doc-5']).toBeUndefined();
  });

  it('renameDocument atualiza título sem alterar filePath', () => {
    useEditorStore.getState().createDocument({ id: 'doc-6', title: 'old.md', filePath: '/old.md' });
    useEditorStore.getState().renameDocument('doc-6', 'new.md');

    const doc = useEditorStore.getState().documents['doc-6'];
    expect(doc.title).toBe('new.md');
    expect(doc.filePath).toBe('/old.md'); // filePath inalterado
  });

  it('setDocDraftId atualiza draftId independente de filePath', () => {
    useEditorStore.getState().createDocument({ id: 'doc-7', title: 'Draft', filePath: '/x.md' });
    useEditorStore.getState().setDocDraftId('doc-7', 'draft-uuid');
    expect(useEditorStore.getState().documents['doc-7'].draftId).toBe('draft-uuid');
    expect(useEditorStore.getState().documents['doc-7'].filePath).toBe('/x.md');
  });

  it('hydrate restaura documents por completo', () => {
    const docs = {
      'd1': { id: 'd1', title: 'A', markdown: '# A', mode: 'markdown' as const, filePath: '/a.md', draftId: null },
      'd2': { id: 'd2', title: 'B', markdown: '# B', mode: 'rich' as const, filePath: null, draftId: 'd2' },
    };
    useEditorStore.getState().hydrate({ documents: docs });

    expect(useEditorStore.getState().documents['d1'].filePath).toBe('/a.md');
    expect(useEditorStore.getState().documents['d2'].filePath).toBeNull();
  });

  it('normaliza somente modos de exibição suportados', () => {
    expect(normalizeEditorMode('view')).toBe('view');
    expect(normalizeEditorMode('rich')).toBe('rich');
    expect(normalizeEditorMode('desconhecido', 'markdown')).toBe('markdown');
  });

  it('restaura displayMode da aba antes do fallback legado', () => {
    expect(resolveEditorDisplayMode('view', 'rich', false)).toBe('view');
    expect(resolveEditorDisplayMode(undefined, 'rich', false)).toBe('rich');
    expect(resolveEditorDisplayMode('markdown', 'rich', true)).toBe('view');
  });

  it('substitui apenas documento provisório durante hidratação posterior', () => {
    const loaded = {
      id: 'tab-1',
      title: 'Disco',
      markdown: '# Disco',
      mode: 'view' as const,
      sessionHydrated: true,
    };
    const provisional = {
      ...loaded,
      title: 'Provisório',
      mode: 'markdown' as const,
      sessionHydrated: false,
    };
    const live = {
      ...loaded,
      title: 'Editado',
      markdown: '# Alterado',
    };
    const editedProvisional = {
      ...provisional,
      markdown: '# Alteração local',
      hasLocalChanges: true,
    };

    expect(preferLiveEditorDocument(loaded, provisional)).toBe(loaded);
    expect(preferLiveEditorDocument(loaded, live)).toBe(live);
    expect(preferLiveEditorDocument(loaded, editedProvisional)).toEqual({
      ...editedProvisional,
      sessionHydrated: true,
    });
  });

});

describe('editorStore — projeção somente leitura', () => {
  beforeEach(resetStore);

  it('força modo view e impede alternância para editores', () => {
    useEditorStore.getState().createDocument({
      id: 'manual',
      title: 'manual.docx',
      markdown: '# Manual',
      mode: 'view',
      readOnly: true,
      projection: { format: 'docx', warnings: [] },
    });

    useEditorStore.getState().setDocMode('manual', 'markdown');
    useEditorStore.getState().toggleDocMode('manual');

    const document = useEditorStore.getState().documents.manual;
    expect(document.mode).toBe('view');
    expect(document.readOnly).toBe(true);
    expect(document.projection?.format).toBe('docx');
  });

  it('limpa o erro e restaura edição quando uma releitura textual funciona', () => {
    useEditorStore.getState().createDocument({
      id: 'recuperado',
      title: 'arquivo.md',
      mode: 'view',
      readOnly: true,
      loadError: true,
    });

    useEditorStore.getState().setDocProjection('recuperado', null);

    const document = useEditorStore.getState().documents.recuperado;
    expect(document.loadError).toBe(false);
    expect(document.readOnly).toBe(false);
    expect(document.mode).toBe('markdown');
  });

  it('restaura modo editável quando uma projeção passa a ser texto', () => {
    useEditorStore.getState().createDocument({
      id: 'convertido',
      title: 'arquivo.dat',
      mode: 'view',
      readOnly: true,
      projection: { format: 'pdf', warnings: [] },
    });

    useEditorStore.getState().setDocProjection('convertido', null);

    const document = useEditorStore.getState().documents.convertido;
    expect(document.projection).toBeNull();
    expect(document.readOnly).toBe(false);
    expect(document.mode).toBe('markdown');
  });
});
