# AEP-0103 — Contratos versionados de infraestrutura

Complemento operacional de D2/D2.1/D6, sem habilitar adapters ou handlers de produto.

## Compatibilidade

- Versões de documento são inteiros explícitos; versão desconhecida é recusada, não reinterpretada como a atual.
- O schema tipado do catálogo é um subconjunto fechado: objeto, array, string, número, inteiro, booleano e null, com limites e enum. Propriedade opcional e valor nullable são condições diferentes. Objetos rejeitam propriedades desconhecidas. Recursos fora do subconjunto exigem evolução explícita do contrato, não passagem sem validação.
- JSON usado em assinatura segue RFC 8785: ordenação de propriedades por UTF-16, números IEEE-754/ECMAScript, sem normalização Unicode. Entrada inválida, chaves duplicadas (também aninhadas), surrogate isolado, profundidade ou tamanho excessivo são recusados antes da assinatura.
- IDs e valores inteiros que precisam de precisão além de IEEE-754 devem ser representados como strings no schema. A canonicalização numérica não promete preservar inteiros arbitrários.
- Overflow e números não zero que sofreriam underflow para zero são explicitamente fora do conjunto suportado. IDs de workspace existentes continuam opacos (`ws-…`); não são PKs criadas pela AEP e não são renomeados para UUID.
- Documentos persistidos passam pela mesma validação lexical do ingresso. A validação do schema preserva requisitos próprios, como a representação inteira de `version`; canonicalizar para assinar não afrouxa esses requisitos.
- O signer legado `commandledger.SignLocalRead` mantém sua projeção e seus fingerprints, para não reinterpretar registros já criados. O envelope ampliado tem projeção/domínio próprios; um registro legado não é promovido ao contrato ampliado por fallback.
- Versão de chave vem do bootstrap ou de registro consultado sob ownership autenticado. Versão de documento não é versão de chave. Rotação não torna uma chave antiga dispensável enquanto seus registros existirem.
- HMACs de argumentos e request têm domínios separados. Timestamps de recebimento/captura e a decisão resultante não mudam a identidade semântica; os campos de ocorrência/deadline de replay de evento seguem a inclusão explícita de D2.1.

## Defaults

O construtor semântico consome o catálogo completo e inclui comando, argumentos, especificação/identidade de trigger, condição, efeito, escopo, prioridades, invariantes e requisitos do adapter. A apresentação localizada é excluída. Versão de publicação pode avançar sem mudança de fingerprint; identidade de conteúdo builtin público usa SHA-256 sobre JCS com domínio próprio. Argumentos de usuário não usam esse hash público: passam pelo HMAC de invocação.

O resolvedor continua recebendo uma projeção indexada. O fingerprint é construído antes da publicação, não recalculado ao pressionar cada tecla. Contrato completo não é autorização: disponibilidade, ownership, política, contexto e decisão continuam sendo revalidados pelo executor.

## Evidência e limites

Os arquivos de teste dos pacotes `commandjson`, `commandcatalog`, `commandcontract` e `commandconfig` são o corpus executável. O registro de execução e de aceite de cada pacote permanece em `0103-tasklist-infraestrutura.md`.

Importação/exportação de configurações, adapters físicos e migração dos comandos existentes continuam nos pacotes próprios. Este documento não declara integração desses caminhos nem qualificação de release.
