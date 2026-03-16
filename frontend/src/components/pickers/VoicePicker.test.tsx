import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { VoicePicker } from './VoicePicker';
import { TTSProvider } from '../../services/tts/types';

const getVoicesSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../../services/tts', () => ({
  ttsService: { getVoices: () => getVoicesSpy() },
}));

vi.mock('./BasePicker', () => ({
  BasePicker: (props: { items: Array<{ value: string }> }) => (
    <div data-testid="base-picker" data-items={props.items.length} />
  ),
}));

describe('VoicePicker', () => {
  it('carrega vozes e inclui opcao desativada', async () => {
    getVoicesSpy.mockResolvedValueOnce([
      { id: 'v1', name: 'Voice', language: 'pt-BR', provider: TTSProvider.WEBSPEECH, premium: false },
    ]);

    render(<VoicePicker value="" onChange={() => {}} />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2');
    });
  });
});
