import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { LLMProviderPicker } from './LLMProviderPicker';

const getProvidersSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetLLMProviders: () => getProvidersSpy(),
}));

vi.mock('./BasePicker', () => ({
  BasePicker: (props: { items: Array<{ value: string }>; loading?: boolean; error?: string | null }) => (
    <div data-testid="base-picker" data-items={props.items.length} data-loading={props.loading ? 'yes' : 'no'} data-error={props.error ?? ''} />
  ),
}));

describe('LLMProviderPicker', () => {
  it('carrega provedores e renderiza itens', async () => {
    getProvidersSpy.mockResolvedValueOnce([
      { id: 'p1', name: 'Provider', type: 'x', base_url: 'http://x' },
    ]);

    render(<LLMProviderPicker value="" onChange={() => {}} />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '1');
    });
  });
});
