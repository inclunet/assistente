package portability

import (
	"context"
	"fmt"
	"strings"
)

// LegacyImportSource abstracts legacy read-only inputs. Implementations should
// list and read original files without renaming, deleting, or rewriting them.
type LegacyImportSource interface {
	ListLegacyImportFiles(ctx context.Context) ([]LegacyImportFile, error)
	ReadLegacyImportFile(ctx context.Context, filename string) ([]byte, error)
}

type LegacyImportFile struct {
	Name     string
	Filename string
	Path     string
	Source   string
}

type LegacyImportResult struct {
	ResourceType string
	Imported     int
	Skipped      int
	Failed       int
	Warnings     []string
	Errors       []string
}

type LegacyImportRequest[T any] struct {
	ResourceType string
	Source       LegacyImportSource
	FileSuffix   string
	Parse        func(LegacyImportFile, []byte) (T, error)
	Import       func(context.Context, T) (bool, error)
}

func ImportLegacyResourcesWithContext[T any](ctx context.Context, req LegacyImportRequest[T]) (LegacyImportResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := LegacyImportResult{
		ResourceType: req.ResourceType,
		Warnings:     make([]string, 0),
		Errors:       make([]string, 0),
	}
	if req.Source == nil {
		return result, fmt.Errorf("legacy import source is required")
	}
	if req.Parse == nil {
		return result, fmt.Errorf("legacy import parser is required")
	}
	if req.Import == nil {
		return result, fmt.Errorf("legacy import handler is required")
	}

	files, err := req.Source.ListLegacyImportFiles(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("não foi possível listar entradas legadas de %s: %v", req.ResourceType, err))
		return result, nil
	}
	for _, file := range files {
		if req.FileSuffix != "" && !strings.HasSuffix(file.Filename, req.FileSuffix) {
			continue
		}
		data, err := req.Source.ReadLegacyImportFile(ctx, file.Filename)
		if err != nil {
			result.Failed++
			msg := fmt.Sprintf("erro ao ler %s legado %s: %v", req.ResourceType, file.Filename, err)
			result.Errors = append(result.Errors, msg)
			continue
		}
		item, err := req.Parse(file, data)
		if err != nil {
			result.Failed++
			msg := fmt.Sprintf("erro ao parsear %s legado %s: %v", req.ResourceType, file.Filename, err)
			result.Errors = append(result.Errors, msg)
			continue
		}
		imported, err := req.Import(ctx, item)
		if err != nil {
			result.Failed++
			msg := fmt.Sprintf("erro ao importar %s legado %s: %v", req.ResourceType, file.Filename, err)
			result.Errors = append(result.Errors, msg)
			continue
		}
		if imported {
			result.Imported++
		} else {
			result.Skipped++
		}
	}
	return result, nil
}
