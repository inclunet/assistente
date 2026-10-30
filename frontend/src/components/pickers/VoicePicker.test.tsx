import { describe, expect, it, vi, beforeEach } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { VoicePicker, VOICE_DISABLED, VOICE_REF_ASSISTANT } from './VoicePicker';
import { TTSProvider } from '../../services/tts/types';

const getVoicesForProviderSpy = vi.fn();
const getVoicesSpy = vi.fn();

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: vi.fn(() => () => {}) }));

vi.mock('../../services/tts', () => ({
  ttsService: {
    getVoiceSelectionForProvider: (...args: unknown[]) => getVoicesForProviderSpy(...args),
    getVoices: (...args: unknown[]) => getVoicesSpy(...args),
  },
}));

const basePickerPropsSpy = vi.fn();
vi.mock('./BasePicker', () => ({
  BasePicker: (props: { items: Array<{ value: string; label: string }>; selected: string; onSelect: (v: string) => void; loading?: boolean; error?: string | null; triggerRef?: React.Ref<HTMLButtonElement> }) => {
    basePickerPropsSpy(props);
    return (
      <div
        data-testid="base-picker"
        data-items={props.items.length}
        data-selected={props.selected}
        data-loading={props.loading ?? false}
        data-error={props.error ?? ''}
      >
        <button ref={props.triggerRef} aria-label="Abrir vozes" />
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
    getVoicesForProviderSpy.mockResolvedValue({ voices: mockVoices, completeness: 'partial', selectionMode: 'model_and_voice' });
    getVoicesSpy.mockResolvedValue(mockVoices);
  });

  it('carrega vozes e inclui opcao desativada', async () => {
    render(<VoicePicker value="" onChange={() => {}} providerId="webspeech" />);

    await waitFor(() => {
      // 2 vozes + 1 opção "desativada" = 3
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '3');
    });
  });

  it('não inclui opção desativada quando allowDisabled=false', async () => {
    render(<VoicePicker value="" onChange={() => {}} providerId="webspeech" allowDisabled={false} />);

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
    render(<VoicePicker value="" onChange={handleChange} providerId="webspeech" />);

    await waitFor(() => {
      expect(screen.getByTestId('item-v1')).toBeInTheDocument();
    });

    screen.getByTestId('item-v1').click();
    expect(handleChange).toHaveBeenCalledWith('v1');
  });

  it('permite selecionar a opção desativada', async () => {
    const handleChange = vi.fn();
    render(<VoicePicker value="" onChange={handleChange} providerId="webspeech" />);

    await waitFor(() => {
      expect(screen.getByTestId(`item-${VOICE_DISABLED}`)).toBeInTheDocument();
    });

    screen.getByTestId(`item-${VOICE_DISABLED}`).click();
    expect(handleChange).toHaveBeenCalledWith(VOICE_DISABLED);
  });

  it('exibe o valor selecionado corretamente', async () => {
    render(<VoicePicker value="v1" onChange={() => {}} providerId="webspeech" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-selected', 'v1');
    });
  });

  it('não carrega vozes para provider especial (disabled)', async () => {
    render(<VoicePicker value="" onChange={() => {}} providerId="disabled" />);

    await waitFor(() => {
      // Apenas a opção desativada
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '1');
    });

    expect(getVoicesForProviderSpy).not.toHaveBeenCalled();
  });

  it('não carrega vozes para provider ref_*', async () => {
    render(<VoicePicker value="" onChange={() => {}} providerId="ref_assistant" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '1');
    });

    expect(getVoicesForProviderSpy).not.toHaveBeenCalled();
  });

  it('lida com erro ao carregar vozes', async () => {
    getVoicesForProviderSpy.mockRejectedValueOnce(new Error('Voice load failed'));

    render(<VoicePicker value="" onChange={() => {}} providerId="webspeech" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-error', 'pickers.voice.loadError');
    });
  });

  it('busca vozes com getVoicesForProvider quando providerId é fornecido', async () => {
    render(<VoicePicker value="" onChange={() => {}} providerId="openai-1" />);

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
    const { rerender } = render(<VoicePicker value="" onChange={() => {}} providerId="webspeech" />);

    await waitFor(() => {
      expect(getVoicesForProviderSpy).toHaveBeenCalledWith('webspeech', '');
    });

    getVoicesForProviderSpy.mockClear();
    rerender(<VoicePicker value="" onChange={() => {}} providerId="openai-1" />);

    await waitFor(() => {
      expect(getVoicesForProviderSpy).toHaveBeenCalledWith('openai-1', '');
    });
  });
});


