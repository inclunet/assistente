---
title: "Gestão de dados"
weight: 17
---

# Gestão de dados

Na tela **Configurações → Dados** (`/settings/data`), você encontra a
importação de dados e a manutenção do armazenamento do Assistente. O roteiro
abaixo trata da exportação e importação de camadas de comandos; a seção de
manutenção explica o banco SQLite, suas migrações e sua compactação.

## Exportar camadas de comandos

No painel **Exportar camadas de comandos**, a exportação comum gera um arquivo
JSON versão **2**, com o recurso `resources.commandLayers`. Por padrão, o
arquivo inclui somente o escopo global. Marque **Incluir workspaces acessíveis**
para incluir também as camadas dos workspaces que o backend
autenticado apresentar como acessíveis; a caixa de seleção não amplia a
permissão nem permite informar workspaces manualmente.

O pacote exportado contém as camadas, seus bindings, regras e contêineres de
deltas builtin. Defaults puros não são materializados no arquivo. Também ficam
fora do pacote claims, grants, histórico de invocações e o owner: identidade e
autorização são sempre derivadas no destino autenticado. Os IDs persistidos são
preservados, assim como as referências entre os registros.

Antes de gerar o arquivo, o backend valida o catálogo completo, os argumentos
sensíveis e as referências de credenciais por contrato. O resultado não inclui
valores secretos. O arquivo inteiro deve ter no máximo **64 KiB** e conter no
máximo **64 entradas**, contando camadas e contêineres de delta builtin. Um
arquivo vazio, sem `resources.commandLayers`, ou com qualquer registro inválido
é recusado por inteiro; nenhuma parte é salva como backup parcial.

### Roundtrip do arquivo exportado

Para levar a configuração a outra instância, use o arquivo gerado e, no painel
de importação, confirme o lote inteiro:

1. Abra **Configurações → Dados**, selecione o arquivo exportado e confira o
   preview.
2. Escolha a política **Manter (keep)** ou **Copiar (copy)** para o lote. Em
   **Copiar**, os IDs das cópias são novos e as referências internas são
   remapeadas; em **Manter**, o destino existente é preservado.
3. Preencha nomes novos individualmente quando a cópia exigir nome disponível;
   não presuma que um nome único será aplicado a todas as camadas.
4. Mapeie explicitamente cada workspace de origem para um destino real
   oferecido pelo backend. O escopo global permanece global.
5. Aceite todas as confirmações de escopo apresentadas. Recusar ou cancelar
   qualquer confirmação cancela o lote inteiro, sem aplicação parcial.

O caminho comum não inclui credenciais nem senhas. Uma ação separada para
**Incluir credenciais** depende de decisão explícita da UI, confirmação de alto
risco e criptografia ainda pendente; portanto, permanece indisponível. Não use
a exportação genérica como alternativa para incluir segredos neste arquivo.

## Importar camadas de comandos

A importação de camadas aceita somente um envelope JSON de configuração com
`resources.commandLayers`. O arquivo pode estar na versão **1 ou 2**; a versão
1 publicada é normalizada internamente para a versão 2. Versões desconhecidas
são recusadas.

O arquivo inteiro precisa conter apenas esse recurso. Se trouxer
`resources.credentials`, conversas, providers, tasklists, memórias ou qualquer
outro recurso, a importação é recusada por inteiro — esses itens não são
ignorados silenciosamente nem misturados neste fluxo. O limite do arquivo é
**64 KiB**.

### Antes de confirmar

Depois de selecionar o JSON, confira o preview e trate o arquivo como um lote
único:

- escolha **uma vez para o lote inteiro** o modo **Manter (keep)**,
  **Substituir (replace)** ou **Copiar (copy)**;
- em **Manter**, o destino existente é preservado; se o resultado não mudar
  nada, o app informa um no-op;
- em **Substituir**, o conteúdo da camada correspondente no destino é
  atualizado;
- em **Copiar**, a camada e seus itens recebem novos IDs, e as referências
  internas são remapeadas. A cópia exige um nome disponível;
- se necessário, informe um novo nome **por camada**. Um nome único não é
  aplicado automaticamente a todas as camadas;
- para camadas de workspace, mapeie cada origem para um workspace de destino
  real e disponível. A lista de destinos vem do backend autenticado; não crie
  nem digite um ID inventado. O workspace global continua sendo global;
- confirme cada escopo que o backend apresentar (global ou workspace). Nenhuma
  alteração é gravada antes de todas as confirmações necessárias serem aceitas.
  Recusar ou cancelar qualquer confirmação cancela o lote inteiro.

