import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { CommandLayerActionFields } from './CommandLayerActionFields';
import type { CommandLayer, CommandSettingsRule } from '../../types/commandSettingsTypes';

const layers: CommandLayer[] = [{ id: 'layer-private', name: 'Trabalho', description: '', builtin: false, enabled: true, active: false, manualReady: true, manualActive: false }];
const rules: CommandSettingsRule[] = [{ id: 'rule-private', layerId: 'layer-private', mode: 'manual', lifecycle: 'temporary', condition: { version: 1, clauses: [] }, enabled: true, reviewStatus: 'active' }];

describe('CommandLayerActionFields', () => {
  it('seleciona a regra pelo nome sem exibir IDs e permite duração explícita', () => {
    const change = vi.fn(); const valid = vi.fn();
    const { rerender } = render(<CommandLayerActionFields commandID="layer.activate" scope="global" layers={layers} rules={rules} value={{}} onChange={change} onValidityChange={valid} />);
    expect(valid).toHaveBeenLastCalledWith(false);
    expect(screen.queryByText('rule-private')).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('commandSettings.layerActionTarget'), { target: { value: 'rule-private' } });
    expect(change).toHaveBeenLastCalledWith({ scope: 'global', rule_id: 'rule-private', duration_seconds: 60 });
    rerender(<CommandLayerActionFields commandID="layer.activate" scope="global" layers={layers} rules={rules} value={change.mock.calls[0][0]} onChange={change} onValidityChange={valid} />);
    expect(valid).toHaveBeenLastCalledWith(true);
    fireEvent.change(screen.getByLabelText('commandSettings.manualDurationSeconds'), { target: { value: '42' } });
    expect(change).toHaveBeenLastCalledWith({ scope: 'global', rule_id: 'rule-private', duration_seconds: 42 });
  });
  it('não substitui referência removida, de outro escopo ou regra desabilitada', () => {
    const change = vi.fn(); const valid = vi.fn();
    const { rerender } = render(<CommandLayerActionFields commandID="layer.toggle" scope="global" layers={layers} rules={rules} value={{ scope: 'workspace', rule_id: 'rule-private', duration_seconds: 60 }} onChange={change} onValidityChange={valid} />);
    expect(valid).toHaveBeenLastCalledWith(false);
    expect(screen.getByRole('option', { name: 'commandSettings.unavailableConditionValue' })).toBeDisabled();
    rerender(<CommandLayerActionFields commandID="layer.toggle" scope="global" layers={layers} rules={[{ ...rules[0], enabled: false }]} value={{ scope: 'global', rule_id: 'rule-private', duration_seconds: 60 }} onChange={change} onValidityChange={valid} />);
    expect(valid).toHaveBeenLastCalledWith(false);
    expect(change).not.toHaveBeenCalled();
  });
  it('voltar não aceita uma regra arbitrária nem argumentos de autoridade', () => {
    const change = vi.fn(); const valid = vi.fn();
    render(<CommandLayerActionFields commandID="layer.back" scope="workspace" layers={layers} rules={rules} value={{ scope: 'workspace', rule_id: 'rule-private', duration_seconds: 0, owner: 'forged' }} onChange={change} onValidityChange={valid} />);
    expect(valid).toHaveBeenLastCalledWith(false);
    fireEvent.change(screen.getByLabelText('commandSettings.layerActionScope'), { target: { value: 'workspace' } });
    expect(change).toHaveBeenLastCalledWith({ scope: 'workspace', rule_id: '', duration_seconds: 0 });
  });
  it('exige o contrato completo de voltar sem preencher argumentos ausentes silenciosamente', () => {
    const change = vi.fn(); const valid = vi.fn();
    const { rerender } = render(<CommandLayerActionFields commandID="layer.back" scope="global" layers={layers} rules={rules} value={{ scope: 'global', duration_seconds: 0 }} onChange={change} onValidityChange={valid} />);
    expect(valid).toHaveBeenLastCalledWith(false);
    expect(change).not.toHaveBeenCalled();
    rerender(<CommandLayerActionFields commandID="layer.back" scope="global" layers={layers} rules={rules} value={{ scope: 'global', rule_id: '', duration_seconds: 0 }} onChange={change} onValidityChange={valid} />);
    expect(valid).toHaveBeenLastCalledWith(true);
  });
});
