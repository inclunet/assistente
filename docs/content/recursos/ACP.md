---
title: "Agentes ACP"
weight: 8
---

# Agentes como provedores (ACP)

O Assistente pode usar **agentes de código locais** como provedores de IA, via Agent Client Protocol (ACP) sobre stdio. Cada agente (Cursor, OpenCode, Copilot, Claude) roda como processo na sua máquina e é selecionável por perfil como qualquer provedor HTTP.

## O que é

- **Transporte**: JSON-RPC sobre `stdio` (`coder/acp-go-sdk`). Um processo por provedor, sessões multiplexadas por conversa.
- **Sessão = conversa**: o histórico fica no agente; retomar conversa usa `session/resume` quando anunciado, ou `session/load` como fallback. Se ambos falharem, cria nova sessão com aviso.
- **Sem persona/skills do app**: o turno leva só a mensagem do usuário e anexos compatíveis.

## Instalar via catálogo

1. **Configurações → Provedores → Adicionar → Agente ACP**
2. O catálogo oficial (~38 agentes) mostra estado (`instalado`, `não instalado`, requisito faltando) e integridade (digest).
3. Escolher o agente preenche `comando`, `args` e `env`; a instalação baixa o binário versionado. Sem rede, o modo manual (comando + args) continua.

## Criar manualmente

Informe `comando` (ex.: `node`, `powershell -File agent.ps1 acp`), `args` e `env`. `URL` não é necessária quando `api_format=acp`.

## Modelos e opções

Modelos vêm da sessão de descoberta do agente (cache por provider). A troca usa `set_config_option`/`set_model` conforme o agente.

Na conversa, o Assistente também mostra modos, opções booleanas e outros
seletores que o agente oferecer (por exemplo, nível de raciocínio). A lista é
atualizada pelos snapshots ACP; quando o agente remove todas as opções, os
controles antigos desaparecem. Os rótulos e valores apresentados vêm do agente.

O Assistente mantém os formatos ACP anteriores enquanto qualquer agente
suportado ou publicado no catálogo depender deles. O payload `models` e o
seletor `session/set_model` são avaliados separadamente. A remoção só pode ser
proposta quando a matriz de testes de todos os agentes oficialmente suportados
e de todas as entradas publicadas no snapshot do catálogo oficial não tiver
consumidor nem resultado desconhecido para o contrato correspondente. Não há
telemetria nem janela arbitrária por versão. O procedimento técnico está no
runbook `docs/operations/acp-compatibility-retirement.md` do repositório.

## Permissões

Todo `request_permission` do agente vira um questionário na UI (ou mensagem numerada em canais). `Permitir sempre` grava por **classe** (`execute`/`edit`/`read`) em `.assistente/acp-permissions/<perfil>.json`. Gerencie em **Configurações → Permissões do Agente**; canais e jobs nunca gravam "sempre".

## Ferramentas no histórico

As atividades de ferramentas reportadas pelo agente são preservadas no histórico
local após o fim do turno e ao reabrir a conversa, identificadas como ferramentas
do agente externo. Conclusão, falha e cancelamento mantêm estados distintos.
Esse registro não faz o Assistente executar as ferramentas do agente.

O histórico mantém a ordem entre os trechos da resposta e as ferramentas: uma
explicação anterior a uma ferramenta continua antes dela ao terminar o turno
ou reabrir a conversa. Isso também vale para turnos interrompidos ou cancelados.

O histórico conserva a classe, o resumo da atividade e sua duração. Argumentos e resultados
que não foram capturados não são inventados nem oferecidos como detalhes completos.
Atividades antigas que só existiam durante o streaming não podem ser recuperadas
retroativamente. Uma interrupção abrupta do aplicativo antes da gravação também
pode impedir o registro de uma atividade ainda em andamento.

## Limitações

- Não leva persona/skills/memória do app; não exporta tools MCP.
- Não gerencia credenciais HTTP; `acp_credential_env` injeta segredos do cofre se necessário.
- Cancelamento é serial (1 turno/sessão por vez).
- O agente pode enviar planos e detalhes ricos de execução, mas o chat ainda não os apresenta; as atividades mostram classe, resumo e estado básico.
- Conteúdo enviado ao agente inclui texto e imagem inline quando aceita. Áudio e recursos embutidos não são enviados; imagens, áudio ou recursos recebidos na resposta ainda não são exibidos, e a conversa anuncia essa limitação.
- Se o agente reportar uso ACP, o painel de tokens mostra a ocupação atual e o custo acumulado da sessão enquanto a janela está aberta; avisos aparecem ao atingir 80% e 95%. Esses números vêm do próprio agente e não são salvos como contabilidade local.
- O catálogo instala e inicia agentes, mas não garante que todos os recursos opcionais do ACP estejam integrados.
- O Assistente não lista sessões que o agente conhece fora das conversas locais, nem oferece várias raízes de workspace na mesma sessão.
- Envios manuais que aguardam a vez aparecem como pendentes na barra; **Cancelar próximo envio** remove o próximo envio manual que ainda não começou e anuncia o resultado. Mensagens recebidas por canais continuam na fila, e o turno ativo segue normalmente.
