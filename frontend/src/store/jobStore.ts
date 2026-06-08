import { create } from 'zustand';
import {
  GetJobs,
  GetJob,
  ToggleJob,
  RunJob,
  DryRunJob,
  GetJobRuns,
  GetJobEvents,
  GetJobPipelines,
  GetToolCatalog,
  SaveJob,
  DeleteJob,
  TestToolDryRun,
} from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { jobs } from '@wailsjs/go/models';
import { parseToolSource } from '../utils/toolSource';

function applyEffectiveEnabled(job: jobs.JobInfo, enabled: boolean): jobs.JobInfo {
  const updated = Object.assign(Object.create(Object.getPrototypeOf(job)), job);
  updated.enabled = enabled;
  updated.effective_enabled = enabled && updated.pipeline_enabled !== false;
  return updated as jobs.JobInfo;
}

function rawMessageToString(raw: unknown): string | undefined {
  if (raw == null) return undefined;
  if (typeof raw === 'string') return raw;

  // Wails mapeia Go json.RawMessage como number[] (bytes)
  if (Array.isArray(raw) && raw.every((v) => typeof v === 'number')) {
    if (typeof TextDecoder !== 'undefined') {
      return new TextDecoder().decode(Uint8Array.from(raw));
    }
    return String.fromCharCode(...raw);
  }

  if (raw instanceof Uint8Array) {
    if (typeof TextDecoder !== 'undefined') {
      return new TextDecoder().decode(raw);
    }
    return String.fromCharCode(...Array.from(raw));
  }

  // Fallback: preserva a forma serializável
  return JSON.stringify(raw);
}

function normalizeCatalogEntry(entry: jobs.CatalogEntry): jobs.CatalogEntry {
  const normalized = Object.assign(Object.create(Object.getPrototypeOf(entry)), entry) as jobs.CatalogEntry;
  const schema = rawMessageToString((entry as unknown as { schema?: unknown }).schema);
  if (schema !== undefined) {
    (normalized as unknown as { schema?: unknown }).schema = schema;
  }
  return normalized;
}

function isMCPBridgeToolName(toolName: string): boolean {
  const name = toolName.trim();
  const source = parseToolSource(name);
  return source.type === 'mcp' && Boolean(source.serverSlug);
}

interface JobStoreState {
  jobs: jobs.JobInfo[];
  isLoading: boolean;
  error: string | null;
  selectedJobId: string | null;
  jobDetail: jobs.Job | null;
  runLogs: jobs.RunLog[];
  events: jobs.EventEntry[];
  pipelines: jobs.PipelineInfo[];

  fetchJobs: () => Promise<void>;
  fetchJobDetail: (id: string) => Promise<void>;
  toggleJob: (id: string, enabled: boolean) => Promise<void>;
  runJob: (id: string) => Promise<jobs.RunLog | null>;
  dryRunJob: (id: string) => Promise<jobs.DryRunResult | null>;
  fetchJobRuns: (id: string, limit?: number) => Promise<void>;
  fetchJobEvents: (date: string) => Promise<void>;
  fetchPipelines: () => Promise<void>;
  fetchToolCatalog: () => Promise<jobs.CatalogEntry[]>;
  testTool: (toolName: string, inputs: Record<string, unknown>, eventData?: Record<string, unknown>) => Promise<jobs.TestToolResult | null>;
  saveJob: (jobJSON: string) => Promise<void>;
  deleteJob: (id: string) => Promise<void>;
  setSelectedJobId: (id: string | null) => void;
  clearError: () => void;
}

