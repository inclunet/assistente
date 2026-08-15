import type { TFunction } from 'i18next';
import { generateFilename } from './exportImport';

/**
 * Rótulos já traduzidos para o diálogo nativo de exportar conversas.
 * O backend repassa a string crua ao SO — não há i18n no Go.
 * markdownFilter carrega o rótulo do filtro do formato (HTML/PDF/Markdown).
 */
export interface ExportFileDialogLabels {
  title: string;
  markdownFilter: string;
  allFilesFilter: string;
  defaultFilename: string;
}

/**
 * Monta o payload de rótulos do SaveFileDialog de exportação a partir de `t`.
 */
export function exportConversationsFileDialogLabels(
  t: TFunction,
  format: string,
): ExportFileDialogLabels {
  const formatFilterKey =
    format === 'md' || format === 'markdown'
      ? 'history.exportFormat.markdown'
      : format === 'pdf'
        ? 'history.exportFormat.pdf'
        : 'history.exportFormat.html';
  const ext = format === 'markdown' ? 'md' : format;
  return {
    title: t('history.dialog.exportTitle'),
    markdownFilter: t(formatFilterKey),
    allFilesFilter: t('history.dialog.filterAll'),
    defaultFilename: generateFilename(t('history.dialog.defaultFilenamePrefix')).replace(/\.json$/, `.${ext}`),
  };
}
