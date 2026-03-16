import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MediaUpload } from './MediaUpload';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

class MockFileReader {
  public result: string | ArrayBuffer | null = null;
  public onload: ((this: FileReader, ev: ProgressEvent<FileReader>) => void) | null = null;

  readAsDataURL(_file: Blob) {
    this.result = 'data:image/png;base64,abc';
    if (this.onload) {
      this.onload.call(this as unknown as FileReader, {} as ProgressEvent<FileReader>);
    }
  }
}

describe('MediaUpload', () => {
  beforeEach(() => {
    (globalThis as unknown as { FileReader: typeof FileReader }).FileReader = MockFileReader as unknown as typeof FileReader;
  });

  it('seleciona arquivos e permite limpar', async () => {
    const onFilesSelected = vi.fn();

    render(<MediaUpload onFilesSelected={onFilesSelected} />);

    const input = screen.getByLabelText('mediaUpload.selectFiles') as HTMLInputElement;
    const file = new File(['abc'], 'foto.png', { type: 'image/png' });

    fireEvent.change(input, { target: { files: [file] } });

    await waitFor(() => {
      expect(onFilesSelected).toHaveBeenCalled();
    });

    expect(screen.getByText('foto.png')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'mediaUpload.clearAll' }));
    expect(onFilesSelected).toHaveBeenLastCalledWith([]);
  });
});