export const useJobStore = create<JobStoreState>((set, get) => {
  // Listeners de eventos do backend (Wails EventsEmit)
  if (typeof window !== 'undefined' && (window as unknown as Record<string, unknown>).runtime) {
    EventsOn('jobs:updated', () => {
      get().fetchJobs();
    });

    EventsOn('jobs:removed', () => {
      get().fetchJobs();
    });

    EventsOn('jobs:toggled', (data: { id: string; enabled: boolean }) => {
      set((state) => ({
        jobs: state.jobs.map((j) => {
          if (j.id !== data.id) return j;
          return applyEffectiveEnabled(j, data.enabled);
        }),
      }));
    });

    EventsOn('jobs:run_start', (data: { job_id: string }) => {
      set((state) => ({
        jobs: state.jobs.map((j) => {
          if (j.id !== data.job_id) return j;
          const updated = Object.assign(Object.create(Object.getPrototypeOf(j)), j);
          updated.status = 'running';
          return updated as jobs.JobInfo;
        }),
      }));
    });

    EventsOn('jobs:run_end', (data: { job_id: string; status: string }) => {
      set((state) => ({
        jobs: state.jobs.map((j) => {
          if (j.id !== data.job_id) return j;
          const updated = Object.assign(Object.create(Object.getPrototypeOf(j)), j);
          updated.status = data.status === 'failed' ? 'error' : 'idle';
          return updated as jobs.JobInfo;
        }),
      }));

      // Se estamos vendo detalhes desse job, atualiza runs
      const { selectedJobId } = get();
      if (selectedJobId === data.job_id) {
        get().fetchJobRuns(data.job_id);
      }
    });
  }

  return {
    jobs: [],
    isLoading: false,
    error: null,
    selectedJobId: null,
    jobDetail: null,
    runLogs: [],
    events: [],
    pipelines: [],

    fetchJobs: async () => {
      set({ isLoading: true, error: null });
      try {
        const result = await GetJobs();
        set({ jobs: result || [], isLoading: false });
      } catch (err) {
        set({ error: String(err), isLoading: false });
      }
    },

    fetchJobDetail: async (id: string) => {
      try {
        const result = await GetJob(id);
        set({ jobDetail: result, selectedJobId: id });
      } catch (err) {
        set({ error: String(err) });
      }
    },

    toggleJob: async (id: string, enabled: boolean) => {
      try {
        await ToggleJob(id, enabled);
        set((state) => ({
          jobs: state.jobs.map((j) => {
            if (j.id !== id) return j;
            return applyEffectiveEnabled(j, enabled);
          }),
        }));
      } catch (err) {
        set({ error: String(err) });
        throw err;
      }
    },

    runJob: async (id: string) => {
      try {
        const result = await RunJob(id);
        // Refresh jobs list to get updated status
        get().fetchJobs();
        return result;
      } catch (err) {
        set({ error: String(err) });
        return null;
      }
    },

    dryRunJob: async (id: string) => {
      try {
        return await DryRunJob(id);
      } catch (err) {
        set({ error: String(err) });
        return null;
      }
    },

    fetchJobRuns: async (id: string, limit = 20) => {
      try {
        const result = await GetJobRuns(id, limit);
        set({ runLogs: result || [] });
      } catch (err) {
        set({ error: String(err) });
      }
    },

    fetchJobEvents: async (date: string) => {
      try {
        const result = await GetJobEvents(date);
        set({ events: result || [] });
      } catch (err) {
        set({ error: String(err) });
      }
    },

    fetchPipelines: async () => {
      try {
        const result = await GetJobPipelines();
        set({ pipelines: result || [] });
      } catch (err) {
        set({ error: String(err) });
      }
    },

    fetchToolCatalog: async () => {
      try {
        const result = await GetToolCatalog();
        return (result || []).map(normalizeCatalogEntry);
      } catch (err) {
        set({ error: String(err) });
        return [];
      }
    },

    testTool: async (toolName: string, inputs: Record<string, unknown>, eventData?: Record<string, unknown>) => {
      try {
        const name = toolName.trim();
        const request: Record<string, unknown> = {
          tool_name: name,
          inputs,
          event_data: eventData,
        };
        if (isMCPBridgeToolName(name)) {
          const catalog = await get().fetchToolCatalog();
          const entry = catalog.find((item) => item.name === name);
          if (entry) {
            request.mcp_server_id = entry.mcp_server_id;
            request.tool_catalog_id = entry.id;
            request.origin = entry.origin;
            request.risk = entry.risk;
          }
        }
        return await TestToolDryRun(
          JSON.stringify(request),
        );
      } catch (err) {
        set({ error: String(err) });
        return null;
      }
    },

    saveJob: async (jobJSON: string) => {
      try {
        await SaveJob(jobJSON);
        get().fetchJobs();
      } catch (err) {
        set({ error: String(err) });
        throw err;
      }
    },

    deleteJob: async (id: string) => {
      try {
        await DeleteJob(id);
        get().fetchJobs();
      } catch (err) {
        set({ error: String(err) });
        throw err;
      }
    },

    setSelectedJobId: (id: string | null) => set({ selectedJobId: id }),

    clearError: () => set({ error: null }),
  };
});
