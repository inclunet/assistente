---
title: "Manutenção do catálogo de provedores e modelos"
weight: 65
description: "Como atualizar modelos, parâmetros e vozes publicados para o Assistente"
---

O catálogo é o arquivo público [assistente-catalog.json](/json/assistente-catalog.json),
validado pelo [JSON Schema](/json/assistente-catalog.schema.json). Ele é editado
neste repositório e publicado com o site. Não existe uma segunda lista de presets
para manter no aplicativo.

O aplicativo combina esses dados com a conexão escolhida e o ID exato listado
pelo servidor. Os limites e parâmetros alimentam tanto o editor quanto o envio.
O catálogo não concede acesso a modelos e não substitui configurações salvas nem
restrições aprendidas durante o uso.

## Quando atualizar

O mantenedor revisa o catálogo **mensalmente** e também quando houver lançamento,
descontinuação ou correção relevante de um provedor. A revisão mensal é um
procedimento editorial: não cria um job, sondagem paga ou atualização automática
dos dados.

Cada atualização é um PR pequeno. Prefira revisar um provedor ou família de
modelos por vez. Não é necessário esperar a revisão de todos os provedores para
publicar uma correção confirmada.

## O que conferir

- **Servidor:** URL base, formato da API, rotas relativas à base, autenticação,
  links de cadastro/chave e campos editáveis. O formato deve ser implementado pelo
  aplicativo; trocar uma string no JSON não cria um adaptador.
- **Modelo:** ID canônico estável, estado, modalidades, tools, limites de contexto
  e saída e contratos de parâmetros. Quantização, configuração local e rota
  podem alterar os limites efetivos; não aplique um limite universal sem suporte.
- **Exposição:** associação exata entre perfil, API e ID remoto. Proxies só recebem
  associações verificadas; aliases privados são configurados na própria conexão.
- **Parâmetros:** suporte, faixas, opções e default documentados. Ausência de valor
  conhecido fica ausente; zero só é publicado quando for um valor real aceito.
  Overrides substituem o contrato inteiro daquele campo na rota correspondente.
- **Vozes:** modelo e rota corretos, modo de seleção e lista completa, parcial ou
  desconhecida. Uma lista parcial não deve impedir entrada manual.

Use documentação oficial ou uma observação reproduzível da API para confirmar
os valores. Links e condições do teste cabem no corpo do PR; não acrescente uma
estrutura de procedência ao catálogo ou ao banco. Informação inconclusiva fica
como `unknown`, ou sem limite/default, em vez de ser inferida pelo nome.

Não remova IDs para reaproveitá-los com outro significado. Quando necessário,
marque registros como obsoletos com um estado permitido pelo schema e mantenha
referências válidas. Nunca inclua tokens, endpoints privados ou dados de contas.

## Editar e validar

1. Edite `docs/static/json/assistente-catalog.json`, preservando a formatação
   das partes que não mudaram.
2. Atualize `catalog_version` para `AAAA-MM-DD.N` e `updated_at` para o instante
   real da edição, em UTC. Incremente `N` quando houver outra publicação no dia.
   Confira a versão mais recente da `main` antes do merge. Não use data futura
   nem altere `schema_version` para uma atualização somente de dados.
3. Configure os diretórios de execução sob `build/bin/` conforme o
   [AGENTS.md](https://github.com/inclunet/assistente/blob/main/AGENTS.md).
   Na raiz da worktree, execute:

   ```text
   go test ./internal/llmcatalog -count=1
   ```

   O mesmo teste valida o asset contra o JSON Schema publicado e contra o
   decodificador/validador usado pelo aplicativo. A suíte também verifica
   referências, associações, parâmetros, vozes e atualização do cache. Ela
   não chama provedores nem executa testes ACP.
4. No PR, descreva os registros alterados, o comportamento esperado, os dados
   confirmados e os que continuam desconhecidos. Registre a validação executada.
   Alterações de schema ou contrato exigem atualizar juntos o consumidor, os
   testes e a AEP correspondente.

Sondagens potencialmente pagas são opcionais. Antes de realizá-las, delimite
provedor, modelos, número de chamadas e orçamento. Use credenciais fora do asset.
Não transforme seleção de modelo, abertura do editor ou CI em uma sondagem paga.

## Publicar e corrigir

Após o merge aprovado pelo mantenedor, o workflow **Deploy Docs** executa a
validação do catálogo antes de construir e publicar o Hugo. Se a validação
falhar, a publicação é interrompida e a versão anterior permanece no site.
O CI do PR também executa essa validação.

Confira o sucesso do deploy e abra o JSON publicado para verificar
`catalog_version` e `updated_at`. A publicação usa o mesmo arquivo versionado;
não há etapa de geração de uma segunda lista.

O Assistente atualiza seu snapshot em segundo plano e mantém o último catálogo
válido quando não consegue baixar ou validar o novo. Uma publicação não altera
credenciais, URLs ou escolhas já salvas nas conexões.

Se um dado publicado estiver errado, corrija-o em um novo PR e publique uma
**nova versão com timestamp atual**, mesmo ao restaurar valores anteriores.
Isso evita tratar uma versão antiga como atualização. Não desfaça silenciosamente
uma configuração do usuário para acompanhar o catálogo.
