import type { SurfaceContext } from './chatSurface';

export interface EditorSurfaceSource {
  readonly surfaceType: string;
  readonly surfaceId: string | null;
  readonly workspaceTabId: string | null;
  readonly activeTabId: string | null;
  readonly documentId: string | null;
  readonly document: {
    readonly id: string;
    readonly mode: string;
    readonly readOnly?: boolean;
    readonly loadError?: boolean;
  } | null;
}

function nonEmpty(value: string | null): value is string {
  return value !== null && value.trim().length > 0;
}

/**
 * Monta o contexto mínimo autoritativo do editor. A fonte é reconsultada pelo
 * getter do componente; esta factory somente valida a relação atual e não
 * transporta markdown, seleção ou conteúdo do documento.
 */
export function createEditorSurfaceContext(source: EditorSurfaceSource): SurfaceContext | null {
  if (!nonEmpty(source.surfaceType) ||
    !nonEmpty(source.surfaceId) ||
    !nonEmpty(source.workspaceTabId) ||
    !nonEmpty(source.activeTabId) ||
    !nonEmpty(source.documentId) ||
    source.surfaceId !== source.workspaceTabId ||
    source.workspaceTabId !== source.activeTabId ||
    !source.document ||
    source.document.id !== source.documentId) {
    return null;
  }

  const readOnly = source.document.readOnly === true;
  const loadError = source.document.loadError === true;
  const mode = source.document.mode;
  return {
    surfaceType: source.surfaceType,
    surfaceId: source.surfaceId,
    mode,
    metadata: {
      documentId: source.documentId,
      mode,
      readOnly,
      loadError,
    },
    snapshotVersion: [
      source.surfaceType,
      source.surfaceId,
      source.documentId,
      mode,
      readOnly ? 'readonly' : 'editable',
      loadError ? 'load-error' : 'loaded',
    ].map((part) => encodeURIComponent(part)).join(':'),
  };
}
