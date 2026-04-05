import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ProfileAudioTab } from './ProfileAudioTab';

/* ── Mocks ─────────────────────────────────────────────── */

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
  }),
}));

vi.mock('@wailsjs/go/main/App', () => ({
  GetLLMProviders: () => Promise.resolve([]),
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
  VOICE_REF_ASSISTANT: 'ref_assistant',
  VOICE_REF_USER: 'ref_user',
  VOICE_REF_SYSTEM: 'ref_system',
  VoicePicker: () => null,
}));

/* ── Helpers ───────────────────────────────────────────── */

function makeProfile(overrides: Record<string, unknown> = {}) {
  const voiceOverrides = (overrides.voice ?? {}) as Record<string, unknown>;
  const inputOverrides = (overrides.input ?? {}) as Record<string, unknown>;
  const channelsOverrides = (overrides.channels ?? {}) as Record<string, unknown>;
  return {
    name: 'Test',
    description: '',
    icon: '',
    chat: {},
    voice: {
      assistant: {
        enabled: false,
        provider: 'disabled',
        voice_id: '',
        rate: 1.0,
        pitch: 1.0,
        volume: 1.0,
        ...((voiceOverrides.assistant ?? {}) as Record<string, unknown>),
      },
      user: {
        enabled: false,
        provider: 'disabled',
        rate: 1.0,
        pitch: 1.0,
        volume: 1.0,
        ...((voiceOverrides.user ?? {}) as Record<string, unknown>),
      },
      system: {
        enabled: false,
        provider: 'disabled',
        rate: 1.0,
        pitch: 1.0,
        volume: 1.0,
        ...((voiceOverrides.system ?? {}) as Record<string, unknown>),
      },
    },
    input: {
      enabled: true,
      stt_provider: 'webspeech',
      language: 'pt-BR',
      feedback_sounds: true,
      ...inputOverrides,
    },
    channels: {
      response_mode: 'mirror',
      ...channelsOverrides,
    },
  } as never;
}

/* ── Testes ─────────────────────────────────────────────── */

describe('ProfileAudioTab', () => {
  it('renderiza seções de voz e input colapsáveis', () => {
    const updateField = vi.fn();
    const updateFields = vi.fn();
    render(<ProfileAudioTab editingProfile={makeProfile()} updateField={updateField} updateFields={updateFields} profileId="test-profile" />);

    expect(screen.getByText('Voz (TTS)')).toBeInTheDocument();
    expect(screen.getByText('Entrada de Voz (STT)')).toBeInTheDocument();
  });

  it('mostra badge "off" quando TTS está desabilitado', () => {
    const updateField = vi.fn();
    const updateFields = vi.fn();
    render(<ProfileAudioTab editingProfile={makeProfile()} updateField={updateField} updateFields={updateFields} profileId="test-profile" />);

    // Voice disabled by default
    const offBadges = screen.getAllByTestId('badge-off');
    expect(offBadges.length).toBeGreaterThanOrEqual(1);
  });

  it('mostra badge "on" quando TTS está habilitado', () => {
    const updateField = vi.fn();
    const updateFields = vi.fn();
    const profile = makeProfile({ voice: { assistant: { enabled: true, provider: 'webspeech' } } });
    render(<ProfileAudioTab editingProfile={profile} updateField={updateField} updateFields={updateFields} profileId="test-profile" />);

    const onBadges = screen.getAllByTestId('badge-on');
    expect(onBadges.length).toBeGreaterThanOrEqual(1);
  });

  it('ativa TTS ao toggle da seção voice quando desabilitado', () => {
    const updateField = vi.fn();
    const updateFields = vi.fn();
    render(<ProfileAudioTab editingProfile={makeProfile()} updateField={updateField} updateFields={updateFields} profileId="test-profile" />);

    // Clica no header da collapsible de voz
    const voiceHeader = screen.getByText('Voz (TTS)').closest('button');
    fireEvent.click(voiceHeader!);

    expect(updateFields).toHaveBeenCalledWith({
      'voice.assistant.provider': 'webspeech',
      'voice.assistant.enabled': true,
    });
  });

  it('desativa TTS ao toggle da seção voice quando habilitado', () => {
    const updateField = vi.fn();
    const updateFields = vi.fn();
    const profile = makeProfile({ voice: { assistant: { enabled: true, provider: 'webspeech' } } });
    render(<ProfileAudioTab editingProfile={profile} updateField={updateField} updateFields={updateFields} profileId="test-profile" />);

    const voiceHeader = screen.getByText('Voz (TTS)').closest('button');
    fireEvent.click(voiceHeader!);

    expect(updateField).toHaveBeenCalledWith('voice.assistant.enabled', false);
  });

  it('ativa STT ao toggle da seção interaction quando desabilitado', () => {
    const updateField = vi.fn();
    const updateFields = vi.fn();
    const profile = makeProfile({ input: { stt_provider: '' } });
    render(<ProfileAudioTab editingProfile={profile} updateField={updateField} updateFields={updateFields} profileId="test-profile" />);

    const sttHeader = screen.getByText('Entrada de Voz (STT)').closest('button');
    fireEvent.click(sttHeader!);

    expect(updateFields).toHaveBeenCalledWith({
      'input.stt_provider': 'webspeech',
      'input.enabled': true,
    });
  });

  it('desativa STT ao toggle da seção interaction quando habilitado', () => {
    const updateField = vi.fn();
    const updateFields = vi.fn();
    render(<ProfileAudioTab editingProfile={makeProfile()} updateField={updateField} updateFields={updateFields} profileId="test-profile" />);

    const sttHeader = screen.getByText('Entrada de Voz (STT)').closest('button');
    fireEvent.click(sttHeader!);

    expect(updateFields).toHaveBeenCalledWith({
      'input.stt_provider': '',
      'input.enabled': false,
    });
  });

  it('toggle da voz do assistente chama updateField', () => {
    const updateField = vi.fn();
    const updateFields = vi.fn();
    const profile = makeProfile({ voice: { assistant: { enabled: true, provider: 'webspeech' } } });
    render(<ProfileAudioTab editingProfile={profile} updateField={updateField} updateFields={updateFields} profileId="test-profile" />);

    const assistantHeader = screen.getByText('profiles.voiceLabels.assistant').closest('button');
    fireEvent.click(assistantHeader!);

    expect(updateField).toHaveBeenCalledWith('voice.assistant.enabled', false);
  });

  it('select de resposta do canal chama updateField', () => {
    const updateField = vi.fn();
    const updateFields = vi.fn();
    const profile = makeProfile({ voice: { assistant: { enabled: true, provider: 'webspeech' } } });
    render(<ProfileAudioTab editingProfile={profile} updateField={updateField} updateFields={updateFields} profileId="test-profile" />);

    const select = screen.getByLabelText('Resposta em canais externos');
    fireEvent.change(select, { target: { value: 'always_text' } });

    expect(updateField).toHaveBeenCalledWith('channels.response_mode', 'always_text');
  });
});
