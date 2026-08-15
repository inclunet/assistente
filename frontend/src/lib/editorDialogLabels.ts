import type { TFunction } from 'i18next';

/**
 * Rótulos já traduzidos para os diálogos nativos do SO (abrir/salvar arquivo).
 * O backend repassa a string crua ao SO — não há i18n no Go.
 */
export interface EditorFileDialogLabels {
  title: string;
  markdownFilter: string;
  allFilesFilter: string;
  defaultFilename: string;
}

/**
 * Monta o payload de rótulos do diálogo nativo a partir de `t`.
 * kind escolhe o título (abrir vs salvar); filtros e nome padrão são compartilhados.
 */
export function editorFileDialogLabels(
  t: TFunction,
  kind: 'open' | 'save',
): EditorFileDialogLabels {
  return {
    title: t(kind === 'open' ? 'editor.dialog.openTitle' : 'editor.dialog.saveTitle'),
    markdownFilter: t('editor.dialog.filterMarkdown'),
    allFilesFilter: t('editor.dialog.filterAll'),
    defaultFilename: t('editor.dialog.defaultFilename'),
  };
}
