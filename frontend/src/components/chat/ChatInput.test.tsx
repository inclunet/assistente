import { useState } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, createEvent, render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ChatInput } from './ChatInput';
import type { MediaFile } from '../../services/mediaService';

const getSkillsForProfileSpy = vi.fn();
const processMediaFilesSpy = vi.fn();
const announceSpy = vi.fn();
const voiceState = vi.hoisted(() => ({ transcribe: undefined as ((text: string) => void) | undefined }));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceSpy }),
}));

vi.mock('@wailsjs/go/wailsapi/Skills', () => ({
  GetUserInvocableSkillsForProfile: (profileSlug: string) => getSkillsForProfileSpy(profileSlug),
}));

vi.mock('../../services/mediaService', () => ({
  processMediaFiles: (files: File[]) => processMediaFilesSpy(files),
}));

vi.mock('./SlashCommandMenu', () => ({
  SlashCommandMenu: ({ skills, listboxId }: { skills: Array<{ name: string }>; listboxId: string }) => (
    <div data-testid="slash-menu" id={listboxId} role="listbox">
      {skills.map((skill) => skill.name).join(',')}
    </div>
  ),
  getSlashOptionId: (listboxId: string, item: { key: string }) =>
    `${listboxId}-option-${encodeURIComponent(item.key)}`,
}));

vi.mock('./MediaPreview', () => ({
  MediaPreview: () => <div data-testid="media-preview" />,
}));

vi.mock('./VoiceButton', () => ({
  VoiceButton: ({ onTranscription }: { onTranscription: (text: string) => void }) => {
    voiceState.transcribe = onTranscription;
    return <button data-testid="voice-button" onClick={() => onTranscription('transcrição')}>voz</button>;
  },
}));

