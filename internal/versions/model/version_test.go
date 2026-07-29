package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVersionResponseJSONShape(t *testing.T) {
	t.Parallel()

	response := VersionResponse{
		Products: []ProductVersion{
			{
				Key: "sample",
				Metadata: ProductMetadata{
					DisplayName:     "Sample",
					ApplicationType: "generic",
				},
				Sources: VersionSources{
					CMDB: NewOKCMDBResult("1.0.0", nil),
					Runtime: RuntimeSourceResult{
						Deployments: []RuntimeDeploymentResult{
							NewOKRuntimeResult("qa", "http", "1.0.0"),
							NewOKRuntimeResult("prod", "mimir", "1.0.1"),
						},
					},
					EOL: EOLResult{
						Status:  SourceStatusOK,
						Product: "sample",
						Cycles: []EOLCycle{
							{Cycle: "1.0"},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	body := string(data)

	for _, expected := range []string{
		`"key":"sample"`,
		`"metadata":{"display_name":"Sample","application_type":"generic"}`,
		`"runtime":{"deployments":[`,
		`"env":"qa"`,
		`"env":"prod"`,
		`"eol":{"status":"ok","product":"sample"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response JSON does not contain %q: %s", expected, body)
		}
	}

	if strings.Contains(body, `"environment"`) {
		t.Fatalf("response JSON still contains legacy environment field: %s", body)
	}
}
