import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { TTSControls } from './TTSControls';

const setEnabledSpy = vi.fn();
const setAutoReadSpy = vi.fn();
const stopSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../../hooks/useTTS', () => ({
  useTTS: () => ({
    isEnabled: true,
    isAutoReadEnabled: false,
    isSpeaking: true,
    hasVoiceConfig: true,
    setEnabled: setEnabledSpy,
    setAutoRead: setAutoReadSpy,
    stop: stopSpy,
    isSupported: true,
  }),
}));

describe('TTSControls', () => {
  it('desabilita e para ao clicar no toggle', () => {
    render(<TTSControls />);

    fireEvent.click(screen.getAllByRole('button')[0]);
    expect(stopSpy).toHaveBeenCalled();
    expect(setEnabledSpy).toHaveBeenCalledWith(false);
  });

  it('ativa auto leitura', () => {
    render(<TTSControls />);

    fireEvent.click(screen.getAllByRole('button')[1]);
    expect(setAutoReadSpy).toHaveBeenCalledWith(true);
  });

  it('para ao clicar stop', () => {
    render(<TTSControls />);

    fireEvent.click(screen.getByRole('button', { name: 'chat.stopReadingLabel' }));
    expect(stopSpy).toHaveBeenCalled();
  });
});
