import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { STTProviderPicker } from './STTProviderPicker';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('./BasePicker', () => ({
  BasePicker: (props: { items: Array<{ value: string }> }) => (
    <div data-testid="base-picker" data-items={props.items.length} />
  ),
}));

describe('STTProviderPicker', () => {
  it('renderiza provedores', async () => {
    render(<STTProviderPicker value="" onChange={() => {}} />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2');
    });
  });
});
