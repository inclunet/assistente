package skills

import (
	"context"
	"fmt"
	"io/fs"
	"log"
)

// SeedResult resume o resultado de um seed de builtins.
type SeedResult struct {
	Seeded  int
	Skipped int
	Failed  int
}

// SeedBuiltinSkills lê skills embeddados de fsys sob baseDir (layout
// `{slug}/SKILL.md`) e aplica repo.SeedBuiltin para cada um, com versionamento
// (AEP-0051 D5). fsys é um fs.FS (testável via fstest.MapFS; em produção é o
// embed.FS do binário). Erros por skill não abortam o lote.
func SeedBuiltinSkills(ctx context.Context, repo Repository, fsys fs.FS, baseDir string) (SeedResult, error) {
	var res SeedResult

	entries, err := fs.ReadDir(fsys, baseDir)
	if err != nil {
		return res, fmt.Errorf("read builtin skills dir %q: %w", baseDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		data, err := fs.ReadFile(fsys, baseDir+"/"+slug+"/"+skillFile)
		if err != nil {
			continue // diretório sem SKILL.md — ignora
		}

		meta, content, err := Parse(string(data))
		if err != nil {
			log.Printf("[Skills/Seed] parse %s: %v", slug, err)
			res.Failed++
			continue
		}
		if meta.Name == "" {
			meta.Name = slug
		}

		skill := &Skill{
			SkillMetadata: *meta,
			Slug:          slug,
			Content:       content,
		}
		if err := repo.SeedBuiltin(ctx, skill, meta.Version); err != nil {
			log.Printf("[Skills/Seed] seed %s: %v", slug, err)
			res.Failed++
			continue
		}
		res.Seeded++
	}

	return res, nil
}