function mediaResult(files: File[], prefix: string) {
  return files.map((file, index) => ({
    id: `${prefix}-${index}`,
    fileName: file.name,
    category: 'other',
  })) as unknown as MediaFile[];
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe('ChatInput', () => {
  beforeEach(() => {
    getSkillsForProfileSpy.mockReset();
    processMediaFilesSpy.mockReset();
    announceSpy.mockReset();
    voiceState.transcribe = undefined;
    getSkillsForProfileSpy.mockResolvedValue([]);
    processMediaFilesSpy.mockImplementation(async (files: File[]) => mediaResult(files, 'file'));
  });

  it('envia mensagem ao pressionar Enter', () => {
    const onSend = vi.fn(() => true);

    render(<ChatInput onSend={onSend} />);

    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.change(textarea, { target: { value: 'Oi' } });
    fireEvent.keyDown(textarea, { key: 'Enter' });

    expect(onSend).toHaveBeenCalledWith('Oi', undefined);
    expect(textarea).toHaveValue('');
  });

  it('preserva foco e cursor quando ArrowUp não tem destino de navegação', () => {
    const onArrowUp = vi.fn(() => false);
    render(<ChatInput onSend={() => {}} message="rascunho" onMessageChange={vi.fn()} onArrowUp={onArrowUp} />);

    const textarea = screen.getByLabelText('chat.messageLabel') as HTMLTextAreaElement;
    textarea.focus();
    textarea.setSelectionRange(0, 0);
    const event = createEvent.keyDown(textarea, { key: 'ArrowUp' });
    fireEvent(textarea, event);

    expect(onArrowUp).toHaveBeenCalledOnce();
    expect(event.defaultPrevented).toBe(false);
    expect(textarea).toHaveFocus();
    expect(textarea.selectionStart).toBe(0);
    expect(textarea.selectionEnd).toBe(0);
  });

  it.each([{ repeat: true }, { isComposing: true }, { keyCode: 229 }])('bloqueia envio e cancelamento para evento %j', (flags) => {
    const onSend = vi.fn(); const cancel = vi.fn();
    const view = render(<ChatInput onSend={onSend} clearOnSend={false} />);
    const input = screen.getByLabelText('chat.messageLabel');
    fireEvent.change(input, { target: { value: 'rascunho' } });
    fireEvent.keyDown(input, { key: 'Enter', ...flags });
    expect(onSend).not.toHaveBeenCalled(); expect(input).toHaveValue('rascunho');
    view.rerender(<ChatInput onSend={onSend} clearOnSend={false} isStreaming onCancelStreaming={cancel} />);
    fireEvent.keyDown(input, { key: 'Escape', ...flags });
    expect(cancel).not.toHaveBeenCalled();
  });

  it.each([{ repeat: true }, { isComposing: true }, { keyCode: 229 }])('protege seleção/fechamento slash antes de tratar %j', async (flags) => {
    getSkillsForProfileSpy.mockResolvedValueOnce([{ slug: 'skill', name: 'Skill' }]);
    const onSend = vi.fn();
    render(<ChatInput onSend={onSend} />);
    await waitFor(() => expect(getSkillsForProfileSpy).toHaveBeenCalled());
    const input = screen.getByLabelText('chat.messageLabel');
    fireEvent.change(input, { target: { value: '/' } });
    await screen.findByTestId('slash-menu');
    for (const key of ['Enter', 'Tab', 'Escape']) {
      fireEvent.keyDown(input, { key, ...flags });
      expect(screen.getByTestId('slash-menu')).toBeInTheDocument();
      expect(input).toHaveValue('/');
    }
    expect(onSend).not.toHaveBeenCalled();
  });

  it.each([true, false])('clearOnSend=%s controla limpeza de texto e mídia', (clearOnSend) => {
    const media = mediaResult([new File(['data'], 'file.txt')], 'send');
    const onSend = vi.fn(() => true); const change = vi.fn(); const changeMedia = vi.fn();
    render(<ChatInput onSend={onSend} message="rascunho" mediaFiles={media}
      onMessageChange={change} onMediaFilesChange={changeMedia} clearOnSend={clearOnSend} />);
    fireEvent.keyDown(screen.getByLabelText('chat.messageLabel'), { key: 'Enter' });
    expect(onSend).toHaveBeenCalledExactlyOnceWith('rascunho', media);
    if (clearOnSend) {
      expect(change).toHaveBeenCalledExactlyOnceWith('');
      expect(changeMedia).toHaveBeenCalledExactlyOnceWith([]);
    } else {
      expect(change).not.toHaveBeenCalled(); expect(changeMedia).not.toHaveBeenCalled();
      expect(screen.getByLabelText('chat.messageLabel')).toHaveValue('rascunho');
    }
  });

  it.each([true, false])('voz informa origem explícita e respeita clearOnSend=%s sem limpar texto', (clearOnSend) => {
    const onSend = vi.fn(() => true); const change = vi.fn(); const changeMedia = vi.fn();
    render(<ChatInput onSend={onSend} voiceEnabled clearOnSend={clearOnSend}
      message="" onMessageChange={change} mediaFiles={[]} onMediaFilesChange={changeMedia} />);
    fireEvent.click(screen.getByTestId('voice-button'));
    expect(onSend).toHaveBeenCalledExactlyOnceWith('transcrição', undefined, { voice: true });
    expect(change).not.toHaveBeenCalled();
    if (clearOnSend) expect(changeMedia).toHaveBeenCalledExactlyOnceWith([]);
    else expect(changeMedia).not.toHaveBeenCalled();
  });

  it('clearOnSend=false preserva o rascunho quando o callback síncrono retorna void', () => {
    const onSend = vi.fn(); const change = vi.fn(); const changeMedia = vi.fn();
    const media = mediaResult([new File(['data'], 'file.txt')], 'send');
    render(
      <ChatInput
        onSend={onSend}
        clearOnSend={false}
        message="rascunho"
        mediaFiles={media}
        onMessageChange={change}
        onMediaFilesChange={changeMedia}
        slashMenuEnabled={false}
      />,
    );

    fireEvent.keyDown(screen.getByLabelText('chat.messageLabel'), { key: 'Enter' });

    expect(onSend).toHaveBeenCalledExactlyOnceWith('rascunho', media);
    expect(change).not.toHaveBeenCalled();
    expect(changeMedia).not.toHaveBeenCalled();
    expect(screen.getByLabelText('chat.messageLabel')).toHaveValue('rascunho');
  });

  it('mantém origem de voz mesmo quando transcrição coincide com texto digitado', () => {
    const onSend = vi.fn(); const change = vi.fn();
    const view = render(<ChatInput onSend={onSend} voiceEnabled clearOnSend={false} message="" onMessageChange={change} />);
    const transcribe = voiceState.transcribe;
    expect(transcribe).toBeTypeOf('function');
    view.rerender(<ChatInput onSend={onSend} voiceEnabled clearOnSend={false} message="voz" onMessageChange={change} />);
    act(() => transcribe?.('voz'));
    expect(onSend).toHaveBeenCalledExactlyOnceWith('voz', undefined, { voice: true });
    expect(change).not.toHaveBeenCalled();
  });

  it('não envia por Shift+Enter ou Ctrl+K nem consome Ctrl+K', () => {
    const onSend = vi.fn();
    render(<ChatInput onSend={onSend} message="rascunho" />);
    const input = screen.getByLabelText('chat.messageLabel');
    fireEvent.keyDown(input, { key: 'Enter', shiftKey: true });
    expect(fireEvent.keyDown(input, { key: 'k', code: 'KeyK', ctrlKey: true })).toBe(true);
    expect(onSend).not.toHaveBeenCalled();
  });

  it('carrega slash menu usando profileSlug quando informado', async () => {
    render(<ChatInput onSend={() => {}} profileSlug="programacao" />);

    await waitFor(() => {
      expect(getSkillsForProfileSpy).toHaveBeenCalledWith('programacao');
    });
  });

  it('carrega slash menu pelo perfil ativo do backend quando profileSlug não foi resolvido', async () => {
    render(<ChatInput onSend={() => {}} />);

    await waitFor(() => {
      expect(getSkillsForProfileSpy).toHaveBeenCalledWith('');
    });
  });

  it('mostra menu slash quando ha skills', async () => {
    getSkillsForProfileSpy.mockResolvedValueOnce([{ slug: 'skill', name: 'Skill' }]);

    render(<ChatInput onSend={() => {}} profileSlug="programacao" />);

    await waitFor(() => {
      expect(getSkillsForProfileSpy).toHaveBeenCalledWith('programacao');
    });

    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.change(textarea, { target: { value: '/' } });

    expect(await screen.findByTestId('slash-menu')).toBeInTheDocument();
  });

  it('não carrega nem abre o menu slash quando desabilitado', async () => {
    render(<ChatInput onSend={() => {}} slashMenuEnabled={false} />);

    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.change(textarea, { target: { value: '/' } });

    expect(getSkillsForProfileSpy).not.toHaveBeenCalled();
    expect(screen.queryByTestId('slash-menu')).not.toBeInTheDocument();
    expect(textarea).not.toHaveAttribute('role', 'combobox');
    expect(textarea).not.toHaveAttribute('aria-expanded');
  });

  it('ignora resposta atrasada de profileSlug anterior', async () => {
    const first = deferred<Array<{ slug: string; name: string }>>();
    const second = deferred<Array<{ slug: string; name: string }>>();
    getSkillsForProfileSpy
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const { rerender } = render(<ChatInput onSend={() => {}} profileSlug="old-profile" />);
    await waitFor(() => {
      expect(getSkillsForProfileSpy).toHaveBeenCalledWith('old-profile');
    });

    rerender(<ChatInput onSend={() => {}} profileSlug="new-profile" />);
    await waitFor(() => {
      expect(getSkillsForProfileSpy).toHaveBeenCalledWith('new-profile');
    });

    await act(async () => {
      second.resolve([{ slug: 'new-skill', name: 'New Skill' }]);
      await second.promise;
    });
    const textarea = screen.getByLabelText('chat.messageLabel');
    await waitFor(() => {
      fireEvent.change(textarea, { target: { value: '/' } });
      expect(screen.getByTestId('slash-menu')).toHaveTextContent('New Skill');
    });

    await act(async () => {
      first.resolve([{ slug: 'old-skill', name: 'Old Skill' }]);
      await first.promise;
    });
    fireEvent.change(textarea, { target: { value: '/n' } });
    expect(screen.getByTestId('slash-menu')).toHaveTextContent('New Skill');
    expect(screen.getByTestId('slash-menu')).not.toHaveTextContent('Old Skill');
  });

  it('mostra botao de voz quando vazio', () => {
    render(<ChatInput onSend={() => {}} voiceEnabled />);

    expect(screen.getByTestId('voice-button')).toBeInTheDocument();
  });

  it('mostra botão de cancelar geração durante streaming', () => {
    const onCancelStreaming = vi.fn();

    render(<ChatInput onSend={() => {}} isStreaming onCancelStreaming={onCancelStreaming} />);

    const cancelButton = screen.getByText('chat.cancelGeneration');
    fireEvent.click(cancelButton);

    expect(onCancelStreaming).toHaveBeenCalledTimes(1);
  });

  it('aciona cancelamento no Esc durante streaming', () => {
    const onCancelStreaming = vi.fn();

    render(<ChatInput onSend={() => {}} isStreaming onCancelStreaming={onCancelStreaming} />);

    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.keyDown(textarea, { key: 'Escape' });

    expect(onCancelStreaming).toHaveBeenCalledTimes(1);
  });

  it('não envia mensagem com Enter durante streaming', () => {
    const onSend = vi.fn();

    render(<ChatInput onSend={onSend} isStreaming />);

    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.change(textarea, { target: { value: 'Oi' } });
    fireEvent.keyDown(textarea, { key: 'Enter' });

    expect(onSend).not.toHaveBeenCalled();
  });

  it('usa estado controlado para rascunho e anexos', () => {
    const onMessageChange = vi.fn();
    const onMediaFilesChange = vi.fn();

    render(
      <ChatInput
        onSend={() => {}}
        message="Rascunho da superfície"
        mediaFiles={[]}
        onMessageChange={onMessageChange}
        onMediaFilesChange={onMediaFilesChange}
      />,
    );

    const textarea = screen.getByLabelText('chat.messageLabel');
    expect(textarea).toHaveValue('Rascunho da superfície');

    fireEvent.change(textarea, { target: { value: 'Novo rascunho' } });

    expect(onMessageChange).toHaveBeenCalledWith('Novo rascunho');
    expect(onMediaFilesChange).not.toHaveBeenCalled();
  });

  it('mostra indicador de rascunho salvo quando há rascunho controlado', () => {
    render(
      <ChatInput
        onSend={() => {}}
        message="Rascunho em progresso"
        onMessageChange={() => {}}
      />,
    );

    expect(screen.getByText('chat.draftSaved')).toBeInTheDocument();
  });

  it('não mostra indicador de rascunho quando o rascunho controlado está vazio', () => {
    render(
      <ChatInput
        onSend={() => {}}
        message="   "
        onMessageChange={() => {}}
      />,
    );

    expect(screen.queryByText('chat.draftSaved')).not.toBeInTheDocument();
  });

  it('esconde indicador de rascunho após enviar a mensagem', () => {
    const onSend = vi.fn(() => true);

    function ControlledDraftInput() {
      const [message, setMessage] = useState('Mensagem com rascunho');
      return (
        <ChatInput
          onSend={onSend}
          message={message}
          onMessageChange={setMessage}
        />
      );
    }

    render(<ControlledDraftInput />);

    expect(screen.getByText('chat.draftSaved')).toBeInTheDocument();

    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.keyDown(textarea, { key: 'Enter' });

    expect(onSend).toHaveBeenCalledWith('Mensagem com rascunho', undefined);
    expect(screen.queryByText('chat.draftSaved')).not.toBeInTheDocument();
  });

  it('preserva texto e anexos visíveis quando o envio é rejeitado', async () => {
    const onSend = vi.fn().mockResolvedValue(false);
    const originalMedia = mediaResult([new File(['a'], 'original.txt')], 'original');

    function ControlledDraftInput() {
      const [message, setMessage] = useState('rascunho rejeitado');
      const [mediaFiles, setMediaFiles] = useState<MediaFile[]>(originalMedia);
      return (
        <>
          <ChatInput
            onSend={onSend}
            message={message}
            mediaFiles={mediaFiles}
            onMessageChange={setMessage}
            onMediaFilesChange={setMediaFiles}
            slashMenuEnabled={false}
          />
          <div data-testid="rejected-draft-state">
            {message}|{mediaFiles.map((file) => file.fileName).join(',')}
          </div>
        </>
      );
    }

    render(<ControlledDraftInput />);
    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.keyDown(textarea, { key: 'Enter' });

    expect(screen.getByTestId('rejected-draft-state')).toHaveTextContent('rascunho rejeitado|original.txt');
    await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
    expect(screen.getByTestId('rejected-draft-state')).toHaveTextContent('rascunho rejeitado|original.txt');
  });

  it('voz não apaga texto digitado enquanto a transcrição aguarda aceitação', async () => {
    const send = deferred<boolean>();
    const onSend = vi.fn(() => send.promise);

    function ControlledVoiceInput() {
      const [message, setMessage] = useState('');
      return (
        <>
          <ChatInput
            onSend={onSend}
            message={message}
            onMessageChange={setMessage}
            voiceEnabled
            slashMenuEnabled={false}
          />
          <div data-testid="voice-draft-state">{message}</div>
        </>
      );
    }

    render(<ControlledVoiceInput />);
    fireEvent.click(screen.getByTestId('voice-button'));
    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.change(textarea, { target: { value: 'texto novo' } });

    await act(async () => {
      send.resolve(true);
      await send.promise;
    });

    expect(onSend).toHaveBeenCalledWith('transcrição', undefined, { voice: true });
    expect(screen.getByTestId('voice-draft-state')).toHaveTextContent('texto novo');
  });

  it('restaura rascunho e anexos quando o envio é rejeitado, sem apagar texto novo nem reenviar', async () => {
    const send = deferred<boolean>();
    const onSend = vi.fn(() => send.promise);
    const originalMedia = mediaResult([new File(['a'], 'original.txt')], 'original');

    function ControlledDraftInput() {
      const [message, setMessage] = useState('mensagem grande');
      const [mediaFiles, setMediaFiles] = useState<MediaFile[]>(originalMedia);
      return (
        <>
          <ChatInput
            onSend={onSend}
            message={message}
            mediaFiles={mediaFiles}
            onMessageChange={setMessage}
            onMediaFilesChange={setMediaFiles}
            slashMenuEnabled={false}
          />
          <div data-testid="draft-state">{message}|{mediaFiles.map((file) => file.fileName).join(',')}</div>
        </>
      );
    }

    render(<ControlledDraftInput />);
    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.keyDown(textarea, { key: 'Enter' });
    fireEvent.change(textarea, { target: { value: 'texto digitado depois' } });
    fireEvent.keyDown(textarea, { key: 'Enter' });

    expect(onSend).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId('draft-state')).toHaveTextContent('texto digitado depois|');

    await act(async () => {
      send.resolve(false);
      await send.promise;
    });

    expect(screen.getByTestId('draft-state')).toHaveTextContent('texto digitado depois|');
    expect(screen.getByTestId('draft-state')).not.toHaveTextContent('mensagem grande');
  });

  it('não cria live region local (indicador é puramente visual com aria-hidden)', () => {
    render(
      <ChatInput
        onSend={() => {}}
        message="Rascunho anunciável"
        onMessageChange={() => {}}
      />,
    );

    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.getByText('chat.draftSaved')).toBeInTheDocument();
  });

  it('não anuncia quando monta já com rascunho existente', () => {
    render(
      <ChatInput
        onSend={() => {}}
        message="Rascunho restaurado"
        onMessageChange={() => {}}
      />,
    );

    expect(announceSpy).not.toHaveBeenCalled();
  });

  it('anuncia via announcer global na transição de sem rascunho para com rascunho', () => {
    const { rerender } = render(
      <ChatInput
        onSend={() => {}}
        message=""
        onMessageChange={() => {}}
      />,
    );

    expect(announceSpy).not.toHaveBeenCalled();

    rerender(
      <ChatInput
        onSend={() => {}}
        message="Acabei de digitar"
        onMessageChange={() => {}}
      />,
    );

    expect(announceSpy).toHaveBeenCalledWith('chat.draftSaved', 'polite');
    expect(announceSpy).toHaveBeenCalledTimes(1);
  });

  it('mostra indicador quando apenas os anexos são controlados e não-vazios', () => {
    const controlledMedia = mediaResult(
      [new File(['anexo'], 'anexo.txt', { type: 'text/plain' })],
      'draft',
    );

    render(
      <ChatInput
        onSend={() => {}}
        mediaFiles={controlledMedia}
        onMediaFilesChange={() => {}}
      />,
    );

    expect(screen.getByText('chat.draftSaved')).toBeInTheDocument();
  });

  it('não mostra indicador quando os anexos controlados estão vazios', () => {
    render(
      <ChatInput
        onSend={() => {}}
        mediaFiles={[]}
        onMediaFilesChange={() => {}}
      />,
    );

    expect(screen.queryByText('chat.draftSaved')).not.toBeInTheDocument();
  });

  it('não mostra indicador para anexos locais quando só a mensagem está controlada', async () => {
    const localFile = new File(['local'], 'local.txt', { type: 'text/plain' });

    render(
      <ChatInput
        onSend={() => {}}
        message=""
        onMessageChange={() => {}}
      />,
    );

    const fileInput = screen.getByLabelText('chat.selectFiles');
    fireEvent.change(fileInput, { target: { files: [localFile] } });

    await waitFor(() => {
      expect(processMediaFilesSpy).toHaveBeenCalledWith([localFile]);
    });

    expect(screen.queryByText('chat.draftSaved')).not.toBeInTheDocument();
  });

  it('ignora prop message sem onMessageChange para evitar textarea read-only', () => {
    render(<ChatInput onSend={() => {}} message="Prop sem handler" />);

    const textarea = screen.getByLabelText('chat.messageLabel');
    expect(textarea).toHaveValue('');

    fireEvent.change(textarea, { target: { value: 'Texto local' } });

    expect(textarea).toHaveValue('Texto local');
  });

  it('ignora prop mediaFiles sem onMediaFilesChange para manter anexos mutáveis localmente', async () => {
    const firstFile = new File(['primeiro'], 'first.txt', { type: 'text/plain' });

    render(<ChatInput onSend={() => {}} mediaFiles={[]} />);

    const fileInput = screen.getByLabelText('chat.selectFiles');
    fireEvent.change(fileInput, { target: { files: [firstFile] } });

    await waitFor(() => {
      expect(processMediaFilesSpy).toHaveBeenCalledWith([firstFile]);
    });
  });

  it('expõe o textarea para callback refs', () => {
    const callbackRef = vi.fn();

    render(<ChatInput ref={callbackRef} onSend={() => {}} />);

    expect(callbackRef).toHaveBeenCalledWith(screen.getByLabelText('chat.messageLabel'));
  });

  it('preserva anexos adicionados enquanto outro processamento ainda está pendente', async () => {
    const firstProcessing = deferred<void>();
    const firstFile = new File(['primeiro'], 'first.txt', { type: 'text/plain' });
    const secondFile = new File(['segundo'], 'second.txt', { type: 'text/plain' });
    processMediaFilesSpy
      .mockImplementationOnce(async (files: File[]) => {
        await firstProcessing.promise;
        return mediaResult(files, 'first');
      })
      .mockImplementationOnce(async (files: File[]) => mediaResult(files, 'second'));

    function ControlledChatInput() {
      const [mediaFiles, setMediaFiles] = useState<MediaFile[]>([]);
      return (
        <>
          <ChatInput
            onSend={() => {}}
            mediaFiles={mediaFiles}
            onMediaFilesChange={(nextMediaFiles) => setMediaFiles(nextMediaFiles)}
          />
          <div data-testid="media-files">
            {mediaFiles.map((file) => file.fileName).join(',')}
          </div>
        </>
      );
    }

    render(<ControlledChatInput />);

    const fileInput = screen.getByLabelText('chat.selectFiles');
    fireEvent.change(fileInput, { target: { files: [firstFile] } });
    fireEvent.change(fileInput, { target: { files: [secondFile] } });

    await waitFor(() => {
      expect(screen.getByTestId('media-files')).toHaveTextContent('second.txt');
    });

    firstProcessing.resolve(undefined);

    await waitFor(() => {
      expect(screen.getByTestId('media-files')).toHaveTextContent('second.txt');
      expect(screen.getByTestId('media-files')).toHaveTextContent('first.txt');
    });
  });
});
