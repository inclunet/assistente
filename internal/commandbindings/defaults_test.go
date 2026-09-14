package commandbindings

import "testing"

func defaultFixture() (Default, Delta) {
	d := Default{Candidate: binding("default", Application, nil), Version: "1", Fingerprint: "semantic-v1"}
	x := Delta{ID: "override", DefaultID: "default", DefaultVersion: "1", DefaultFingerprint: "semantic-v1", Trigger: "Ctrl+M", Effect: Execute, CommandID: "custom", ArgumentsKey: "empty-args", Enabled: true, LayerActive: true, ReviewStatus: Active}
	return d, x
}

func TestMaterializacaoDeDeltas(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  func(*Default, *Delta)
		facts   Facts
		status  Status
		command string
	}{
		{"override antes da prioridade", func(d *Default, x *Delta) { d.Candidate.BindingPriority = 100 }, nil, Selected, "custom"},
		{"desabilitado restaura fallback", func(d *Default, x *Delta) { x.Enabled = false }, nil, Selected, "default"},
		{"camada inativa", func(d *Default, x *Delta) { x.LayerActive = false }, nil, Selected, "default"},
		{"fora do contexto", func(d *Default, x *Delta) { x.Condition = Facts{Profile: "dev"} }, nil, Selected, "default"},
		{"contexto restrito", func(d *Default, x *Delta) { x.Condition = Facts{Profile: "dev"} }, Facts{Profile: "dev"}, Selected, "custom"},
		{"supressao", func(d *Default, x *Delta) { x.Effect = Suppress; x.CommandID = ""; x.ArgumentsKey = "" }, nil, Suppressed, ""},
		{"upgrade semantico bloqueia", func(d *Default, x *Delta) { d.Fingerprint = "changed" }, nil, ReviewRequired, ""},
		{"pendencia desabilitada bloqueia", func(d *Default, x *Delta) { x.ReviewStatus = NeedsReview; x.Enabled = false }, nil, ReviewRequired, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, x := defaultFixture()
			tc.change(&d, &x)
			c, err := NewConfiguration([]Default{d}, []Delta{x}, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.Resolve("Ctrl+M", tc.facts, nil)
			if err != nil || got.Status != tc.status || got.CommandID != tc.command {
				t.Fatalf("%+v, %v", got, err)
			}
		})
	}
}

func TestSupressaoPreservaBindingEquivalenteERestauracao(t *testing.T) {
	d, x := defaultFixture()
	x.Effect = Suppress
	x.CommandID = ""
	x.ArgumentsKey = ""
	custom := binding("alternative", Application, nil)
	custom.CommandID = d.Candidate.CommandID
	c, err := NewConfiguration([]Default{d}, []Delta{x}, []Candidate{custom})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Resolve("Ctrl+M", nil, nil)
	if err != nil || got.Status != Selected || len(got.BindingIDs) != 1 || got.BindingIDs[0] != "alternative" {
		t.Fatalf("%+v %v", got, err)
	}
	restored, err := NewConfiguration([]Default{d}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err = restored.Resolve("Ctrl+M", nil, nil)
	if err != nil || got.CommandID != "default" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestUpgradeVersaoSemMutarEntrada(t *testing.T) {
	d, x := defaultFixture()
	d.Version = "2"
	x.Condition = Facts{Profile: "dev"}
	c, err := NewConfiguration([]Default{d}, []Delta{x}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if x.DefaultVersion != "1" {
		t.Fatal("entrada alterada")
	}
	a := c.Adjustments()
	if len(a) != 1 || a[0].Reason != "version_advanced" || a[0].DefaultVersion != "2" {
		t.Fatal(a)
	}
	a[0].Reason = "corrupted"
	x.Condition[Profile] = "changed"
	got, err := c.Resolve("Ctrl+M", Facts{Profile: "dev"}, nil)
	if err != nil || got.CommandID != "custom" || c.Adjustments()[0].Reason != "version_advanced" {
		t.Fatalf("snapshot alterado: %+v %v", got, err)
	}
}

func TestDeltaInvalido(t *testing.T) {
	for _, change := range []func(*Default, *Delta){
		func(d *Default, x *Delta) { d.Invariant = true },
		func(d *Default, x *Delta) { x.DefaultFingerprint = "" },
		func(d *Default, x *Delta) { x.Effect = Suppress },
		func(d *Default, x *Delta) { x.Trigger = "Ctrl+N" },
		func(d *Default, x *Delta) {
			d.Candidate.Condition = Facts{Profile: "a"}
			x.Condition = Facts{Profile: "b"}
		},
	} {
		d, x := defaultFixture()
		change(&d, &x)
		if _, err := NewConfiguration([]Default{d}, []Delta{x}, nil); err == nil {
			t.Fatal("delta inválido aceito")
		}
	}
}

func TestOrfaosNaoMisturamAcionadores(t *testing.T) {
	_, x := defaultFixture()
	y := x
	y.ID = "other"
	y.Trigger = "Ctrl+N"
	c, err := NewConfiguration(nil, []Delta{x, y}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Resolve("Ctrl+M", nil, nil)
	if err != nil || got.Status != ReviewRequired || len(got.BindingIDs) != 1 || got.BindingIDs[0] != x.ID {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestPendenciaPreservadaRespeitaContexto(t *testing.T) {
	d, x := defaultFixture()
	d.Candidate.Condition = Facts{SurfaceType: "chat"}
	x.Condition = Facts{Profile: "dev"}
	x.ReviewStatus = NeedsReview
	c, err := NewConfiguration([]Default{d}, []Delta{x}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Resolve("Ctrl+M", Facts{SurfaceType: "chat", Profile: "personal"}, nil)
	if err != nil || got.CommandID != "default" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestPendenciaNaoAceitaCondicaoContraditoria(t *testing.T) {
	d, x := defaultFixture()
	d.Candidate.Condition = Facts{Profile: "a"}
	x.Condition = Facts{Profile: "b"}
	x.ReviewStatus = NeedsReview
	if _, err := NewConfiguration([]Default{d}, []Delta{x}, nil); err == nil {
		t.Fatal("pendência contraditória aceita")
	}
}

func TestDialogoNaoBloqueadoPorPendenciaInferior(t *testing.T) {
	d, x := defaultFixture()
	x.ReviewStatus = NeedsReview
	top := binding("dialog.repeat", Dialog, nil)
	c, err := NewConfiguration([]Default{d}, []Delta{x}, []Candidate{top})
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Resolve("Ctrl+M", nil, &DialogScope{ID: top.DialogID, AllowedCommandIDs: []string{top.CommandID}, AllowedTriggers: []string{top.Trigger}})
	if err != nil || got.CommandID != top.CommandID {
		t.Fatalf("%+v %v", got, err)
	}
}
