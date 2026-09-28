---
title: "Fontes de credenciais"
---

# Fontes de credenciais

Em **Credenciais → Nova**, escolha a fonte e depois o tipo de autenticação.
O tipo define como o segredo é aplicado: Bearer, Basic ou header customizado.
O tipo Segredo atende consumidores internos, como canais.

- **Valor salvo:** informe o segredo no campo oculto.
- **Variável de ambiente:** escolha ou digite o nome da variável, sem prefixo.
  O app lê o ambiente herdado no início do processo; reinicie após mudanças externas.
- **Keyring:** escolha um target do Credential Manager do Windows, ou preencha
  serviço e usuário do keyring do sistema. Use apenas uma das alternativas.
  Listagem de targets depende da plataforma; entrada manual continua disponível.
- **Comando:** informe executável, argumentos como array JSON de strings e timeout
  entre 1 e 300 segundos. O padrão é 30 segundos. Não há shell implícito:
  pipes, redirecionamentos e expansões não são interpretados.
- **OAuth:** reservado para uma evolução futura; ainda não pode ser salvo como
  fonte. Isso não altera o OAuth dos servidores MCP.

Exemplo local: executável `nu`, argumentos `["genai", "ai-gateway", "token"]`.
No Windows via WSL: executável `wsl.exe`, argumentos
`["--", "nu", "genai", "ai-gateway", "token"]`.

O comando precisa estar instalado e autenticado, retornar um token em stdout
(em uma linha, até 64 KiB) e terminar com sucesso. Espaços externos são removidos.
O Assistente não exibe stdout nem stderr em erros. A execução ocorre com as
permissões do app; evite segredos literais nos argumentos. Timeout cancela o
processo direto, sem promessa de encerrar todos os descendentes ou processos WSL.
Para **Comando**, o resultado é reutilizado em memória, cifrado, separado por
credencial e usuário. Não há TTL presumido, leitura de JWT ou renovação periódica:
a saída é tratada como token opaco. Chamadas simultâneas aguardam a mesma execução
bem-sucedida; cancelar uma espera não cancela a execução de outra chamada.

Nas requisições HTTP que usam o transport de credenciais (incluindo provedores
LLM), uma resposta **401** descarta o token rejeitado. Se o corpo puder ser
recriado, o Assistente obtém um token novo e repete a requisição **no máximo uma
vez**, apenas se o valor mudou. Uploads sem corpo repetível não são reenviados;
a próxima chamada obtém um novo token. Erros 400, 403, de rede ou de banco não
renovam credenciais. Se o servidor indicar expiração de outra forma, salve
novamente a configuração da credencial para limpar o cache.

Salvar/excluir a credencial, recarregar sua configuração, mudar/encerrar a sessão
ou fechar o app descarta o cache. O token materializado não vai para o banco nem
para a tela de edição. Env e keyring continuam sendo consultados a cada uso.
Consumidores que obtêm o segredo diretamente, sem esse transport HTTP (como
ferramentas HTTP e o SDK Gemini), continuam executando o comando a cada uso.
As sondagens de conexão e o monitor de saúde do provedor compartilham o cache
do transport; chaves digitadas apenas para teste continuam isoladas.

Em Basic, o usuário fica na configuração e a fonte fornece a senha. Em header
customizado, informe o nome do header; a fonte fornece seu valor.

## Provedores LLM

Cadastre a credencial com o hostname da URL base como padrão (por exemplo,
`gateway.example.com`). Ao criar o provedor, marque **Usar credencial já
cadastrada para este domínio**. Na edição, mantenha a chave vazia para preservar
a fonte. Teste de conexão, modelos e execução usam a credencial resolvida.
Digitar uma nova chave e salvar substitui a fonte por **Valor salvo**.
Testar uma chave digitada não substitui nem remove a credencial salva.

## Reconfiguração após atualização

Credenciais antigas sem source precisam ser reconfiguradas manualmente.
Não existe migração automática. `env://NOME` e `keyring://entrada` não são
referências suportadas; escolha a fonte correspondente e informe seus campos.
Esses textos, se gravados em Valor salvo, são tratados literalmente.
