import { describe, it, expect } from 'vitest';
import type { TFunction } from 'i18next';

import {
  chatNoticeMessage,
  chatNoticeTone,
  CHAT_NOTICE_ATTACHMENTS_NOT_SENT,
  CHAT_NOTICE_PERMISSION_ALWAYS_ALLOWED,
  CHAT_NOTICE_PERMISSION_ALWAYS_NOT_SAVED,
  CHAT_NOTICE_PERMISSION_NO_WATCHER,
  CHAT_NOTICE_PERMISSION_TIMEOUT,
  CHAT_NOTICE_PERMISSION_UNAVAILABLE,
  CHAT_NOTICE_PLAN_NO_WATCHER,
  CHAT_NOTICE_PLAN_TIMEOUT,
  CHAT_NOTICE_PLAN_UNAVAILABLE,
  CHAT_NOTICE_QUESTION_NO_WATCHER,
  CHAT_NOTICE_QUESTION_TIMEOUT,
  CHAT_NOTICE_QUESTION_UNAVAILABLE,
  CHAT_NOTICE_MODEL_NOT_APPLIED,
  CHAT_NOTICE_MODEL_NOT_OFFERED,
  CHAT_NOTICE_AGENT_MEMORY_LOST,
  CHAT_NOTICE_MODE_SKIPS_PERMISSION,
  CHAT_NOTICE_MODE_ASKS_PERMISSION,
} from './chatNotice';

// Mock de TFunction: ecoa a chave + os args interpolados.
const t = ((key: string, opts?: Record<string, unknown>) =>
  opts ? `${key}|${JSON.stringify(opts)}` : key) as unknown as TFunction;

