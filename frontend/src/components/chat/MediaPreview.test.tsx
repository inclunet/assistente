import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MediaPreview } from './MediaPreview';
import { MediaCategory, formatFileSize, type MediaFile } from '../../services/mediaService';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

describe('MediaPreview', () => {
  it('renderiza itens e permite remover', () => {
    const onRemove = vi.fn();

    const buildMedia = (overrides: Partial<MediaFile>): MediaFile => {
      const fileName = overrides.fileName || 'file.bin';
      const mimeType = overrides.mimeType || 'application/octet-stream';
      const file = overrides.file || new File(['x'], fileName, { type: mimeType });
      const extension = overrides.extension ?? (fileName.includes('.') ? fileName.slice(fileName.lastIndexOf('.')) : null);
      return {
        id: overrides.id || '1',
        file,
        category: overrides.category || MediaCategory.DOCUMENT,
        mimeType: overrides.mimeType || file.type,
        extension,
        fileName,
        fileSize: overrides.fileSize ?? file.size,
        fileSizeFormatted: overrides.fileSizeFormatted ?? formatFileSize(file.size),
        preview: overrides.preview,
        altText: overrides.altText,
        generatingAlt: overrides.generatingAlt,
        icon: overrides.icon || 'i',
      };
    };

    render(
      <MediaPreview
        media={[
          buildMedia({
            id: '1',
            category: MediaCategory.IMAGE,
            preview: 'data:image/png;base64,abc',
            fileName: 'img.png',
            icon: 'i',
            altText: 'Imagem',
            mimeType: 'image/png',
          }),
          buildMedia({
            id: '2',
            category: MediaCategory.AUDIO,
            preview: 'data:audio/mp3;base64,abc',
            fileName: 'audio.mp3',
            icon: 'a',
            mimeType: 'audio/mpeg',
          }),
          buildMedia({
            id: '3',
            category: MediaCategory.DOCUMENT,
            fileName: 'doc.pdf',
            icon: 'd',
            mimeType: 'application/pdf',
          }),
        ]}
        onRemove={onRemove}
      />
    );

    expect(screen.getAllByRole('listitem')).toHaveLength(3);

    fireEvent.click(screen.getByRole('button', { name: 'mediaPreview.remove Imagem' }));
    expect(onRemove).toHaveBeenCalledWith('1');
  });
});
