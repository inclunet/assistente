/** @vitest-environment jsdom */
import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { VoiceButton } from './VoiceButton';

const toggleInteractionSpy = vi.fn();
const startInteractionSpy = vi.fn();
const stopInteractionSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../../hooks/useInteractionProfile', () => ({
  useInteractionProfile: () => ({
    isActive: false,
    isListening: false,
    isRecording: false,
    isProcessing: false,
    volume: 0,
    interimText: '',
    activeProfile: {
      interaction: {
        triggers: [{ type: 'button_toggle', enabled: true }],
      },
    },
    startInteraction: startInteractionSpy,
    stopInteraction: stopInteractionSpy,
    cancelInteraction: vi.fn(),
    toggleInteraction: toggleInteractionSpy,
    isWakewordListening: false,
  }),
}));

describe('VoiceButton', () => {
  it('aciona toggle no clique', () => {
    render(<VoiceButton onTranscription={() => {}} />);

    fireEvent.click(screen.getByRole('button'));
    expect(toggleInteractionSpy).toHaveBeenCalled();
  });

  it('usa aria-label do modo', () => {
    render(<VoiceButton onTranscription={() => {}} />);

    expect(screen.getByRole('button')).toHaveAttribute('aria-label', 'voice.clickToRecord');
  });
});
