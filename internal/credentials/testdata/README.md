# Fixtures OAuth legadas

`oauth-legacy.sql` acrescenta dados sintéticos cifrados às fixtures publicadas de 0.2.0, 0.3.0, 0.4.0 e 0.5.0 em `internal/database/testdata/published/`. A proveniência dos schemas completos está no README daquele diretório. Os dados cobrem as duas tabelas
envolvidas na recuperação. Não é backup de usuário nem dump produzido pelo
executável publicado. Os valores seguem o mapeamento persistido confirmado nos modelos
da release **0.9.0**, commit `e5dd78f975f304f8e9a8b7c330b98252735c7cc7`:

- `internal/database/models.go`: `UUIDModel` e `CredentialEntry`;
- `internal/database/models_mcp.go`: `MCPServer`;
- `internal/credentials/db_store.go`: mapeamento dos campos persistidos;
- `internal/mcp/oauth.go`: `persistClientCreds` e `persistTokens`;
- `internal/credentials/manager.go`: envelope AES-GCM.

Não houve alteração em `models_mcp.go` ou `db_store.go` entre 0.5.0 e 0.9.0. Os testes importam os schemas publicados existentes e os dados OAuth antes de aplicar duas vezes o AutoMigrate das duas tabelas atuais. Não executam os aplicativos históricos nem substituem os testes de upgrade completo do pacote database.

Todos os valores são fictícios. A chave pública de teste contém os bytes 1 a 32;
os textos `fixture-client`, `fixture-secret`, `fixture-access`, `fixture-refresh`
e `fixture-host-token` foram cifrados uma vez com AES-256-GCM, nonce aleatório de
12 bytes, tag de 16 bytes e sem AAD. O formato é base64(nonce + ciphertext + tag),
como na release. Os testes não regeneram esses ciphertexts nem usam o serializer
atual para construir os registros originais. Nunca usar essa chave em produção.

Os casos derivados simulam cadastro parcial, Client Credentials, segredo
ilegível e outro usuário. Verificam preservação no upgrade/captura, recuperação
sem replay de tokens, recarga, callback fixo e ausência de alteração no hostname
ou no outro usuário. Ainda não comprovam conversão para source OAuth, rollback
de versões executáveis nem recuperação do hostname compartilhado.

