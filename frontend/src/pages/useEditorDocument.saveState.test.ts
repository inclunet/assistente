import { describe, expect, it } from 'vitest';
import { apidto } from '../../wailsjs/go/models';

/**
 * Regressão do review Copilot em useEditorDocument.saveEditorState:
 * EditorState.createFrom/convertValues(asMap) muta o mapa passado.
 * O call site deve passar cópia rasa de mergeSessionsByTabId.
 */
describe('EditorState.createFrom e mutação de mapa', () => {
  const session = {
    originalPath: '/tmp/a.md',
    mineDraftId: 'mine',
    diskDraftId: 'disk',
    conflictDraftId: 'conflict',
    createdAt: 1,
  };

  it('muta o mapa original quando passado por referência', () => {
    const original: Record<string, typeof session> = { tab1: { ...session } };
    apidto.EditorState.createFrom({
      fileModeByPath: {},
      mergeSessionsByTabId: original,
    });
    expect(original.tab1).toBeInstanceOf(apidto.EditorMergeSession);
  });

  it('não muta o mapa original quando se passa cópia rasa', () => {
    const original: Record<string, typeof session> = { tab1: { ...session } };
    apidto.EditorState.createFrom({
      fileModeByPath: {},
      mergeSessionsByTabId: { ...original },
    });
    expect(original.tab1).not.toBeInstanceOf(apidto.EditorMergeSession);
    expect(original.tab1).toEqual(session);
  });
});
