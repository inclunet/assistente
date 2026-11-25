package plugins

import (
	"encoding/json"
	"slices"
	"testing"

	"assistente/internal/jobs"
	"assistente/internal/pluginpackage"
)

func TestPortableJobPreviewListsRequestedToolAndTriggers(t *testing.T) {

	const file = nativeExtension + "/jobs/reminder.yaml"
	const valid = `{"id":"reminder","max_runs_per_hour":4,"name":"Reminder","tool":"notify","inputs":{"text":"Review"},"triggers":[{"type":"manual"},{"type":"interval","every":"2h"},{"type":"cron","expression":"TZ=UTC 0 9 * * *"}]}`
	snap := &pluginpackage.LocalSnapshot{Manifest: pluginpackage.Manifest{Extensions: map[string]json.RawMessage{nativeExtension: json.RawMessage(`{"version":2,"jobs":["reminder"]}`)}}, Files: map[string][]byte{file: []byte(valid)}}
	cs := DiscoverComponents(snap)
	if len(cs) != 1 || cs[0].Name != "Reminder" || cs[0].Tool != "notify" || cs[0].Reason != "" || cs[0].JobMaxRunsPerHour != 4 || !slices.Equal(cs[0].JobTriggers, []jobs.Trigger{{Type: jobs.TriggerManual}, {Type: jobs.TriggerInterval, Every: "2h"}, {Type: jobs.TriggerCron, Expression: "TZ=UTC 0 9 * * *"}}) {
		t.Fatal(cs)
	}
	snap.Manifest.Extensions[nativeExtension] = json.RawMessage(`{"version":1,"jobs":["reminder"]}`)
	if rejected := DiscoverComponents(snap); len(rejected) != 1 || rejected[0].Reason != "manifestConfiguration" {
		t.Fatal(rejected)
	}
	snap.Manifest.Extensions[nativeExtension] = json.RawMessage(`{"version":2,"jobs":["reminder"]}`)
	delete(snap.Files, file)
	snap.Files[nativeExtension+"/jobs/reminder.json"] = []byte(valid)
	if rejected := DiscoverComponents(snap); len(rejected) != 1 || rejected[0].Reason != "invalidJob" {
		t.Fatal(rejected)
	}
	job, err := jobs.ParsePackageDefinition([]byte(valid))
	if err != nil || job.Enabled || job.LastRun != nil || job.Inputs["text"] != "Review" {
		t.Fatal(job, err)
	}
	snap.Files[file] = []byte("id: slug-do-job\ntool: notify\ntriggers: [{type: manual}]")
	if preview := DiscoverComponents(snap); len(preview) != 1 || preview[0].Name != "slug-do-job" {
		t.Fatal(preview)
	}
	for _, raw := range []string{
		`{"id":"task","name":"Task","tool":"notify","triggers":[{"type":"cron","expression":"TZ=UTC"}]}`,
		`{"id":"task","name":"Task","tool":"notify","triggers":[{"type":"cron","expression":"CRON_TZ=UTC"}]}`,
		`{"id":"task","name":"Task","tool":"run_command","requiredInputs":["text"],"triggers":[{"type":"manual"}]}`,
		`{"id":"task","name":"Task","tool":"run_command","triggers":[{"type":"hotkey","keys":""}]}`,
		`{"id":"task","name":"Task","tool":"run_command","triggers":[{"type":"interval","every":"oops"}]}`,
		`{"id":"task","name":"Task","tool":"notify","triggers":[{"type":"cron","expression":"bad"}]}`,
		`{"id":"task","name":"Task","tool":"notify","triggers":[]}`,
		`{"id":"task","tool":"notify","max_runs_per_hour":-1,"triggers":[{"type":"manual"}]}`,
		`{"id":"task","name":"Task","tool":"notify","triggers":[{"type":"manual"}]} {}`,
		"",
	} {
		snap.Files[file] = []byte(raw)
		cs = DiscoverComponents(snap)
		if len(cs) != 1 || cs[0].Reason != "invalidJob" || cs[0].Tool != "" {
			t.Errorf("incorrect preview for %s: %+v", raw, cs)
		}
	}
}
