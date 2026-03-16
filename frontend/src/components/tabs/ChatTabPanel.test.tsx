import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ChatTabPanel } from './ChatTabPanel';

vi.mock('../../store/chatStore', () => ({
  useChatStore: () => ({
    tabs: [],
    activeTabId: null,
  }),
}));

describe('ChatTabPanel', () => {
  it('renderiza estado vazio quando nao ha aba ativa', () => {
    render(<ChatTabPanel />);

    expect(screen.getByText('Nenhuma aba ativa')).toBeInTheDocument();
  });
});
