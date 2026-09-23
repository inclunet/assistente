import { act, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { axe } from '../../test/a11yAxe';
import {
  CommandPresentationEditor,
  isCommandPresentationValid,
  normalizeCommandPresentation,
} from './CommandPresentationEditor';

const announce = vi.hoisted(() => vi.fn());
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce }) }));

class DeferredFileReader {
  static instances: DeferredFileReader[] = [];
  result: string | ArrayBuffer | null = null;
  onload: ((event: ProgressEvent<FileReader>) => void) | null = null;
  onerror: ((event: ProgressEvent<FileReader>) => void) | null = null;
  onabort: ((event: ProgressEvent<FileReader>) => void) | null = null;

  constructor() {
    DeferredFileReader.instances.push(this);
  }

  readAsDataURL() { /* controlled by the test */ }

  abort() {
    this.onabort?.(new ProgressEvent('abort') as ProgressEvent<FileReader>);
  }

  finish(result: string) {
    this.result = result;
    this.onload?.(new ProgressEvent('load') as ProgressEvent<FileReader>);
  }
}

afterEach(() => {
  DeferredFileReader.instances = [];
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe('CommandPresentationEditor', () => {
  it('oferece seletor de ícone e três campos nomeados atualizáveis por teclado', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    function Harness() {
      const [value, setValue] = useState<Record<string, unknown>>({ version: 1 });
      return <CommandPresentationEditor value={value} onChange={next => { setValue(next); onChange(next); }} />;
    }
    const { container } = render(
      <Harness />
    );

    const icon = screen.getByRole('combobox', { name: 'commandSettings.presentation.icon.label' });
    const portuguese = screen.getByRole('textbox', { name: 'commandSettings.presentation.locales.ptBR' });
    await user.tab();
    expect(icon).toHaveFocus();
    await user.selectOptions(icon, 'folder');
    await user.tab();
    expect(icon).toHaveValue('folder');
    expect(onChange).toHaveBeenLastCalledWith({ version: 1, icon: 'folder' });
    expect(portuguese).toHaveFocus();
    await user.keyboard('  Configurações  ');

    expect(portuguese).toHaveFocus();
    expect(onChange).toHaveBeenCalledWith({
      version: 1,
      icon: 'folder',
      title_by_locale: { 'pt-BR': '  Configurações  ' },
    });
    await user.tab();
    expect(screen.getByRole('textbox', { name: 'commandSettings.presentation.locales.en' })).toHaveFocus();
    await user.keyboard('Settings');
    await user.tab();
    expect(screen.getByRole('textbox', { name: 'commandSettings.presentation.locales.es' })).toHaveFocus();
    await user.tab({ shift: true });
    expect(screen.getByRole('textbox', { name: 'commandSettings.presentation.locales.en' })).toHaveFocus();
    expect(await axe(container)).toHaveNoViolations();
  });

  it('bloqueia NUL e títulos acima de 256 codepoints', () => {
    const { rerender } = render(
      <CommandPresentationEditor
        value={{ version: 1, title_by_locale: { en: 'ok' } }}
        onChange={vi.fn()}
      />
    );

    rerender(
      <CommandPresentationEditor
        value={{ version: 1, title_by_locale: { en: 'bad\0title' } }}
        onChange={vi.fn()}
      />
    );
    expect(isCommandPresentationValid({ title_by_locale: { en: 'bad\0title' } })).toBe(false);
    expect(screen.getByText('commandSettings.presentation.invalidNul')).toBeInTheDocument();

    rerender(
      <CommandPresentationEditor
        value={{ version: 1, title_by_locale: { en: '😀'.repeat(257) } }}
        onChange={vi.fn()}
      />
    );
    expect(isCommandPresentationValid({ title_by_locale: { en: '😀'.repeat(257) } })).toBe(false);
    expect(screen.getByText('commandSettings.presentation.invalidLength')).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'commandSettings.presentation.locales.en' })).toHaveAttribute('aria-invalid', 'true');
    expect(isCommandPresentationValid({ title_by_locale: { en: ' 😀'.trim().repeat(256) } })).toBe(true);
  });

  it('remove locale vazio, recorta títulos e preserva outros metadados', () => {
    const presentation = {
      version: 1,
      icon: 'settings',
      title_by_locale: { 'pt-BR': '  Configurações  ', en: '' },
    };
    expect(normalizeCommandPresentation(presentation)).toEqual({
      version: 1,
      icon: 'settings',
      title_by_locale: { 'pt-BR': 'Configurações' },
    });
  });

  it('preserva um ícone legado desconhecido até o usuário substituí-lo ou removê-lo', () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <CommandPresentationEditor
        value={{ version: 1, icon: 'legacy-deck-token', status_label_keys: { active: 'status.active' } }}
        onChange={onChange}
      />
    );

    const icon = screen.getByRole('combobox', { name: 'commandSettings.presentation.icon.label' });
    expect(icon).toHaveValue('legacy-deck-token');
    expect(screen.getByRole('option', { name: 'commandSettings.presentation.icon.unavailable' })).toHaveValue('legacy-deck-token');
    fireEvent.change(icon, { target: { value: 'folder' } });
    expect(onChange).toHaveBeenLastCalledWith({ version: 1, icon: 'folder', status_label_keys: { active: 'status.active' } });
    rerender(
      <CommandPresentationEditor
        value={{ version: 1, icon: 'folder', status_label_keys: { active: 'status.active' } }}
        onChange={onChange}
      />
    );
    fireEvent.change(screen.getByRole('combobox', { name: 'commandSettings.presentation.icon.label' }), { target: { value: '' } });
    expect(onChange).toHaveBeenLastCalledWith({ version: 1, status_label_keys: { active: 'status.active' } });
  });

  it('lê PNG como base64 sem prefixo, substitui image_ref e preserva título e ícone', async () => {
    vi.stubGlobal('FileReader', DeferredFileReader);
    const onChange = vi.fn();
    const onBusyChange = vi.fn();
    function Harness() {
      const [value, setValue] = useState<Record<string, unknown>>({
        version: 1,
        icon: 'folder',
        image_ref: 'sha256-do-backend',
        title_by_locale: { en: 'Keep me' },
      });
      return <CommandPresentationEditor value={value} onChange={next => { setValue(next); onChange(next); }} onBusyChange={onBusyChange} />;
    }
    render(<Harness />);

    fireEvent.change(screen.getByLabelText('commandSettings.presentation.image.label'), {
      target: { files: [new File(['bytes'], 'ignored-name.png', { type: 'image/png' })] },
    });
    expect(onBusyChange).toHaveBeenLastCalledWith(true);
    expect(screen.getByRole('button', { name: 'commandSettings.presentation.image.remove' })).toBeEnabled();

    await act(async () => DeferredFileReader.instances[0].finish('data:image/png;base64,Ynl0ZXM='));

    expect(onChange).toHaveBeenCalledWith({
      version: 1,
      icon: 'folder',
      title_by_locale: { en: 'Keep me' },
      image_upload: 'Ynl0ZXM=',
    });
    expect(onBusyChange).toHaveBeenLastCalledWith(false);
    expect(screen.getByText('commandSettings.presentation.image.selected')).toBeInTheDocument();
    expect(announce).toHaveBeenCalledExactlyOnceWith('commandSettings.presentation.image.selected');
  });

  it('recusa formato e tamanho no frontend com texto e anúncio assertivo', () => {
    vi.stubGlobal('FileReader', DeferredFileReader);
    const { rerender } = render(
      <CommandPresentationEditor value={{ version: 1 }} onChange={vi.fn()} />
    );
    const input = screen.getByLabelText('commandSettings.presentation.image.label');
    fireEvent.change(input, { target: { files: [new File(['text'], 'image.gif', { type: 'image/gif' })] } });
    expect(screen.getByText('commandSettings.presentation.image.invalidFormat')).toBeInTheDocument();
    expect(announce).toHaveBeenCalledExactlyOnceWith('commandSettings.presentation.image.invalidFormat', 'assertive');
    expect(DeferredFileReader.instances).toHaveLength(0);

    const oversized = new File([new Uint8Array(1024 * 1024 + 1)], 'large.png', { type: 'image/png' });
    announce.mockClear();
    rerender(<CommandPresentationEditor value={{ version: 1 }} onChange={vi.fn()} />);
    fireEvent.change(screen.getByLabelText('commandSettings.presentation.image.label'), { target: { files: [oversized] } });
    expect(screen.getByText('commandSettings.presentation.image.tooLarge')).toBeInTheDocument();
    expect(announce).toHaveBeenCalledExactlyOnceWith('commandSettings.presentation.image.tooLarge', 'assertive');
  });

  it.each(['error', 'throw', 'null', 'missing-prefix', 'empty'] as const)(
    'Input anuncia exatamente uma vez a falha de leitura: %s',
    async (failure) => {
      vi.stubGlobal('FileReader', DeferredFileReader);
      const read = vi.spyOn(DeferredFileReader.prototype, 'readAsDataURL');
      if (failure === 'throw') read.mockImplementationOnce(() => { throw new Error('read failed'); });
      const onChange = vi.fn();
      const onBusyChange = vi.fn();
      const view = render(<CommandPresentationEditor onChange={onChange} onBusyChange={onBusyChange} />);
      try {
        fireEvent.change(screen.getByLabelText('commandSettings.presentation.image.label'), {
          target: { files: [new File(['image'], 'image.png', { type: 'image/png' })] },
        });
        const reader = DeferredFileReader.instances[0];
        await act(async () => {
          if (failure === 'error') reader.onerror?.(new ProgressEvent('error') as ProgressEvent<FileReader>);
          if (failure === 'null') reader.onload?.(new ProgressEvent('load') as ProgressEvent<FileReader>);
          if (failure === 'missing-prefix') reader.finish('invalid-result');
          if (failure === 'empty') reader.finish('data:image/png;base64,');
        });
        expect(screen.getByText('commandSettings.presentation.image.readError')).toBeInTheDocument();
        expect(announce).toHaveBeenCalledExactlyOnceWith('commandSettings.presentation.image.readError', 'assertive');
        expect(onChange).not.toHaveBeenCalled();
        expect(onBusyChange).toHaveBeenLastCalledWith(false);
        view.rerender(<CommandPresentationEditor onChange={onChange} onBusyChange={onBusyChange} />);
        expect(announce).toHaveBeenCalledTimes(1);
      } finally {
        read.mockRestore();
      }
    },
  );

  it('cancela uma leitura obsoleta e não perde título editado durante a leitura vigente', async () => {
    vi.stubGlobal('FileReader', DeferredFileReader);
    const onChange = vi.fn();
    function Harness() {
      const [value, setValue] = useState<Record<string, unknown>>({
        version: 1,
        image_ref: 'old-ref',
        title_by_locale: { en: 'Antes' },
      });
      return <CommandPresentationEditor value={value} onChange={next => { setValue(next); onChange(next); }} />;
    }
    render(<Harness />);
    const input = screen.getByLabelText('commandSettings.presentation.image.label');
    fireEvent.change(input, { target: { files: [new File(['one'], 'one.png', { type: 'image/png' })] } });
    fireEvent.change(input, { target: { files: [new File(['two'], 'two.jpg', { type: 'image/jpeg' })] } });
    expect(DeferredFileReader.instances[0]).toBeDefined();
    expect(DeferredFileReader.instances[1]).toBeDefined();

    fireEvent.change(screen.getByLabelText('commandSettings.presentation.locales.en'), { target: { value: 'Editado' } });
    await act(async () => DeferredFileReader.instances[0].finish('data:image/png;base64,first'));
    expect(onChange).not.toHaveBeenCalledWith(expect.objectContaining({ image_upload: 'first' }));
    await act(async () => DeferredFileReader.instances[1].finish('data:image/jpeg;base64,second'));
    expect(onChange).toHaveBeenLastCalledWith({
      version: 1,
      title_by_locale: { en: 'Editado' },
      image_upload: 'second',
    });
  });

  it('remove upload e ref sem perder título nem ícone e não exibe hash salvo', () => {
    const onChange = vi.fn();
    render(
      <CommandPresentationEditor
        value={{ version: 1, icon: 'star', image_ref: 'sha256-secret', image_upload: 'transient', title_by_locale: { es: 'Título' } }}
        onChange={onChange}
      />
    );

    expect(screen.getByText('commandSettings.presentation.image.selected')).toBeInTheDocument();
    expect(screen.queryByText('sha256-secret')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.presentation.image.remove' }));
    expect(onChange).toHaveBeenLastCalledWith({
      version: 1,
      icon: 'star',
      title_by_locale: { es: 'Título' },
    });
    expect(announce).toHaveBeenCalledExactlyOnceWith('commandSettings.presentation.image.removed');
  });

  it('descarta leitura stale ao remover a imagem durante o FileReader', async () => {
    vi.stubGlobal('FileReader', DeferredFileReader);
    const onChange = vi.fn();
    render(
      <CommandPresentationEditor
        value={{ version: 1, image_ref: 'old-ref', title_by_locale: { en: 'Título' } }}
        onChange={onChange}
      />
    );
    fireEvent.change(screen.getByLabelText('commandSettings.presentation.image.label'), {
      target: { files: [new File(['new'], 'new.png', { type: 'image/png' })] },
    });
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.presentation.image.remove' }));
    await act(async () => DeferredFileReader.instances[0].finish('data:image/png;base64,stale'));
    expect(onChange).toHaveBeenCalledExactlyOnceWith({ version: 1, title_by_locale: { en: 'Título' } });
  });

  it('descarta leitura stale ao desabilitar ou desmontar o editor', async () => {
    vi.stubGlobal('FileReader', DeferredFileReader);
    const onChange = vi.fn();
    function Harness() {
      const [disabled, setDisabled] = useState(false);
      return <>
        <CommandPresentationEditor value={{ version: 1 }} disabled={disabled} onChange={onChange} />
        <button type="button" onClick={() => setDisabled(true)}>disable</button>
      </>;
    }
    const { unmount } = render(<Harness />);
    fireEvent.change(screen.getByLabelText('commandSettings.presentation.image.label'), {
      target: { files: [new File(['new'], 'new.png', { type: 'image/png' })] },
    });
    fireEvent.click(screen.getByRole('button', { name: 'disable' }));
    await act(async () => DeferredFileReader.instances[0].finish('data:image/png;base64,disabled-stale'));
    expect(onChange).not.toHaveBeenCalled();

    const second = render(<CommandPresentationEditor value={{ version: 1 }} onChange={onChange} />);
    const imageInputs = screen.getAllByLabelText('commandSettings.presentation.image.label');
    fireEvent.change(imageInputs[imageInputs.length - 1], {
      target: { files: [new File(['new'], 'new.png', { type: 'image/png' })] },
    });
    const reader = DeferredFileReader.instances[DeferredFileReader.instances.length - 1];
    second.unmount();
    await act(async () => reader.finish('data:image/png;base64,unmounted-stale'));
    expect(onChange).not.toHaveBeenCalled();
    unmount();
  });

  it('respeita disabled e busy sem criar outra região viva', () => {
    vi.stubGlobal('FileReader', DeferredFileReader);
    const onBusyChange = vi.fn();
    render(<CommandPresentationEditor value={{ version: 1 }} disabled onBusyChange={onBusyChange} onChange={vi.fn()} />);
    expect(screen.getByLabelText('commandSettings.presentation.image.label')).toBeDisabled();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });
});
