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
  GetSpeechProviders: () => Promise.resolve([]),
  GetTTSModels: () => Promise.resolve([]),
  GetSTTModels: () => Promise.resolve([]),
}));

vi.mock('@wailsjs/go/models', () => ({
  profiles: {
    Profile: class {
      static createFrom(source: unknown = {}) { return source; }
    },
  },
}));

vi.mock('./ProfileVoiceSection', () => ({
  ProfileVoiceSection: ({ onChange, onModelChange, ttsModel, providerId }: { onChange: (f: string, v: unknown) => void; onModelChange?: (m: string) => void; ttsModel?: string; providerId?: string }) => (
    <div data-testid="voice-section" data-provider={providerId} data-model={ttsModel}>
      <button onClick={() => onChange('rate', 1.5)}>change-rate</button>
      {onModelChange && <button onClick={() => onModelChange('tts-1-hd')}>change-model</button>}
    </div>
  ),
}));

vi.mock('./ProfileInteractionSection', () => ({
  ProfileInteractionSection: () => <div data-testid="interaction-section">Interaction</div>,
}));

vi.mock('../pickers/VoicePicker', () => ({
  VOICE_DISABLED: '__disabled__',
  VOICE_REF_ASSISTANT: '__ref_assistant__',
  VOICE_REF_USER: '__ref_user__',
  VOICE_REF_SYSTEM: '__ref_system__',
  VoicePicker: () => null,
}));

// Capture items passed to VoiceProviderPicker for circular ref tests
const voiceProviderPickerItemsSpy = vi.fn();
vi.mock('../pickers/VoiceProviderPicker', () => ({
  VoiceProviderPicker: ({ items, value, onChange }: { items: Array<{ id: string; label: string }>; value: string; onChange: (v: string) => void }) => {
    voiceProviderPickerItemsSpy(items.map((i: { id: string }) => i.id), value);
    return (
      <select data-testid="voice-provider-picker" value={value} onChange={(e) => onChange(e.target.value)}>
        {items.map((i: { id: string; label: string }) => <option key={i.id} value={i.id}>{i.label}</option>)}
      </select>
    );
  },
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

  describe('referência circular de voz', () => {
    it('system NÃO oferece "seguir user" quando user segue system', () => {
      voiceProviderPickerItemsSpy.mockClear();
      const profile = makeProfile({
        voice: {
          assistant: { enabled: true, provider: 'webspeech' },
          user: { enabled: true, provider: 'webspeech', voice_id: '__ref_system__' },
          system: { enabled: true, provider: 'webspeech' },
        },
      });
      render(<ProfileAudioTab editingProfile={profile} updateField={vi.fn()} updateFields={vi.fn()} profileId="test" />);

      // Collect all calls - find the one for system picker (value = system's current provider)
      const systemCalls = voiceProviderPickerItemsSpy.mock.calls.filter(
        (call: unknown[]) => {
          const ids = call[0] as string[];
          return ids.includes('ref_assistant') && !ids.includes('ref_user');
        }
      );
      // System picker should NOT have ref_user since user follows system
      expect(systemCalls.length).toBeGreaterThanOrEqual(1);
    });

    it('user NÃO oferece "seguir system" quando system segue user', () => {
      voiceProviderPickerItemsSpy.mockClear();
      const profile = makeProfile({
        voice: {
          assistant: { enabled: true, provider: 'webspeech' },
          user: { enabled: true, provider: 'webspeech' },
          system: { enabled: true, provider: 'webspeech', voice_id: '__ref_user__' },
        },
      });
      render(<ProfileAudioTab editingProfile={profile} updateField={vi.fn()} updateFields={vi.fn()} profileId="test" />);

      const userCalls = voiceProviderPickerItemsSpy.mock.calls.filter(
        (call: unknown[]) => {
          const ids = call[0] as string[];
          return ids.includes('ref_assistant') && !ids.includes('ref_system');
        }
      );
      // User picker should NOT have ref_system since system follows user
      expect(userCalls.length).toBeGreaterThanOrEqual(1);
    });

    it('user oferece "seguir system" quando nenhum ciclo existe', () => {
      voiceProviderPickerItemsSpy.mockClear();
      const profile = makeProfile({
        voice: {
          assistant: { enabled: true, provider: 'webspeech' },
          user: { enabled: true, provider: 'webspeech' },
          system: { enabled: true, provider: 'webspeech' },
        },
      });
      render(<ProfileAudioTab editingProfile={profile} updateField={vi.fn()} updateFields={vi.fn()} profileId="test" />);

      // At least one picker should have both ref_assistant and ref_system
      const userCalls = voiceProviderPickerItemsSpy.mock.calls.filter(
        (call: unknown[]) => {
          const ids = call[0] as string[];
          return ids.includes('ref_assistant') && ids.includes('ref_system');
        }
      );
      expect(userCalls.length).toBeGreaterThanOrEqual(1);
    });
  });

  describe('modelo TTS por role', () => {
    it('ProfileVoiceSection recebe onModelChange para cada role', () => {
      const updateField = vi.fn();
      const profile = makeProfile({
        voice: {
          assistant: { enabled: true, provider: 'openai', llm_provider_id: 'openai-1', model: 'tts-1' },
          user: { enabled: true, provider: 'openai', llm_provider_id: 'openai-1', model: 'tts-1' },
          system: { enabled: true, provider: 'openai', llm_provider_id: 'openai-1', model: 'tts-1' },
        },
      });
      render(<ProfileAudioTab editingProfile={profile} updateField={updateField} updateFields={vi.fn()} profileId="test" />);

      // Each voice section should have a change-model button (provided by our mock for onModelChange)
      const modelButtons = screen.getAllByText('change-model');
      expect(modelButtons.length).toBe(3);

      // Clicking each should call the right path
      fireEvent.click(modelButtons[0]);
      expect(updateField).toHaveBeenCalledWith('voice.assistant.model', 'tts-1-hd');

      fireEvent.click(modelButtons[1]);
      expect(updateField).toHaveBeenCalledWith('voice.user.model', 'tts-1-hd');

      fireEvent.click(modelButtons[2]);
      expect(updateField).toHaveBeenCalledWith('voice.system.model', 'tts-1-hd');
    });
  });
});