it.each(['complete', 'partial', 'unknown'])('permite entrada manual conforme completude %s e preserva voz salva', async (completeness) => {
  getVoicesForProviderSpy.mockResolvedValue({ voices: [], completeness, selectionMode: 'model_and_voice' });
  render(<VoicePicker providerId="proxy" modelId="vendor/model" value="tenant:voice-id" onChange={() => {}} />);
  await waitFor(() => expect(basePickerPropsSpy).toHaveBeenLastCalledWith(expect.objectContaining({
    busy: false, allowFreeInput: completeness !== 'complete',
    selected: 'tenant:voice-id', items: expect.arrayContaining([{ value: 'tenant:voice-id', label: 'tenant:voice-id' }]),
  })));
});

it('ignora resposta antiga após trocar o modelo', async () => {
  let resolveOld!: (value: unknown) => void;
  getVoicesForProviderSpy.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve; }));
  getVoicesForProviderSpy.mockResolvedValue({ voices: [{ id: 'new', name: 'New' }], completeness: 'complete', selectionMode: 'model_and_voice' });
  const { rerender } = render(<VoicePicker providerId="proxy" modelId="old" value="" onChange={() => {}} />);
  rerender(<VoicePicker providerId="proxy" modelId="new" value="" onChange={() => {}} />);
  await screen.findByTestId('item-new');
  resolveOld({ voices: [{ id: 'old', name: 'Old' }], completeness: 'partial', selectionMode: 'model_and_voice' });
  await waitFor(() => expect(basePickerPropsSpy).toHaveBeenLastCalledWith(expect.objectContaining({ busy: false, allowFreeInput: false })));
  expect(screen.queryByTestId('item-old')).not.toBeInTheDocument();
});


it('não consulta novamente ao editar a voz sem mudar conexão ou modelo', async () => {
 getVoicesForProviderSpy.mockResolvedValue({ voices: [], completeness: 'unknown', selectionMode: '' });
 const { rerender } = render(<VoicePicker providerId="proxy" modelId="model" value="" onChange={() => {}} />);
 await waitFor(() => expect(basePickerPropsSpy).toHaveBeenLastCalledWith(expect.objectContaining({ busy: false })));
 const calls = getVoicesForProviderSpy.mock.calls.length;
 rerender(<VoicePicker providerId="proxy" modelId="model" value="manual" onChange={() => {}} />);
 expect(getVoicesForProviderSpy.mock.calls).toHaveLength(calls);
});

it('anuncia falha na toolbar e permite repetir sem retirar o seletor manual', async () => {
  const announce = vi.fn();
  getVoicesForProviderSpy.mockRejectedValueOnce(new Error('offline'));
  render(<VoicePicker value="manual" providerId="proxy" variant="toolbar" onChange={() => {}} onAnnounce={announce} />);
  const retry = await screen.findByRole('button', { name: 'pickers.base.retry' });
  expect(announce).toHaveBeenCalledWith('pickers.voice.loadError');
  expect(screen.getByTestId('base-picker')).toHaveAttribute('data-selected', 'manual');
  let finish!: (value: unknown) => void;
  getVoicesForProviderSpy.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
  retry.focus();
  fireEvent.click(retry);
  expect(retry).toHaveFocus();
  await act(async () => { finish({ voices: [], completeness: 'unknown', selectionMode: '' }); });
  await waitFor(() => expect(screen.queryByRole('button', { name: 'pickers.base.retry' })).not.toBeInTheDocument());
  expect(screen.getByTestId('base-picker')).toHaveAttribute('data-selected', 'manual');
  expect(screen.getByRole('button', { name: 'Abrir vozes' })).toHaveFocus();
});
