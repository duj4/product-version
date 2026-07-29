package provider

import (
	"testing"

	"product-versions/internal/versions/model"
)

func TestAssessRuntimeUsesProductLifecycleCatalog(t *testing.T) {
	t.Parallel()

	eol := model.EOLResult{
		Status: model.SourceStatusOK,
		Cycles: []model.EOLCycle{
			{
				Cycle:        "10.3",
				Label:        "10.3 (LTS)",
				IsLTS:        true,
				IsMaintained: true,
				Latest:       "10.3.12",
				LatestDate:   "2026-06-10",
			},
		},
	}

	got := AssessRuntime(eol, "10.3.7", "major_minor", "10.3.8")

	if got.Status != model.SourceStatusOK {
		t.Fatalf("Status = %q, want ok", got.Status)
	}
	if got.CurrentCycle != "10.3" {
		t.Fatalf("CurrentCycle = %q, want 10.3", got.CurrentCycle)
	}
	if !got.IsLTS || !got.IsMaintained {
		t.Fatalf("assessment flags = %+v, want LTS and maintained", got)
	}
	if !got.CMDBMismatch {
		t.Fatal("CMDBMismatch = false, want true")
	}
	if !got.PatchAvailable {
		t.Fatal("PatchAvailable = false, want true")
	}
}

func TestAssessRuntimeDoesNotRequireEOLForCMDBMismatch(t *testing.T) {
	t.Parallel()

	got := AssessRuntime(
		model.NewDisabledEOLResult(),
		"2.0.0",
		"major_minor",
		"1.0.0",
	)

	if got.Status != model.SourceStatusDisabled {
		t.Fatalf("Status = %q, want disabled", got.Status)
	}
	if !got.CMDBMismatch {
		t.Fatal("CMDBMismatch = false, want true")
	}
}
