import { describe, expect, it, vi } from 'vitest';
import { downloadJSON, generateFilename, openFileDialog, openImportFileDialog } from './exportImport';

describe('exportImport', () => {
  it('gera filename com prefixo', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-01T12:00:00.000Z'));

    const name = generateFilename('backup');
    expect(name).toMatch(/^backup_\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}\.json$/);

    vi.useRealTimers();
  });

  it('faz download criando link temporario', () => {
    if (!URL.createObjectURL) {
      Object.defineProperty(URL, 'createObjectURL', {
        value: () => 'blob:stub',
        writable: true,
      });
    }
    if (!URL.revokeObjectURL) {
      Object.defineProperty(URL, 'revokeObjectURL', {
        value: () => {},
        writable: true,
      });
    }
    const createUrlSpy = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:123');
    const revokeSpy = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    const appendSpy = vi.spyOn(document.body, 'appendChild');
    const removeSpy = vi.spyOn(document.body, 'removeChild');

    const clickSpy = vi.fn();
    const anchor = document.createElement('a');
    anchor.click = clickSpy;

    const originalCreateElement = document.createElement.bind(document);
    vi
      .spyOn(document, 'createElement')
      .mockImplementation((...args: Parameters<Document['createElement']>) => {
        const [tagName, options] = args;
        if (tagName === 'a') return anchor;
        return originalCreateElement(tagName, options);
      });

    downloadJSON('{"a":1}', 'file.json');

    expect(createUrlSpy).toHaveBeenCalled();
    expect(appendSpy).toHaveBeenCalled();
    expect(clickSpy).toHaveBeenCalled();
    expect(removeSpy).toHaveBeenCalled();
    expect(revokeSpy).toHaveBeenCalled();

    createUrlSpy.mockRestore();
    revokeSpy.mockRestore();
    appendSpy.mockRestore();
    removeSpy.mockRestore();
    (document.createElement as { mockRestore?: () => void }).mockRestore?.();
  });

  it('abre dialogo de arquivo e retorna conteudo', async () => {
    const input = document.createElement('input');
    const file = new File(['abc'], 'file.json', { type: 'application/json' });

    const readSpy = vi.fn();
    class MockFileReader {
      public result: string | ArrayBuffer | null = null;
      public onload: (() => void) | null = null;
      public onerror: (() => void) | null = null;
      readAsText(_f: Blob) {
        this.result = 'abc';
        readSpy();
        this.onload?.();
      }
    }

    const originalCreateElement = document.createElement.bind(document);
    vi
      .spyOn(document, 'createElement')
      .mockImplementation((...args: Parameters<Document['createElement']>) => {
        const [tagName, options] = args;
        if (tagName === 'input') return input;
        return originalCreateElement(tagName, options);
      });

    Object.defineProperty(globalThis, 'FileReader', {
      value: MockFileReader as unknown as typeof FileReader,
      configurable: true,
    });

    const promise = openFileDialog('.json');
    Object.defineProperty(input, 'files', { value: [file] });
    type InputChangeEvent = Event & { target: HTMLInputElement };
    input.onchange?.({ target: input } as InputChangeEvent);

    await expect(promise).resolves.toBe('abc');
    expect(readSpy).toHaveBeenCalled();

    (document.createElement as { mockRestore?: () => void }).mockRestore?.();
  });

  it('abre dialogo de importacao e retorna nome e conteudo', async () => {
    const input = document.createElement('input');
    const file = new File(['{"a":1}'], 'backup.json', { type: 'application/json' });

    class MockFileReader {
      public result: string | ArrayBuffer | null = null;
      public onload: (() => void) | null = null;
      public onerror: (() => void) | null = null;

      readAsText() {
        this.result = '{"a":1}';
        this.onload?.();
      }
    }

    const originalCreateElement = document.createElement.bind(document);
    vi
      .spyOn(document, 'createElement')
      .mockImplementation((...args: Parameters<Document['createElement']>) => {
        const [tagName, options] = args;
        if (tagName === 'input') return input;
        return originalCreateElement(tagName, options);
      });

    Object.defineProperty(globalThis, 'FileReader', {
      value: MockFileReader as unknown as typeof FileReader,
      configurable: true,
    });

    const promise = openImportFileDialog('.json');
    Object.defineProperty(input, 'files', { value: [file] });
    type InputChangeEvent = Event & { target: HTMLInputElement };
    input.onchange?.({ target: input } as InputChangeEvent);

    await expect(promise).resolves.toEqual({
      name: 'backup.json',
      content: '{"a":1}',
    });

    (document.createElement as { mockRestore?: () => void }).mockRestore?.();
  });
});
