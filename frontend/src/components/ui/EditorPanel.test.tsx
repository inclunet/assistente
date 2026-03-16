import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { EditorPanel, EditorPanelFields, EditorPanelFooter } from './EditorPanel';

describe('EditorPanel', () => {
  it('renderiza container e secoes', () => {
    render(
      <EditorPanel>
        <EditorPanelFields>Campos</EditorPanelFields>
        <EditorPanelFooter>Rodape</EditorPanelFooter>
      </EditorPanel>
    );

    expect(screen.getByText('Campos')).toBeInTheDocument();
    expect(screen.getByText('Rodape')).toBeInTheDocument();
  });
});