O preview é uma análise sem gravação. O commit é feito como um único lote,
com revalidação de posse, nomes, referências, catálogo e destinos. O envelope
não transporta a identidade do dono, claims, grants, recibos ou histórico de
execução; a identidade e a autorização são derivadas no destino autenticado.

Regras de ativação por evento entram **desabilitadas** e sem claims ou grants.
A importação não concede autorização para executar comandos nem ativa eventos
externos por conta própria.

### Relatório e falhas

Ao terminar, leia o relatório redigido. Avisos são agregados por código e
quantidade; o relatório não mostra secrets, valores de credenciais, padrões de
credencial, argumentos de comandos ou conteúdo privado de outra pessoa.

Se a tela informar que o commit foi **gravado**, mas o mapa de comandos **não
foi publicado**, não repita a importação. Aguarde o diagnóstico ou a recarga
indicada pelo app; se for necessário, feche e reabra o Assistente uma vez e
aguarde a reconstrução. Uma nova importação pode criar outra cópia.

O fluxo comum não habilita exportação de segredos. Valores secretos brutos não
fazem parte do envelope; referências a credenciais, quando aceitas pelo
contrato, não carregam o segredo. A ação separada de exportação sensível segue
indisponível até haver decisão de UI de alto risco e criptografia.

### Roteiro manual mínimo

Use uma **cópia do arquivo de dados para o roteiro**, não o seu banco pessoal
nem dados pessoais de produção. As caixas abaixo são um roteiro para execução
manual pela interface do aplicativo já aberto; não exigem comandos de build,
CLI ou executáveis de teste e não substituem o relatório do app.

- [ ] Abrir **Configurações → Dados** e selecionar o arquivo JSON.
- [ ] Usar um arquivo na versão 1 ou 2, com no máximo 64 KiB e presença
      exclusiva de `resources.commandLayers`; conferir as camadas no painel.
- [ ] Confirmar que não há credenciais nem outros recursos no arquivo.
- [ ] Escolher Manter, Substituir ou Copiar para o lote inteiro.
- [ ] Verificar os destinos de workspace oferecidos pelo backend e mapear
      somente workspaces reais disponíveis.
- [ ] Revisar e preencher os nomes individualmente nas camadas que precisam
      ser renomeadas.
- [ ] Aceitar todas as confirmações de escopo antes de qualquer commit; se uma
      for recusada, confirmar que o lote não foi aplicado.
- [ ] Ler o relatório agregado e verificar que não há secrets ou conteúdo
      privado nele.
- [ ] Se houver falha pós-commit com mapa não publicado, não repetir; aguardar
      diagnóstico/recarga e só então fechar e reabrir o app se essa orientação
      aparecer.

## Manutenção e retenção

Na implementação AEP-0103, a manutenção de comandos é montada antes do início
dos jobs quando o armazenamento de comandos está pronto. Ela compartilha o
timer existente: verifica leases e outbox antes de reter jobs e compactar.
Falha de montagem impede esse início e é registrada no diagnóstico de reload;
não inicia uma limpeza alternativa silenciosa. Encerrar o runtime aguarda a
passagem em andamento antes de liberar a instância do banco.

Isso não conclui toda a AEP: a projeção de claims de jobs já está ligada ao
resolvedor do App, mas a integração completa de acionadores e contextos
continua em desenvolvimento. A tasklist AEP-0103 delimita os aceites e a
qualificação integrada ainda pendente.

Jobs encadeados preservam a origem da execução e o histórico de comandos em
contexto interno, inclusive quando um job chama outro pela ferramenta de jobs.
Campos de origem recebidos no payload público não concedem permissão para
ativar camadas. Eventos legados sem origem comprovada continuam executando
seus jobs, mas não passam a autorizar camadas por atravessarem um job interno.

A prova de execução viva de um job não é encerrada por uma reconstrução da
configuração de comandos. Uma mudança de perfil pode retirar sua claim
contextual quando a condição deixa de valer, sem cancelar o job. Invalidações
de segurança continuam encerrando essa autoridade. Uma claim de job só
sustenta a camada no resolvedor enquanto a lease, o runtime e sua condição
continuam válidos. Renovar essa prova sem mudar o mapa de comandos não
interrompe comandos em andamento. Isso não habilita acionadores físicos nem
substitui a configuração e autorização das regras.

### Reinício e comandos interrompidos

