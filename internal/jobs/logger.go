package jobs

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Logger persiste run logs (JSON) e event logs (JSONL).
type Logger struct {
	baseDir string // ~/.assistente/jobs/
	mu      sync.Mutex
}

// NewLogger cria um logger para o diretorio base de jobs.
func NewLogger(baseDir string) *Logger {
	return &Logger{baseDir: baseDir}
}

// LogRun grava o resultado de uma execucao em runs/<jobId>/<timestamp>.json
func (l *Logger) LogRun(rl *RunLog) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	dir := filepath.Join(l.baseDir, "runs", rl.JobID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create run log dir: %w", err)
	}

	ts := rl.StartedAt.Format("2006-01-02T15-04-05.000000000")
	filename := filepath.Join(dir, ts+".json")

	data, err := json.MarshalIndent(rl, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal run log: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("write run log: %w", err)
	}

	log.Printf("[Jobs] Run logged: %s/%s -> %s", rl.JobID, rl.RunID, rl.Status)
	return nil
}

// LogEvent grava uma entrada no event log do dia (append JSONL).
func (l *Logger) LogEvent(entry *EventEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	dir := filepath.Join(l.baseDir, "events")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create events dir: %w", err)
	}

	date := entry.Timestamp.Format("2006-01-02")
	filename := filepath.Join(dir, date+".jsonl")

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal event entry: %w", err)
	}

	f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open event log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write event entry: %w", err)
	}

	return nil
}

// GetRun retorna um run log especifico pelo jobID e runID.
func (l *Logger) GetRun(jobID, runID string) (*RunLog, error) {
	dir := filepath.Join(l.baseDir, "runs", jobID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("run not found: %s/%s", jobID, runID)
		}
		return nil, fmt.Errorf("read runs dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var rl RunLog
		if err := json.Unmarshal(data, &rl); err != nil {
			continue
		}
		if rl.RunID == runID {
			return &rl, nil
		}
	}

	return nil, fmt.Errorf("run not found: %s/%s", jobID, runID)
}

// GetRuns retorna os ultimos N run logs de um job, do mais recente ao mais antigo.
func (l *Logger) GetRuns(jobID string, limit int) ([]RunLog, error) {
	dir := filepath.Join(l.baseDir, "runs", jobID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read runs dir: %w", err)
	}

	// Filtra apenas .json e ordena por nome (descendente = mais recente primeiro)
	var jsonFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			jsonFiles = append(jsonFiles, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(jsonFiles)))

	if limit > 0 && len(jsonFiles) > limit {
		jsonFiles = jsonFiles[:limit]
	}

	var runs []RunLog
	for _, name := range jsonFiles {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var rl RunLog
		if err := json.Unmarshal(data, &rl); err != nil {
			continue
		}
		runs = append(runs, rl)
	}

	return runs, nil
}

// GetLastRun retorna o run log mais recente de um job, ou nil.
func (l *Logger) GetLastRun(jobID string) *RunLog {
	runs, err := l.GetRuns(jobID, 1)
	if err != nil || len(runs) == 0 {
		return nil
	}
	return &runs[0]
}

// GetEvents retorna as entradas do event log de uma data (formato "2006-01-02").
func (l *Logger) GetEvents(date string) ([]EventEntry, error) {
	filename := filepath.Join(l.baseDir, "events", date+".jsonl")

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read event log: %w", err)
	}

	var entries []EventEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry EventEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// GetEventsToday retorna as entradas do event log de hoje.
func (l *Logger) GetEventsToday() ([]EventEntry, error) {
	return l.GetEvents(time.Now().Format("2006-01-02"))
}

// CleanOldRuns remove run logs mais antigos que maxAge para um job.
func (l *Logger) CleanOldRuns(jobID string, maxAge time.Duration) (int, error) {
	dir := filepath.Join(l.baseDir, "runs", jobID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
				removed++
			}
		}
	}

	return removed, nil
}