describe('chatNoticeMessage', () => {
  it('traduz o motivo nomeado pelo backend, com a quantidade', () => {
    const message = chatNoticeMessage(t, {
      conversationId: 'conversa-1',
      kind: CHAT_NOTICE_ATTACHMENTS_NOT_SENT,
      count: 2,
    });

    expect(message).toContain('app.chatNotice.attachmentsNotSent|');
    expect(message).toContain('"count":2');
  });

  it('não inventa aviso para motivo desconhecido', () => {
    expect(chatNoticeMessage(t, { kind: 'motivo_novo', count: 1 })).toBeNull();
    expect(chatNoticeMessage(t, {})).toBeNull();
  });

  it('diz o que foi negado quando não havia quem respondesse', () => {
    const message = chatNoticeMessage(t, {
      conversationId: 'conversa-1',
      kind: CHAT_NOTICE_PERMISSION_NO_WATCHER,
      action: 'execute',
    });

    expect(message).toContain('app.chatNotice.permissionNoWatcher|');
    expect(message).toContain('"action":"app.chatNotice.action.execute"');
  });

  it('distingue prazo estourado de pedido que não pôde ser mostrado', () => {
    const prazo = chatNoticeMessage(t, { kind: CHAT_NOTICE_PERMISSION_TIMEOUT, action: 'edit' });
    const semTela = chatNoticeMessage(t, {
      kind: CHAT_NOTICE_PERMISSION_UNAVAILABLE,
      action: 'edit',
    });

    expect(prazo).toContain('app.chatNotice.permissionTimeout|');
    expect(semTela).toContain('app.chatNotice.permissionUnavailable|');
  });

  it('classe de ação desconhecida vira o termo genérico, e não código cru', () => {
    const message = chatNoticeMessage(t, {
      kind: CHAT_NOTICE_PERMISSION_TIMEOUT,
      action: 'invocar_o_kraken',
    });

    expect(message).toContain('"action":"app.chatNotice.action.unknown"');
    expect(message).not.toContain('invocar_o_kraken');
  });

  it('conta a autorização permanente com o nome que a tela de revogar usa', () => {
    const message = chatNoticeMessage(t, {
      conversationId: 'conversa-1',
      kind: CHAT_NOTICE_PERMISSION_ALWAYS_ALLOWED,
      action: 'execute',
    });

    expect(message).toContain('app.chatNotice.permissionAlwaysAllowed|');
    expect(message).toContain('"actionClass":"agentPermissions.action.execute"');
  });

  it('avisa quando o sempre não pôde ser guardado', () => {
    const message = chatNoticeMessage(t, {
      kind: CHAT_NOTICE_PERMISSION_ALWAYS_NOT_SAVED,
      action: 'edit',
    });

    expect(message).toContain('app.chatNotice.permissionAlwaysNotSaved|');
    expect(message).toContain('"actionClass":"agentPermissions.action.edit"');
  });

  it('conta que a pergunta do agente passou sem que ninguém respondesse', () => {
    expect(chatNoticeMessage(t, { kind: CHAT_NOTICE_QUESTION_NO_WATCHER })).toContain(
      'app.chatNotice.questionNoWatcher|',
    );
    expect(chatNoticeMessage(t, { kind: CHAT_NOTICE_QUESTION_TIMEOUT })).toContain(
      'app.chatNotice.questionTimeout|',
    );
    expect(chatNoticeMessage(t, { kind: CHAT_NOTICE_QUESTION_UNAVAILABLE })).toContain(
      'app.chatNotice.questionUnavailable|',
    );
  });

  it('conta que o plano foi recusado sem que ninguém decidisse', () => {
    expect(chatNoticeMessage(t, { kind: CHAT_NOTICE_PLAN_NO_WATCHER })).toContain(
      'app.chatNotice.planNoWatcher|',
    );
    expect(chatNoticeMessage(t, { kind: CHAT_NOTICE_PLAN_TIMEOUT })).toContain(
      'app.chatNotice.planTimeout|',
    );
    expect(chatNoticeMessage(t, { kind: CHAT_NOTICE_PLAN_UNAVAILABLE })).toContain(
      'app.chatNotice.planUnavailable|',
    );
  });

  it('diz que modelo atendeu quando o escolhido não pôde valer', () => {
    const naoOferecido = chatNoticeMessage(t, {
      conversationId: 'conversa-1',
      kind: CHAT_NOTICE_MODEL_NOT_OFFERED,
      model: 'sonnet-4.5',
    });
    const naoAplicado = chatNoticeMessage(t, {
      kind: CHAT_NOTICE_MODEL_NOT_APPLIED,
      model: 'sonnet-4.5',
    });

    expect(naoOferecido).toContain('app.chatNotice.modelNotOffered|');
    // O nome do modelo precisa chegar à frase: sem ele o aviso diz que a
    // resposta veio de outro modelo sem dizer de qual.
    expect(naoOferecido).toContain('"model":"sonnet-4.5"');
    expect(naoAplicado).toContain('app.chatNotice.modelNotApplied|');
    expect(naoAplicado).toContain('"model":"sonnet-4.5"');
  });

  it('conta que o agente reabriu a conversa sem a memória anterior', () => {
    const message = chatNoticeMessage(t, {
      conversationId: 'conversa-1',
      kind: CHAT_NOTICE_AGENT_MEMORY_LOST,
    });

    expect(message).toContain('app.chatNotice.agentMemoryLost|');
  });

  it('conta que o modo escolhido dispensa a autorização, nomeando o modo', () => {
    const message = chatNoticeMessage(t, {
      conversationId: 'conversa-1',
      kind: CHAT_NOTICE_MODE_SKIPS_PERMISSION,
      mode: 'Não perguntar',
    });

    expect(message).toContain('app.chatNotice.modeSkipsPermission|');
    // Sem o nome do modo o aviso diria que o agente parou de perguntar sem
    // dizer o que desfazer para ele voltar a perguntar.
    expect(message).toContain('"mode":"Não perguntar"');
  });

  it('conta que a barreira voltou quando o modo pergunta de novo', () => {
    const message = chatNoticeMessage(t, {
      conversationId: 'conversa-1',
      kind: CHAT_NOTICE_MODE_ASKS_PERMISSION,
      mode: 'Plan Mode',
    });

    expect(message).toContain('app.chatNotice.modeAsksPermission|');
    expect(message).toContain('"mode":"Plan Mode"');
  });
});

describe('chatNoticeTone', () => {
  it('a autorização concedida é informação, não alerta', () => {
    expect(chatNoticeTone(CHAT_NOTICE_PERMISSION_ALWAYS_ALLOWED)).toBe('info');
  });

  it('a barreira de permissão que voltou é informação, não alerta', () => {
    expect(chatNoticeTone(CHAT_NOTICE_MODE_ASKS_PERMISSION)).toBe('info');
  });

  it('o que atrapalhou o turno continua sendo alerta', () => {
    expect(chatNoticeTone(CHAT_NOTICE_PERMISSION_ALWAYS_NOT_SAVED)).toBe('warning');
    expect(chatNoticeTone(CHAT_NOTICE_PERMISSION_NO_WATCHER)).toBe('warning');
    expect(chatNoticeTone(undefined)).toBe('warning');
  });

  // O modo que dispensa a autorização é a única mudança de comportamento que o
  // app não consegue mais barrar: o alerta é lido em voz alta antes da frase, e
  // é ele que distingue este aviso de uma notícia qualquer sobre o turno.
  it('o modo que dispensa a autorização é alerta', () => {
    expect(chatNoticeTone(CHAT_NOTICE_MODE_SKIPS_PERMISSION)).toBe('warning');
  });
});
