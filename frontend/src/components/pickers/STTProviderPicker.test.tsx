import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { STTProviderPicker } from './STTProviderPicker';
import { GetSpeechProviders } from '@wailsjs/go/app/App';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetSpeechProviders: vi.fn().mockResolvedValue([
    { id: 'openai-1', name: 'OpenAI', api_format: 'openai', base_url: 'https://api.openai.com/v1' },
  ]),
}));

const basePickerPropsSpy = vi.fn();
vi.mock('./BasePicker', () => ({
  BasePicker: (props: { items: Array<{ value: string; label: string; sublabel?: string }>; selected: string; onSelect: (v: string) => void; loading?: boolean; error?: string | null; onRetry?: () => void }) => {
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
        {props.onRetry && <button data-testid="retry-btn" onClick={props.onRetry}>retry</button>}
      </div>
    );
  },
}));

describe('STTProviderPicker', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (GetSpeechProviders as ReturnType<typeof vi.fn>).mockResolvedValue([
      { id: 'openai-1', name: 'OpenAI', api_format: 'openai', base_url: 'https://api.openai.com/v1' },
    ]);
  });

  it('renderiza webspeech + provedores LLM', async () => {
    render(<STTProviderPicker value="" onChange={() => {}} />);

    await waitFor(() => {
      // 1 webspeech + 1 LLM provider = 2
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2');
    });
  });

  it('exibe webspeech como primeiro item', async () => {
    render(<STTProviderPicker value="webspeech" onChange={() => {}} />);

    await waitFor(() => {
      expect(screen.getByTestId('item-webspeech')).toBeInTheDocument();
    });
  });

  it('seleciona webspeech e chama onChange com provider webspeech', async () => {
    const handleChange = vi.fn();
    render(<STTProviderPicker value="" onChange={handleChange} />);

    await waitFor(() => {
      expect(screen.getByTestId('item-webspeech')).toBeInTheDocument();
    });

    screen.getByTestId('item-webspeech').click();
    expect(handleChange).toHaveBeenCalledWith('webspeech', undefined);
  });

  it('seleciona provider LLM e chama onChange com whisper_api + llmProviderId', async () => {
    const handleChange = vi.fn();
    render(<STTProviderPicker value="" onChange={handleChange} />);

    await waitFor(() => {
      expect(screen.getByTestId('item-openai-1')).toBeInTheDocument();
    });

    screen.getByTestId('item-openai-1').click();
    expect(handleChange).toHaveBeenCalledWith('whisper_api', 'openai-1');
  });

  it('usa providers externos quando fornecidos (sem fetch)', async () => {
    const externalProviders = [
      { id: 'ext-1', name: 'External', api_format: 'openai', base_url: 'https://ext.example.com/v1' },
      { id: 'ext-2', name: 'External 2', api_format: 'openai', base_url: 'https://ext2.example.com/v1' },
    ] as never[];

    render(<STTProviderPicker value="" onChange={() => {}} providers={externalProviders} />);

    await waitFor(() => {
      // 1 webspeech + 2 external = 3
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '3');
    });

    // Não deve ter chamado GetSpeechProviders
    expect(GetSpeechProviders).not.toHaveBeenCalled();
  });

  it('renderiza múltiplos providers LLM', async () => {
    (GetSpeechProviders as ReturnType<typeof vi.fn>).mockResolvedValue([
      { id: 'openai-1', name: 'OpenAI', api_format: 'openai', base_url: 'https://api.openai.com/v1' },
      { id: 'deepseek-1', name: 'DeepSeek', api_format: 'openai', base_url: 'https://api.deepseek.com/v1' },
    ]);

    render(<STTProviderPicker value="" onChange={() => {}} />);

    await waitFor(() => {
      // 1 webspeech + 2 LLM providers = 3
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '3');
    });
  });

  it('exibe o valor selecionado corretamente para webspeech', async () => {
    render(<STTProviderPicker value="webspeech" onChange={() => {}} />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-selected', 'webspeech');
    });
  });

  it('exibe o valor selecionado corretamente para provider LLM', async () => {
    render(<STTProviderPicker value="openai-1" onChange={() => {}} />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-selected', 'openai-1');
    });
  });

  it('lida com erro ao carregar providers', async () => {
    (GetSpeechProviders as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('Network error'));

    render(<STTProviderPicker value="" onChange={() => {}} />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-error', 'Network error');
    });
  });

  it('exibe estado de loading enquanto carrega providers', () => {
    // Simula promise que nunca resolve (ainda loading)
    (GetSpeechProviders as ReturnType<typeof vi.fn>).mockReturnValue(new Promise(() => {}));

    render(<STTProviderPicker value="" onChange={() => {}} />);

    expect(screen.getByTestId('base-picker')).toHaveAttribute('data-loading', 'true');
  });

  it('permite retry após erro', async () => {
    (GetSpeechProviders as ReturnType<typeof vi.fn>)
      .mockRejectedValueOnce(new Error('fail'))
      .mockResolvedValueOnce([
        { id: 'openai-1', name: 'OpenAI', api_format: 'openai', base_url: 'https://api.openai.com/v1' },
      ]);

    render(<STTProviderPicker value="" onChange={() => {}} />);

    await waitFor(() => {
      expect(screen.getByTestId('retry-btn')).toBeInTheDocument();
    });

    screen.getByTestId('retry-btn').click();

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2');
    });
  });
});
