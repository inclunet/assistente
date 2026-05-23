import { useState } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ChatInput } from './ChatInput';
import type { MediaFile } from '../../services/mediaService';

const getSkillsSpy = vi.fn();
const processMediaFilesSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetUserInvocableSkills: () => getSkillsSpy(),
}));

vi.mock('../../services/mediaService', () => ({
  processMediaFiles: (files: File[]) => processMediaFilesSpy(files),
}));

vi.mock('./SlashCommandMenu', () => ({
  SlashCommandMenu: () => <div data-testid="slash-menu" />,
  countFilteredSkills: () => 1,
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
    getSkillsSpy.mockReset();
    processMediaFilesSpy.mockReset();
    processMediaFilesSpy.mockImplementation(async (files: File[]) => mediaResult(files, 'file'));
  });

  it('envia mensagem ao pressionar Enter', () => {
    getSkillsSpy.mockResolvedValueOnce([]);
    const onSend = vi.fn();

    render(<ChatInput onSend={onSend} />);

    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.change(textarea, { target: { value: 'Oi' } });
    fireEvent.keyDown(textarea, { key: 'Enter' });

    expect(onSend).toHaveBeenCalledWith('Oi', undefined);
  });

  it('mostra menu slash quando ha skills', async () => {
    getSkillsSpy.mockResolvedValueOnce([{ slug: 'skill', name: 'Skill' }]);

    render(<ChatInput onSend={() => {}} />);

    await waitFor(() => {
      expect(getSkillsSpy).toHaveBeenCalled();
    });

    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.change(textarea, { target: { value: '/' } });

    expect(await screen.findByTestId('slash-menu')).toBeInTheDocument();
  });

  it('mostra botao de voz quando vazio', () => {
    getSkillsSpy.mockResolvedValueOnce([]);

    render(<ChatInput onSend={() => {}} voiceEnabled />);

    expect(screen.getByTestId('voice-button')).toBeInTheDocument();
  });

  it('mostra botão de cancelar geração durante streaming', () => {
    getSkillsSpy.mockResolvedValueOnce([]);
    const onCancelStreaming = vi.fn();

    render(<ChatInput onSend={() => {}} isStreaming onCancelStreaming={onCancelStreaming} />);

    const cancelButton = screen.getByText('chat.cancelGeneration');
    fireEvent.click(cancelButton);

    expect(onCancelStreaming).toHaveBeenCalledTimes(1);
  });

  it('aciona cancelamento no Esc durante streaming', () => {
    getSkillsSpy.mockResolvedValueOnce([]);
    const onCancelStreaming = vi.fn();

    render(<ChatInput onSend={() => {}} isStreaming onCancelStreaming={onCancelStreaming} />);

    const textarea = screen.getByLabelText('chat.messageLabel');
    fireEvent.keyDown(textarea, { key: 'Escape' });

    expect(onCancelStreaming).toHaveBeenCalledTimes(1);
  });

  it('usa estado controlado para rascunho e anexos', () => {
    getSkillsSpy.mockResolvedValueOnce([]);
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

  it('ignora prop message sem onMessageChange para evitar textarea read-only', () => {
    getSkillsSpy.mockResolvedValueOnce([]);

    render(<ChatInput onSend={() => {}} message="Prop sem handler" />);

    const textarea = screen.getByLabelText('chat.messageLabel');
    expect(textarea).toHaveValue('');

    fireEvent.change(textarea, { target: { value: 'Texto local' } });

    expect(textarea).toHaveValue('Texto local');
  });

  it('ignora prop mediaFiles sem onMediaFilesChange para manter anexos mutáveis localmente', async () => {
    getSkillsSpy.mockResolvedValueOnce([]);
    const firstFile = new File(['primeiro'], 'first.txt', { type: 'text/plain' });

    render(<ChatInput onSend={() => {}} mediaFiles={[]} />);

    const fileInput = screen.getByLabelText('chat.selectFiles');
    fireEvent.change(fileInput, { target: { files: [firstFile] } });

    await waitFor(() => {
      expect(processMediaFilesSpy).toHaveBeenCalledWith([firstFile]);
    });
  });

  it('expõe o textarea para callback refs', () => {
    getSkillsSpy.mockResolvedValueOnce([]);
    const callbackRef = vi.fn();

    render(<ChatInput ref={callbackRef} onSend={() => {}} />);

    expect(callbackRef).toHaveBeenCalledWith(screen.getByLabelText('chat.messageLabel'));
  });

  it('preserva anexos adicionados enquanto outro processamento ainda está pendente', async () => {
    getSkillsSpy.mockResolvedValueOnce([]);
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
