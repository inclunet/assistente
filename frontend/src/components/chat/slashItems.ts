import type { apidto, skills } from '../../../wailsjs/go/models';

/**
 * De onde veio o item do menu da barra. As duas origens convivem na mesma lista
 * porque quem digita "/" não pensa em duas listas: pensa no que pode pedir
 * agora. Mas elas não se confundem, porque respondem por coisas diferentes — a
 * skill é do app, o comando é do agente de código desta conversa (AEP-0084 D8).
 */
export type SlashItemSource = 'skill' | 'agent';

/** Um item do menu da barra, seja skill do app ou comando do agente. */
export interface SlashItem {
  /** Chave de render, única entre as duas origens. */
  key: string;
  source: SlashItemSource;
  /** O que fica escrito depois da barra ao escolher. */
  token: string;
  /** O nome lido na tela e pelo leitor de telas. */
  label: string;
  description?: string;
  /** Dica do que escrever depois do nome, quando o item aceita argumento. */
  argumentHint?: string;
  /** O item espera texto depois do nome. */
  acceptsInput: boolean;
  /** A skill original, para quem precisa invocá-la. */
  skill?: skills.SkillInfo;
}

/**
 * buildSlashItems junta as duas origens numa lista só. As skills vêm primeiro
 * por serem as de sempre: quem já usa o menu conta com elas no topo, e mudar a
 * ordem debaixo de quem navega por teclado mexeria no que a seta encontra.
 */
export function buildSlashItems(
  skillList: skills.SkillInfo[],
  agentCommands: apidto.AgentCommand[] = [],
): SlashItem[] {
  const items: SlashItem[] = skillList.map((skill) => ({
    key: `skill:${skill.slug}`,
    source: 'skill',
    token: skill.slug,
    label: skill.displayName || skill.name || skill.slug,
    description: skill.description,
    argumentHint: skill.argumentHint,
    acceptsInput: Boolean(skill.argumentHint),
    skill,
  }));

  // Comando do agente com o mesmo nome de uma skill não entra: os dois seriam
  // escritos igual no campo, e o app invocaria a skill. Dois itens indistintos
  // no menu fariam alguém escolher às cegas.
  const taken = new Set(items.map((item) => item.token));
  for (const command of agentCommands) {
    const token = (command.name || '').trim();
    if (!token || taken.has(token)) continue;
    taken.add(token);
    items.push({
      key: `agent:${token}`,
      source: 'agent',
      token,
      label: token,
      description: command.description,
      acceptsInput: Boolean(command.acceptsInput),
    });
  }
  return items;
}

/** filterSlashItems aplica o texto digitado depois da barra. */
export function filterSlashItems(items: SlashItem[], filter: string): SlashItem[] {
  const searchText = filter.trim().toLowerCase();
  if (!searchText) return items;
  return items.filter((item) => {
    const label = item.label.toLowerCase();
    const token = item.token.toLowerCase();
    const description = (item.description || '').toLowerCase();
    return (
      label.includes(searchText) ||
      token.includes(searchText) ||
      description.includes(searchText)
    );
  });
}
