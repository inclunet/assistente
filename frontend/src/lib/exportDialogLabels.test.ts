import { describe, expect, it } from 'vitest';
import type { TFunction } from 'i18next';
import { exportConversationsFileDialogLabels } from './exportDialogLabels';

const t = ((key: string) => `tr:${key}`) as unknown as TFunction;

describe('exportConversationsFileDialogLabels', () => {
  it('monta rótulos traduzidos e nome padrão com extensão do formato', () => {
    const labels = exportConversationsFileDialogLabels(t, 'md');
    expect(labels.title).toBe('tr:history.dialog.exportTitle');
    expect(labels.markdownFilter).toBe('tr:history.exportFormat.markdown');
    expect(labels.allFilesFilter).toBe('tr:history.dialog.filterAll');
    expect(labels.defaultFilename).toMatch(/^tr:history\.dialog\.defaultFilenamePrefix_.+\.md$/);
  });
});
