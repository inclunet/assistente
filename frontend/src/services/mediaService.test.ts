/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach } from 'vitest';

import {
  detectMediaType,
  formatFileSize,
  processMediaFile,
  MediaCategory,
  createMediaPreview,
} from './mediaService';

class MockFileReader {
  onload: ((event: { target: { result: string } }) => void) | null = null;
  onerror: (() => void) | null = null;
  readAsDataURL() {
    this.onload?.({ target: { result: 'data:image/png;base64,AAA' } });
  }
}

describe('mediaService', () => {
  beforeEach(() => {
    (globalThis as unknown as { FileReader?: unknown }).FileReader = MockFileReader as never;
    globalThis.URL.createObjectURL = vi.fn(() => 'blob:preview');
  });

  it('detecta categoria por mime e extensao', () => {
    const file = new File(['x'], 'foto.png', { type: 'image/png' });
    const detection = detectMediaType(file);

    expect(detection.category).toBe(MediaCategory.IMAGE);
    expect(detection.extension).toBe('.png');
  });

  it('formata tamanho de arquivo', () => {
    expect(formatFileSize(1024)).toBe('1 KB');
  });

  it('gera preview para imagem', async () => {
    const file = new File(['x'], 'foto.png', { type: 'image/png' });

    const preview = await createMediaPreview(file, MediaCategory.IMAGE);

    expect(preview).toContain('data:image/png');
  });

  it('processa arquivo completo', async () => {
    const file = new File(['x'], 'foto.png', { type: 'image/png' });
    const media = await processMediaFile(file);

    expect(media.preview).toContain('data:image/png');
    expect(media.category).toBe(MediaCategory.IMAGE);
  });
});
