import { useState, useEffect } from 'react';
import { GetSkills, ReloadSkills } from '../../wailsjs/go/main/App';
import { skills } from '../../wailsjs/go/models';
import { Toolbar, ToolbarAction } from '../components/ui/Toolbar';
import './SkillsPage.css';

type Skill = skills.Skill;

export default function SkillsPage() {
  const [skillsList, setSkillsList] = useState<Skill[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadSkills();
  }, []);

  const loadSkills = async () => {
    try {
      setLoading(true);
      const data = await GetSkills();
      setSkillsList(data || []);
    } catch (error) {
      console.error('Erro ao carregar skills:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleReload = async () => {
    try {
      await ReloadSkills();
      await loadSkills();
    } catch (error) {
      console.error('Erro ao recarregar skills:', error);
    }
  };

  const actions: ToolbarAction[] = [
    {
      key: 'reload',
      label: 'Recarregar',
      icon: '🔄',
      onClick: handleReload,
    },
  ];

  return (
    <div className="skills-page">
      <Toolbar left={<span style={{ fontWeight: 600, fontSize: '1.1em' }}>Skills</span>} actions={actions} />
      <div className="page-content">
        {loading ? (
          <div className="skills-loading">Carregando skills...</div>
        ) : skillsList.length === 0 ? (
          <div className="skills-empty">
            <div className="skills-empty-icon">📚</div>
            <h3>Nenhuma skill instalada</h3>
            <p>
              Skills permitem ensinar novas habilidades ao assistente.
              Crie um diretorio em <code>~/.assistente/skills/</code> com um arquivo <code>SKILL.md</code>.
            </p>
          </div>
        ) : (
          <div className="skills-grid">
            {skillsList.map((skill) => (
              <div key={skill.name} className="skill-card">
                <div className="skill-card-header">
                  <span className="skill-card-name">{skill.display_name || skill.name}</span>
                  {skill.auto_load && (
                    <span className="skill-badge skill-badge-auto">auto-load</span>
                  )}
                </div>
                <p className="skill-card-description">
                  {skill.description || 'Sem descricao'}
                </p>
                {skill.tools && skill.tools.length > 0 && (
                  <div className="skill-card-tools">
                    {skill.tools.map((tool) => (
                      <span key={tool} className="skill-tool-tag">{tool}</span>
                    ))}
                  </div>
                )}
                <div className="skill-card-path">
                  <code>{skill.path}</code>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
