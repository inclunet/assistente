import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ChatInput } from './ChatInput';

const getSkillsSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetUserInvocableSkills: () => getSkillsSpy(),
}));

vi.mock('../../services/mediaService', () => ({
  processMediaFiles: async (files: File[]) => files.map((file, index) => ({
    id: String(index),
    fileName: file.name,
    category: 'other',
  })),
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

describe('ChatInput', () => {
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
});
