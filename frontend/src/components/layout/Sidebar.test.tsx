import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import { Sidebar } from './Sidebar';

describe('Sidebar', () => {
  it('nao renderiza nada', () => {
    const { container } = render(<Sidebar />);
    expect(container.firstChild).toBeNull();
  });
});