O sistema de comandos mantém exclusividade nativa sobre o arquivo do banco
enquanto seus executores estiverem ativos. Uma segunda instância participante
não monta os comandos no mesmo arquivo até a primeira liberar essa posse.
Isso não é um bloqueio de todas as funcionalidades de versões antigas.

Na próxima inicialização, pendências de gerações registradas pelo protocolo
anterior são reconciliadas antes de publicar o catálogo. Invocações interrompidas
ficam com resultado desconhecido (`outcome_unknown`), sem repetir automaticamente
a ação. Confira o efeito antes de iniciar uma nova tentativa. Decisões abandonadas
são canceladas ou expiradas, não aceitas automaticamente.

Pendências sem origem comprovada — inclusive de versões anteriores ao protocolo
ou de outra cópia física do banco — não são alteradas por suposição. Elas podem
manter o catálogo indisponível até uma análise segura. Não apague registros
manualmente para contornar essa proteção.

A migração 29 acrescenta o registro interno das gerações participantes. Não
cria segredo adicional no gerenciador de credenciais. Antes de voltar a uma
versão anterior do app, preserve uma cópia do banco: compatibilidade reversa
do novo schema de comandos não é garantida.

Ao resetar o banco pelo app, os comandos são encerrados e sua exclusividade
é liberada antes do fechamento/remoção. Se o encerramento falhar, o reset
é recusado antes de fechar o banco. Reinicie o app após o reset para montar
os comandos sobre o banco novo. Nenhum reset é necessário para atualizar.

### Campos de retenção

Na aba **Dados**, a seção de manutenção permite ajustar a retenção da
infraestrutura de integração. Salvar esses valores não ativa atalhos, listeners,
adapters, handlers ou execuções; eles só controlam a manutenção quando o
coordenador correspondente está integrado.

Os seis campos de comandos são:

- `command_invocation_retention_days` — dias de retenção da auditoria detalhada
  de invocações. Padrão: **30**.
- `command_invocations_per_user_keep` — máximo de auditorias terminais por
  usuário. Padrão: **10.000**.
- `command_invocations_system_keep` — máximo de auditorias terminais internas
  sem usuário. Padrão: **1.000**.
- `command_activation_terminal_retention_days` — dias de retenção da auditoria
  e do estado terminal das ativações. Padrão: **30**. Estados ativos não entram
  na limpeza; a barreira de replay do ledger é preservada.
- `command_activation_terminal_keep_per_user` — máximo de ativações terminais
  mantidas por usuário. Padrão: **10.000**.
- `command_job_activation_lease_seconds` — validade, em segundos, da claim de
  ativação de job mantida pelo heartbeat do runtime. Padrão: **180**. Esse valor
  não representa tempo de processamento do outbox.

Os limites de idade e quantidade se aplicam à auditoria/estado terminal;
registros ativos e barreiras de replay não são removidos por um cap de
quantidade. A ausência de integração do recurso mantém a execução desabilitada.

Ao salvar a política, a próxima passagem da manutenção usa os novos valores
em conjunto: heartbeat, novas ativações e retenções recebem o mesmo snapshot.
Uma passagem já em andamento termina com a política que capturou ao começar;
salvar não dispara uma segunda rotina. Reduzir a retenção não encurta os prazos
de replay já gravados, nem permite apagar chaves ainda necessárias para evitar
reentrega duplicada. Uma lease vencida não é ressuscitada pelo novo valor.

### Diagnóstico da manutenção de comandos

Uma falha é registrada como `instance maintenance failed` com `stage` e
contadores do trabalho já confirmado. As etapas distinguem heartbeat,
reencaminhamento/consumo/purga da outbox, recuperação de decisões/invocações/
claims, retenções e compactação. Esses campos não incluem argumentos de
comandos, segredos ou identidade de usuário. O erro original é preservado.

`settings unavailable` ou `invalid complete policy` indicam que a passagem
foi recusada antes das limpezas. Corrija a configuração ou a indisponibilidade
indicada pelo erro; não apague ledgers nem force a expiração de claims para
contornar a proteção. A cadência tenta novamente; trabalho já confirmado não
é desfeito. Havendo continuação ou erro, não se certifica compactação concluída.

A ausência de workspace/contexto visual não bloqueia a retirada de claims
sem fonte viva, inclusive de outros usuários. Não concede permissão para
ativar ou renovar uma camada sem o contexto exigido.

Os horários internos da outbox de comandos, da política de replay e de suas
leases são gravados e comparados em UTC. O fuso local não antecipa a expiração
nem faz um evento válido parecer anterior à política. Não é necessário mudar
o fuso do computador para executar jobs ou realizar a manutenção.
