package jobs

import (
	"strings"
	"testing"
)

func TestPortableJobRejectsIncompatibleBehavior(t *testing.T) {
	for _, change := range []func(*Job){func(j *Job) { j.DryRun.Enabled = true }, func(j *Job) { j.Pipeline = "nightly" }, func(j *Job) { j.ErrorPolicy.Strategy = ErrorRetry }, func(j *Job) { j.Triggers[0].When = "false" }, func(j *Job) { j.Events.OnSuccess = "chain" }} {
		j := &Job{ID: "review", Name: "Review", Tool: "notify", Triggers: []Trigger{{Type: TriggerManual}}}
		change(j)
		if _, err := ExportPortableJob(j); err == nil {
			t.Fatal("comportamento incompatível exportado")
		}
	}
	source := &Job{ID: "review", Name: "Review", Tool: "notify", MaxRunsPerHour: 1, Triggers: []Trigger{{Type: TriggerInterval, Every: "5m"}}}
	raw, err := ExportPortableJob(source)
	if err != nil {
		t.Fatal(err)
	}
	job, err := ReadPortableJob("review", raw)
	if err != nil || job.MaxRunsPerHour != 1 || job.Triggers[0].Every != "5m" {
		t.Fatal(job, err)
	}
	conflict := strings.Replace(string(raw), "{", `{"requiredInputs":["text"],"inputs":{"text":"supplied"},`, 1)
	if _, err := ReadPortableJob("review", []byte(conflict)); err == nil {
		t.Fatal("pacote preencheu sua própria entrada local")
	}
}
