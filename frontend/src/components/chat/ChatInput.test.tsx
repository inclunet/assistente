import { useState } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ChatInput } from './ChatInput';
import type { MediaFile } from '../../services/mediaService';

const getSkillsForProfileSpy = vi.fn();
const processMediaFilesSpy = vi.fn();
const announceSpy = vi.fn();

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
  countFilteredSlashItems: () => 1,
  getSlashOptionId: (listboxId: string, item: { key: string }) =>
    `${listboxId}-option-${item.key.replace(/[^a-zA-Z0-9_-]/g, '-')}`,
}));

vi.mock('./MediaPreview', () => ({
  MediaPreview: () => <div data-testid="media-preview" />,
}));

vi.mock('./VoiceButton', () => ({
  VoiceButton: () => <button data-testid="voice-button" />,
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
    getSkillsForProfileSpy.mockResolvedValue([]);
    processMediaFilesSpy.mockImplementation(async (files: File[]) => mediaResult(files, 'file'));
  });

  it('envia mensagem ao pressionar Enter', () => {
    const onSend = vi.fn();

    render(<ChatInput onSend={onSend} />);

    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.change(textarea, { target: { value: 'Oi' } });
    fireEvent.keyDown(textarea, { key: 'Enter' });

    expect(onSend).toHaveBeenCalledWith('Oi', undefined);
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
    const onSend = vi.fn();

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
