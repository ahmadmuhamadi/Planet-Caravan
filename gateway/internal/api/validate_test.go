package api

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

func decodeView(t *testing.T, body string) (*viewRequest, *apiError) {
	t.Helper()
	var req viewRequest
	return &req, decodeJSON(strings.NewReader(body), &req)
}

func TestNormalizeDefaults(t *testing.T) {
	req, e := decodeView(t, `{"target":{"ra":10,"dec":20}}`)
	if e != nil {
		t.Fatal(e)
	}
	p, e := req.normalize()
	if e != nil {
		t.Fatal(e)
	}
	if p.FOVArcmin != 3 || p.Mode != "blink" || !slices.Equal(p.Detectors, []int{1, 2, 3, 4, 5, 6}) {
		t.Errorf("defaults = fov %v mode %q detectors %v", p.FOVArcmin, p.Mode, p.Detectors)
	}
}

func TestNormalizeSortsDetectors(t *testing.T) {
	req, _ := decodeView(t, `{"target":{"ra":10,"dec":20},"detectors":[3,1,3]}`)
	p, e := req.normalize()
	if e != nil || !slices.Equal(p.Detectors, []int{1, 3}) {
		t.Fatalf("detectors = %v, %v; want [1 3]", p.Detectors, e)
	}
}

func TestViewRequestValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body, field string
		status            int
	}{
		{"fov above max (R1)", `{"target":{"ra":1,"dec":1},"fov_arcmin":30.5}`, "fov_arcmin", 400},
		{"fov below min", `{"target":{"ra":1,"dec":1},"fov_arcmin":0.5}`, "fov_arcmin", 400},
		{"bad mode", `{"target":{"ra":1,"dec":1},"mode":"rgb"}`, "mode", 400},
		{"detector 7", `{"target":{"ra":1,"dec":1},"detectors":[7]}`, "detectors", 400},
		{"empty detectors", `{"target":{"ra":1,"dec":1},"detectors":[]}`, "detectors", 400},
		{"bad date", `{"target":{"ra":1,"dec":1},"epochs":{"from":"June"}}`, "epochs.from", 400},
		{"dates reversed", `{"target":{"ra":1,"dec":1},"epochs":{"from":"2026-01-02","to":"2026-01-01"}}`, "epochs", 400},
		{"difference without a,b", `{"target":{"ra":1,"dec":1},"mode":"difference"}`, "difference", 400},
		{"difference same epoch", `{"target":{"ra":1,"dec":1},"mode":"difference","difference":{"a":1,"b":1}}`, "difference", 400},
		{"difference in blink", `{"target":{"ra":1,"dec":1},"difference":{"a":0,"b":1}}`, "difference", 400},
		{"bad stretch", `{"target":{"ra":1,"dec":1},"stretch":{"type":"linear","pmin":1,"pmax":99}}`, "stretch.type", 400},
		{"stretch range", `{"target":{"ra":1,"dec":1},"stretch":{"type":"log","pmin":99,"pmax":1}}`, "stretch", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, e := decodeView(t, tc.body)
			if e == nil {
				_, e = req.normalize()
			}
			if e == nil || e.status != tc.status || e.details["field"] != tc.field {
				t.Fatalf("got %v, want %d on %s", e, tc.status, tc.field)
			}
		})
	}
}

func TestTargetValidation(t *testing.T) {
	for _, tc := range []struct{ name, body, field string }{
		{"missing", `{}`, "target"},
		{"ra only", `{"target":{"ra":1}}`, "target"},
		{"both", `{"target":{"name":"Vega","ra":1,"dec":2}}`, "target"},
		{"ra above 360 (R3)", `{"target":{"ra":360.1,"dec":0}}`, "ra"},
		{"ra negative", `{"target":{"ra":-1,"dec":0}}`, "ra"},
		{"dec above 90", `{"target":{"ra":1,"dec":90.5}}`, "dec"},
		{"empty name", `{"target":{"name":"   "}}`, "target.name"},
		{"long name", `{"target":{"name":"` + strings.Repeat("x", 101) + `"}}`, "target.name"},
		{"control char", `{"target":{"name":"Ve\u0000ga"}}`, "target.name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, e := decodeView(t, tc.body)
			if e == nil {
				_, _, _, e = req.targetOf()
			}
			if e == nil || e.details["field"] != tc.field {
				t.Fatalf("got %v, want error on %s", e, tc.field)
			}
		})
	}

	req, _ := decodeView(t, `{"target":{"name":"  Barnard's   Star "}}`)
	if name, _, _, e := req.targetOf(); e != nil || name != "Barnard's Star" {
		t.Fatalf("name = %q, %v", name, e)
	}
}

func TestDecodeJSONErrors(t *testing.T) {
	for _, tc := range []struct{ name, body, field, contains string }{
		{"unknown field", `{"target":{"ra":1,"dec":1},"zoom":2}`, "", "unknown field"},
		{"wrong type", `{"fov_arcmin":"big"}`, "fov_arcmin", "must be a number"},
		{"trailing data", `{} {}`, "", "after the JSON"},
		{"empty", ``, "", "empty"},
		{"not json", `ra=1`, "", "not valid JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, e := decodeView(t, tc.body)
			if e == nil || e.status != http.StatusBadRequest || e.details["field"] != tc.field || !strings.Contains(e.message, tc.contains) {
				t.Fatalf("got %+v", e)
			}
		})
	}
}

func TestQueryCoordsRejectsNaN(t *testing.T) {
	for _, q := range [][2]string{{"NaN", "0"}, {"0", "NaN"}, {"Inf", "0"}, {"", "0"}, {"1", "x"}} {
		if _, _, e := queryCoords(q[0], q[1]); e == nil {
			t.Errorf("queryCoords(%q, %q) accepted", q[0], q[1])
		}
	}
	if ra, dec, e := queryCoords("360", "-90"); e != nil || ra != 360 || dec != -90 {
		t.Errorf("edges rejected: %v", e)
	}
}
