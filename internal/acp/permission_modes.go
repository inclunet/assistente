package acp

import "strings"

// modesWithoutPermissionPrompt são os modos que sabidamente dispensam o
// `session/request_permission` — a única barreira que o app tem para autorizar
// o que o agente faz na máquina (AEP-0084 D9). Ligado um deles, o agente lê,
// edita e executa sem que nada seja perguntado.
//
// Isto é uma lista de valores conhecidos, e não uma classificação de modos. O
// app não presume o significado de um modo pelo nome: cada agente batiza os
// seus, e adivinhar o que um modo novo faz erraria nos dois sentidos — calaria
// sobre um que desliga a pergunta e alarmaria sobre um que não desliga. A
// exceção existe porque sem reconhecer valor nenhum não haveria como avisar
// que a barreira caiu, e esse silêncio é o pior dos desfechos.
//
// Por isso a lista é curta de propósito, e só entra nela o que se tem certeza:
//
//   - `bypassPermissions` (Claude Code): responde sozinho a todo pedido.
//   - `dontAsk` (Claude Code): idem.
//
// Fica de fora tudo de que não se tem certeza, inclusive o `acceptEdits` do
// mesmo agente: ele dispensa a pergunta só para edição e continua perguntando
// pelo resto, então o aviso diria que a barreira caiu inteira, o que seria
// falso. Errar para o lado do silêncio é o certo aqui — aviso que não
// corresponde ao que o agente faz ensina a ignorar os avisos.
var modesWithoutPermissionPrompt = map[string]bool{
	"bypasspermissions": true,
	"dontask":           true,
}

// ModeSkipsPermissionPrompt diz se este é um dos modos conhecidos por dispensar
// o pedido de permissão.
//
// Compara sem caixa e sem espaço nas pontas porque o valor vem do agente pelo
// fio: é o mesmo aparo que o resto deste pacote faz antes de comparar valor de
// opção, e sem ele um `DontAsk` passaria batido justamente onde o silêncio
// custa caro.
func ModeSkipsPermissionPrompt(mode string) bool {
	return modesWithoutPermissionPrompt[strings.ToLower(strings.TrimSpace(mode))]
}
