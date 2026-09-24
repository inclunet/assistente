---
title: "Cadastro administrativo de identidades externas"
weight: 26
---

# Cadastro administrativo de identidades externas

Este recurso prepara a migração de uma instalação com `auth.mode=external`.
Ele **não é necessário para usar teclado, paleta ou Stream Deck no modo local**.
Não cria usuário, senha ou segredo: associa uma identidade do provedor externo
a um usuário local já existente e ativo.

O middleware externo exige esses vínculos: `/auth/me` retorna o ID do usuário
local associado, não o `sub` do provedor, e mantém a role do token externo.
O cadastro **não habilita comandos externos** nem dispositivos físicos externos.

Na política interna de comandos externos, as permissões vêm das roles e scopes
do token validado. Tornar o usuário vinculado administrador local não concede
roles externas. Uma regra pode aceitar qualquer uma das roles que lista, mas
continua exigindo todos os scopes configurados. Essa política ainda depende da
montagem do ingresso externo para ser disponibilizada aos clientes.

Ao atualizar uma instalação externa, conclua o bootstrap abaixo e cadastre as
demais contas antes de usá-las. Sem bootstrap do emissor, `/auth/me` responde
**503**; sem vínculo habilitado ou usuário ativo, responde **401**, inclusive
quando o subject já coincide com um ID local. Não há conversão automática.
O acesso é relido a cada solicitação; não há cache de vínculo que mantenha
acesso após revogação. Instalações no modo local não precisam dessa migração.

## Preparação

O administrador configura em `auth.json`, junto aos campos externos existentes,
`external.identity_admin_scopes`: uma lista não vazia de scopes administrativos
emitidos pelo seu provedor. Todos são exigidos. Uma role chamada `admin`, sozinha,
não substitui esses scopes. O token também precisa satisfazer os scopes gerais
de `external.required_scopes`, se configurados.

Sem essa configuração, as rotas de cadastro respondem **503**. No modo local,
respondem **404**. Use a API HTTP já configurada; acesso fora de loopback exige
HTTPS. Não copie tokens para logs, mensagens de suporte ou histórico de comandos.

## Primeiro vínculo

Envie `POST /auth/external/identities/bootstrap`, com `Authorization: Bearer`
seguido do token administrativo e corpo JSON `{}`.

O emissor precisa ser exatamente o configurado. O `sub` do próprio token
precisa coincidir com o ID de um usuário local ativo. Não é possível escolher
outra pessoa no corpo da solicitação. Se essa correspondência não existir,
o cadastro é recusado: não há criação automática nem uso do usuário atual.

O bootstrap só pode ser concluído uma vez por emissor. Repeti-lo responde
**409**; revogar um vínculo não reabre o bootstrap. Se o administrador perder
seu acesso, não apague auditoria para tentar reiniciar esse procedimento.

## Demais vínculos

Um administrador já vinculado e ativo, ainda com todos os scopes exigidos,
pode enviar `POST /auth/external/identities` com os campos:

- `issuer`: emissor exato, igual ao configurado;
- `subject`: identificador exato no provedor, sensível a maiúsculas;
- `userId`: ID do usuário local existente e ativo escolhido pelo administrador.

Sucesso responde **201**. Vínculo já existente responde **409**, sem substituição
silenciosa; autenticação/autorização ou alvo inválido são recusados sem divulgar
detalhes das contas. Campos desconhecidos e corpos inválidos são rejeitados.

O vínculo e sua auditoria são gravados na mesma transação: falha em qualquer
parte desfaz ambas. A auditoria contém ator, alvo, ação e horário, nunca o JWT.
O cadastro não expõe exclusão de auditoria, remapeamento automático ou uma
opção para forçar a ativação do executor.

Na base interna de comandos, cada token tem identidade própria, derivada de
seu fingerprint, sem armazenar o JWT. Revogar um vínculo invalida os contextos
de todos os seus tokens e cancela suas esperas; reativá-lo não recupera
execuções antigas. Outros vínculos do mesmo usuário não são revogados juntos.
Essa proteção ainda não disponibiliza revogação por uma nova rota HTTP nem
habilita comandos externos: a montagem do ingresso no executor continua pendente.
