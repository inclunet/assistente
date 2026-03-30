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
  TestTool,
} from '@wailsjs/go/main/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { jobs } from '@wailsjs/go/models';

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
          const updated = Object.assign(Object.create(Object.getPrototypeOf(j)), j);
          updated.enabled = data.enabled;
          return updated as jobs.JobInfo;
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
            const updated = Object.assign(Object.create(Object.getPrototypeOf(j)), j);
            updated.enabled = enabled;
            return updated as jobs.JobInfo;
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
        return await GetToolCatalog() || [];
      } catch (err) {
        set({ error: String(err) });
        return [];
      }
    },

    testTool: async (toolName: string, inputs: Record<string, unknown>, eventData?: Record<string, unknown>) => {
      try {
        return await TestTool(toolName, JSON.stringify(inputs), eventData ? JSON.stringify(eventData) : '');
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
