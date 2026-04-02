import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ProfileAudioTab } from './ProfileAudioTab';

/* ── Mocks ─────────────────────────────────────────────── */

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}));

vi.mock('@wailsjs/go/models', () => ({
  profiles: {
    Profile: class {
      static createFrom(source: unknown = {}) { return source; }
    },
  },
}));

vi.mock('./ProfileVoiceSection', () => ({
  ProfileVoiceSection: ({ onChange }: { onChange: (f: string, v: unknown) => void }) => (
    <div data-testid="voice-section">
      <button onClick={() => onChange('rate', 1.5)}>change-rate</button>
    </div>
  ),
}));

vi.mock('./ProfileInteractionSection', () => ({
  ProfileInteractionSection: () => <div data-testid="interaction-section">Interaction</div>,
}));

vi.mock('../pickers/VoicePicker', () => ({
  VOICE_DISABLED: '__disabled__',
  VoicePicker: () => null,
}));

/* ── Helpers ───────────────────────────────────────────── */

function makeProfile(overrides: Record<string, unknown> = {}) {
  return {
    name: 'Test',
    description: '',
    icon: '',
    chat: {},
    voice: {
      provider: 'disabled',
      voice_id: '',
      rate: 1.0,
      volume: 1.0,
      enabled_for_agent: false,
      enabled_for_user: false,
      channel_response_mode: 'mirror',
      ...((overrides.voice ?? {}) as Record<string, unknown>),
    },
    interaction: {
      stt_provider: 'webspeech',
      language: 'pt-BR',
      feedback_sounds: true,
      ...((overrides.interaction ?? {}) as Record<string, unknown>),
    },
  } as never;
}

/* ── Testes ─────────────────────────────────────────────── */

describe('ProfileAudioTab', () => {
  it('renderiza seções de voz e interação colapsáveis', () => {
    const updateField = vi.fn();
    render(<ProfileAudioTab editingProfile={makeProfile()} updateField={updateField} />);

    expect(screen.getByText('Voz (TTS)')).toBeInTheDocument();
    expect(screen.getByText('Interação (STT)')).toBeInTheDocument();
  });

  it('mostra badge "off" quando TTS está desabilitado', () => {
    const updateField = vi.fn();
    render(<ProfileAudioTab editingProfile={makeProfile()} updateField={updateField} />);

    // Voice disabled by default
    const offBadges = screen.getAllByTestId('badge-off');
    expect(offBadges.length).toBeGreaterThanOrEqual(1);
  });

  it('mostra badge "on" quando TTS está habilitado', () => {
    const updateField = vi.fn();
    const profile = makeProfile({ voice: { provider: 'webspeech' } });
    render(<ProfileAudioTab editingProfile={profile} updateField={updateField} />);

    const onBadges = screen.getAllByTestId('badge-on');
    expect(onBadges.length).toBeGreaterThanOrEqual(1);
  });

  it('ativa TTS ao toggle da seção voice quando desabilitado', () => {
    const updateField = vi.fn();
    render(<ProfileAudioTab editingProfile={makeProfile()} updateField={updateField} />);

    // Clica no header da collapsible de voz
    const voiceHeader = screen.getByText('Voz (TTS)').closest('button');
    fireEvent.click(voiceHeader!);

    expect(updateField).toHaveBeenCalledWith('voice.provider', 'webspeech');
  });

  it('desativa TTS ao toggle da seção voice quando habilitado', () => {
    const updateField = vi.fn();
    const profile = makeProfile({ voice: { provider: 'webspeech' } });
    render(<ProfileAudioTab editingProfile={profile} updateField={updateField} />);

    const voiceHeader = screen.getByText('Voz (TTS)').closest('button');
    fireEvent.click(voiceHeader!);

    expect(updateField).toHaveBeenCalledWith('voice.provider', 'disabled');
  });

  it('ativa STT ao toggle da seção interaction quando desabilitado', () => {
    const updateField = vi.fn();
    const profile = makeProfile({ interaction: { stt_provider: '' } });
    render(<ProfileAudioTab editingProfile={profile} updateField={updateField} />);

    const sttHeader = screen.getByText('Interação (STT)').closest('button');
    fireEvent.click(sttHeader!);

    expect(updateField).toHaveBeenCalledWith('interaction.stt_provider', 'webspeech');
  });

  it('desativa STT ao toggle da seção interaction quando habilitado', () => {
    const updateField = vi.fn();
    render(<ProfileAudioTab editingProfile={makeProfile()} updateField={updateField} />);

    const sttHeader = screen.getByText('Interação (STT)').closest('button');
    fireEvent.click(sttHeader!);

    expect(updateField).toHaveBeenCalledWith('interaction.stt_provider', '');
  });

  it('checkbox TTS agent chama updateField', () => {
    const updateField = vi.fn();
    const profile = makeProfile({ voice: { provider: 'webspeech' } });
    render(<ProfileAudioTab editingProfile={profile} updateField={updateField} />);

    const checkbox = screen.getByLabelText('TTS para mensagens do assistente');
    fireEvent.click(checkbox);

    expect(updateField).toHaveBeenCalledWith('voice.enabled_for_agent', true);
  });

  it('select de resposta do canal chama updateField', () => {
    const updateField = vi.fn();
    const profile = makeProfile({ voice: { provider: 'webspeech' } });
    render(<ProfileAudioTab editingProfile={profile} updateField={updateField} />);

    const select = screen.getByLabelText('Resposta em canais externos');
    fireEvent.change(select, { target: { value: 'always_text' } });

    expect(updateField).toHaveBeenCalledWith('voice.channel_response_mode', 'always_text');
  });
});
