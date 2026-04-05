import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { STTProviderPicker } from './STTProviderPicker';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@wailsjs/go/main/App', () => ({
  GetLLMProviders: vi.fn().mockResolvedValue([
    { id: 'openai-1', name: 'OpenAI', api_format: 'openai', base_url: 'https://api.openai.com/v1' },
  ]),
}));

vi.mock('./BasePicker', () => ({
  BasePicker: (props: { items: Array<{ value: string }> }) => (
    <div data-testid="base-picker" data-items={props.items.length} />
  ),
}));

describe('STTProviderPicker', () => {
  it('renderiza webspeech + provedores LLM', async () => {
    render(<STTProviderPicker value="" onChange={() => {}} />);

    await waitFor(() => {
      // 1 webspeech + 1 LLM provider = 2
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2');
    });
  });
});
