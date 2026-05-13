package jobs

import (
	"fmt"
	"sort"
	"sync"
)

// Registry armazena jobs carregados em memoria com acesso thread-safe.
type Registry struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

// NewRegistry cria um registry vazio.
func NewRegistry() *Registry {
	return &Registry{
		jobs: make(map[string]*Job),
	}
}

// Add registra um job. Retorna erro se o ID ja existir.
func (r *Registry) Add(job *Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.jobs[job.ID]; exists {
		return fmt.Errorf("job already registered: %s", job.ID)
	}

	r.jobs[job.ID] = job
	return nil
}

// Set registra ou substitui um job (upsert).
func (r *Registry) Set(job *Job) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs[job.ID] = job
}

// Remove desregistra um job pelo ID. Retorna false se nao existia.
func (r *Registry) Remove(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, exists := r.jobs[id]
	if exists {
		delete(r.jobs, id)
	}
	return exists
}

// Clear remove todos os jobs do registry.
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs = make(map[string]*Job)
}

// Get retorna um job pelo ID ou nil se nao existir.
func (r *Registry) Get(id string) *Job {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.jobs[id]
}

// Has verifica se um job existe.
func (r *Registry) Has(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.jobs[id]
	return ok
}

// GetAll retorna todos os jobs ordenados por ID.
func (r *Registry) GetAll() []*Job {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Job, 0, len(r.jobs))
	for _, job := range r.jobs {
		result = append(result, job)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}

// GetEnabled retorna apenas jobs habilitados, ordenados por ID.
func (r *Registry) GetEnabled() []*Job {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Job
	for _, job := range r.jobs {
		if job.Enabled {
			result = append(result, job)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}

// GetByPipeline retorna jobs agrupados por pipeline.
func (r *Registry) GetByPipeline() map[string][]*Job {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pipelines := make(map[string][]*Job)
	for _, job := range r.jobs {
		key := job.Pipeline
		if key == "" {
			key = "(ungrouped)"
		}
		pipelines[key] = append(pipelines[key], job)
	}

	for k := range pipelines {
		sort.Slice(pipelines[k], func(i, j int) bool {
			return pipelines[k][i].ID < pipelines[k][j].ID
		})
	}

	return pipelines
}

// GetByEvent retorna jobs que escutam um determinado evento.
func (r *Registry) GetByEvent(eventName string) []*Job {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Job
	for _, job := range r.jobs {
		if !job.Enabled {
			continue
		}
		for _, t := range job.Triggers {
			if t.Type == TriggerEvent && t.Listen == eventName {
				result = append(result, job)
				break
			}
		}
	}

	return result
}

// Count retorna o numero total de jobs registrados.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.jobs)
}

// IDs retorna os IDs de todos os jobs registrados, ordenados.
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.jobs))
	for id := range r.jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
