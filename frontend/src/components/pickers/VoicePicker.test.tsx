import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { VoicePicker, VOICE_DISABLED, VOICE_REF_ASSISTANT } from './VoicePicker';
import { TTSProvider } from '../../services/tts/types';

const getVoicesForProviderSpy = vi.fn();
const getVoicesSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../../services/tts', () => ({
  ttsService: {
    getVoicesForProvider: (...args: unknown[]) => getVoicesForProviderSpy(...args),
    getVoices: (...args: unknown[]) => getVoicesSpy(...args),
  },
}));

const basePickerPropsSpy = vi.fn();
vi.mock('./BasePicker', () => ({
  BasePicker: (props: { items: Array<{ value: string; label: string }>; selected: string; onSelect: (v: string) => void; loading?: boolean; error?: string | null }) => {
    basePickerPropsSpy(props);
    return (
      <div
        data-testid="base-picker"
        data-items={props.items.length}
        data-selected={props.selected}
        data-loading={props.loading ?? false}
        data-error={props.error ?? ''}
      >
        {props.items.map((item) => (
          <button
            key={item.value}
            data-testid={`item-${item.value}`}
            onClick={() => props.onSelect(item.value)}
          >
            {item.label}
          </button>
        ))}
      </div>
    );
  },
}));

const mockVoices = [
  { id: 'v1', name: 'Alice', language: 'pt-BR', provider: TTSProvider.WEBSPEECH, premium: false },
  { id: 'v2', name: 'Bob', language: 'en-US', provider: TTSProvider.WEBSPEECH, premium: false },
];

describe('VoicePicker', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getVoicesForProviderSpy.mockResolvedValue(mockVoices);
    getVoicesSpy.mockResolvedValue(mockVoices);
  });

  it('carrega vozes e inclui opcao desativada', async () => {
    render(<VoicePicker value="" onChange={() => {}} providerId="webspeech" profileId="test" />);

    await waitFor(() => {
      // 2 vozes + 1 opção "desativada" = 3
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '3');
    });
  });

  it('não inclui opção desativada quando allowDisabled=false', async () => {
    render(<VoicePicker value="" onChange={() => {}} providerId="webspeech" profileId="test" allowDisabled={false} />);

    await waitFor(() => {
      // 2 vozes, sem opção "desativada"
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2');
    });
  });

  it('inclui referências quando fornecidas', async () => {
    const references = [
      { id: VOICE_REF_ASSISTANT, label: 'Seguir assistente' },
    ];

    render(
      <VoicePicker
        value=""
        onChange={() => {}}
        providerId="webspeech"
        profileId="test"
        references={references}
      />
    );

    await waitFor(() => {
      // 2 vozes + 1 desativada + 1 referência = 4
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '4');
    });
  });

  it('chama onChange quando uma voz é selecionada', async () => {
    const handleChange = vi.fn();
    render(<VoicePicker value="" onChange={handleChange} providerId="webspeech" profileId="test" />);

    await waitFor(() => {
      expect(screen.getByTestId('item-v1')).toBeInTheDocument();
    });

    screen.getByTestId('item-v1').click();
    expect(handleChange).toHaveBeenCalledWith('v1');
  });

  it('permite selecionar a opção desativada', async () => {
    const handleChange = vi.fn();
    render(<VoicePicker value="" onChange={handleChange} providerId="webspeech" profileId="test" />);

    await waitFor(() => {
      expect(screen.getByTestId(`item-${VOICE_DISABLED}`)).toBeInTheDocument();
    });

    screen.getByTestId(`item-${VOICE_DISABLED}`).click();
    expect(handleChange).toHaveBeenCalledWith(VOICE_DISABLED);
  });

  it('exibe o valor selecionado corretamente', async () => {
    render(<VoicePicker value="v1" onChange={() => {}} providerId="webspeech" profileId="test" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-selected', 'v1');
    });
  });

  it('não carrega vozes para provider especial (disabled)', async () => {
    render(<VoicePicker value="" onChange={() => {}} providerId="disabled" profileId="test" />);

    await waitFor(() => {
      // Apenas a opção desativada
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '1');
    });

    expect(getVoicesForProviderSpy).not.toHaveBeenCalled();
  });

  it('não carrega vozes para provider ref_*', async () => {
    render(<VoicePicker value="" onChange={() => {}} providerId="ref_assistant" profileId="test" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '1');
    });

    expect(getVoicesForProviderSpy).not.toHaveBeenCalled();
  });

  it('lida com erro ao carregar vozes', async () => {
    getVoicesForProviderSpy.mockRejectedValueOnce(new Error('Voice load failed'));

    render(<VoicePicker value="" onChange={() => {}} providerId="webspeech" profileId="test" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-error', 'Voice load failed');
    });
  });

  it('busca vozes com getVoicesForProvider quando providerId é fornecido', async () => {
    render(<VoicePicker value="" onChange={() => {}} providerId="openai-1" profileId="prof-1" />);

    await waitFor(() => {
      expect(getVoicesForProviderSpy).toHaveBeenCalledWith('openai-1', '');
    });
  });

  it('busca vozes sem depender de profileId', async () => {
    render(<VoicePicker value="" onChange={() => {}} providerId="openai-1" />);

    await waitFor(() => {
      expect(getVoicesForProviderSpy).toHaveBeenCalledWith('openai-1', '');
    });
  });

  it('recarrega vozes quando providerId muda', async () => {
    const { rerender } = render(<VoicePicker value="" onChange={() => {}} providerId="webspeech" profileId="test" />);

    await waitFor(() => {
      expect(getVoicesForProviderSpy).toHaveBeenCalledWith('webspeech', '');
    });

    getVoicesForProviderSpy.mockClear();
    rerender(<VoicePicker value="" onChange={() => {}} providerId="openai-1" profileId="test" />);

    await waitFor(() => {
      expect(getVoicesForProviderSpy).toHaveBeenCalledWith('openai-1', '');
    });
  });
});
