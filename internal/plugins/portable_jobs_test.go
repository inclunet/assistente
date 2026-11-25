package plugins

import (
	"encoding/json"
	"slices"
	"testing"

	"assistente/internal/jobs"
	"assistente/internal/pluginpackage"
)

func TestPortableJobPreviewListsRequestedToolAndTriggers(t *testing.T) {
	const key = "reminder"
	const file = nativeExtension + "/jobs/reminder.json"
	const valid = `{"maxRunsPerHour":4,"name":"Reminder","tool":"notify","inputs":{"text":"Review"},"triggers":[{"type":"manual"},{"type":"interval","every":"2h"},{"type":"cron","expression":"TZ=UTC 0 9 * * *"}]}`
	snap := &pluginpackage.LocalSnapshot{Manifest: pluginpackage.Manifest{Extensions: map[string]json.RawMessage{nativeExtension: json.RawMessage(`{"version":1,"jobs":["reminder"]}`)}}, Files: map[string][]byte{file: []byte(valid)}}
	cs := DiscoverComponents(snap)
	if len(cs) != 1 || cs[0].Name != "Reminder" || cs[0].Tool != "notify" || cs[0].Reason != "" || cs[0].JobMaxRunsPerHour != 4 || !slices.Equal(cs[0].JobTriggers, []jobs.Trigger{{Type: jobs.TriggerManual}, {Type: jobs.TriggerInterval, Every: "2h"}, {Type: jobs.TriggerCron, Expression: "TZ=UTC 0 9 * * *"}}) {
		t.Fatal(cs)
	}
	job, err := jobs.ReadPortableJob(key, []byte(valid))
	if err != nil || job.Enabled || job.LastRun != nil || job.Inputs["text"] != "Review" {
		t.Fatal(job, err)
	}
	for _, raw := range []string{
		`{"name":"Task","tool":"notify","triggers":[{"type":"cron","expression":"TZ=UTC"}]}`,
		`{"name":"Task","tool":"notify","triggers":[{"type":"cron","expression":"CRON_TZ=UTC"}]}`,
		`{"name":"Task","tool":"run_command","enabled":true,"triggers":[{"type":"manual"}]}`,
		`{"name":"Task","tool":"run_command","triggers":[{"type":"hotkey","keys":"x"}]}`,
		`{"name":"Task","tool":"run_command","triggers":[{"type":"manual","every":"3h"}]}`,
		`{"name":"Task","tool":"notify","triggers":[{"type":"cron","expression":"bad"}]}`,
		`{"name":"Task","tool":"notify","triggers":[]}`,
		`{"name":"Task","tool":"notify","triggers":[{"type":"manual"}]} {}`,
		"",
	} {
		snap.Files[file] = []byte(raw)
		cs = DiscoverComponents(snap)
		if len(cs) != 1 || cs[0].Reason != "invalidJob" || cs[0].Tool != "" {
			t.Errorf("incorrect preview for %s: %+v", raw, cs)
		}
	}
}
