import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { ModelPicker } from './ModelPicker';

const getModelsSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@wailsjs/go/main/App', () => ({
  GetModels: () => getModelsSpy(),
  GetModelsByProvider: (providerId: string) => getModelsSpy(providerId),
}));

vi.mock('./BasePicker', () => ({
  BasePicker: (props: { items: Array<{ value: string }>; allowFreeInput?: boolean; error?: string | null }) => (
    <div data-testid="base-picker" data-items={props.items.length} data-allowfree={props.allowFreeInput ? 'yes' : 'no'} data-error={props.error ?? ''} />
  ),
}));

describe('ModelPicker', () => {
  it('carrega modelos por provider', async () => {
    getModelsSpy.mockResolvedValueOnce(['m1']);

    render(<ModelPicker value="" onChange={() => {}} providerID="p1" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '1');
    });
  });

  it('habilita input livre quando endpoint nao suportado', async () => {
    getModelsSpy.mockRejectedValueOnce('models_endpoint_not_supported');

    render(<ModelPicker value="" onChange={() => {}} providerID="p1" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-allowfree', 'yes');
    });
  });
});
