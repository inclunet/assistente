import { describe, expect, it } from 'vitest';
import type { TFunction } from 'i18next';

import { editorFileDialogLabels } from './editorDialogLabels';

const t = ((key: string) => `tr:${key}`) as unknown as TFunction;

describe('editorFileDialogLabels', () => {
  it('monta rótulos de abrir com as chaves de i18n', () => {
    expect(editorFileDialogLabels(t, 'open')).toEqual({
      title: 'tr:editor.dialog.openTitle',
      markdownFilter: 'tr:editor.dialog.filterDocuments',
      allFilesFilter: 'tr:editor.dialog.filterAll',
      defaultFilename: 'tr:editor.dialog.defaultFilename',
    });
  });

  it('monta rótulos de salvar com título distinto', () => {
    expect(editorFileDialogLabels(t, 'save')).toEqual({
      title: 'tr:editor.dialog.saveTitle',
      markdownFilter: 'tr:editor.dialog.filterMarkdown',
      allFilesFilter: 'tr:editor.dialog.filterAll',
      defaultFilename: 'tr:editor.dialog.defaultFilename',
    });
  });
});
