import { describe, expect, it } from 'vitest';
import { createEditorSurfaceContext, type EditorSurfaceSource } from './commandEditorSurface';

function source(overrides: Partial<EditorSurfaceSource> = {}): EditorSurfaceSource {
  return {
    surfaceType: 'editor',
    surfaceId: 'tab-a',
    workspaceTabId: 'tab-a',
    activeTabId: 'tab-a',
    documentId: 'doc-a',
    document: { id: 'doc-a', mode: 'markdown', readOnly: false, loadError: false },
    ...overrides,
  };
}

describe('editor command surface', () => {
  it.each([
    ['sem documento', { document: null }],
    ['sem relação com a aba', { documentId: 'doc-b' }],
    ['aba retargeted', { workspaceTabId: 'tab-b' }],
    ['aba não ativa', { activeTabId: 'tab-b' }],
  ])('retorna null quando há %s', (_, overrides) => {
    expect(createEditorSurfaceContext(source(overrides))).toBeNull();
  });

  it('relê a fonte e recusa troca de documento sem notificação', () => {
    let current = source();
    const getter = () => createEditorSurfaceContext(current);
    const captured = getter();
    current = source({
      documentId: 'doc-b',
      document: { id: 'doc-b', mode: 'markdown', readOnly: false, loadError: false },
    });
    const retargeted = getter();

    expect(captured?.metadata?.documentId).toBe('doc-a');
    expect(retargeted?.metadata?.documentId).toBe('doc-b');
    expect(retargeted?.snapshotVersion).not.toBe(captured?.snapshotVersion);
  });

  it('expõe somente metadados coerentes e versão sem hash do buffer', () => {
    const context = createEditorSurfaceContext(source({
      document: { id: 'doc-a', mode: 'view', readOnly: true, loadError: true },
    }));
    expect(context).toMatchObject({
      surfaceType: 'editor',
      surfaceId: 'tab-a',
      mode: 'view',
      metadata: { documentId: 'doc-a', mode: 'view', readOnly: true, loadError: true },
    });
    expect(JSON.stringify(context)).not.toContain('conteúdo do documento');
    expect(context?.snapshotVersion).toContain('doc-a');
  });
});
