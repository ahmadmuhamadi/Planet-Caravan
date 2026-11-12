package cache

import (
	"testing"

	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
)

func TestRoundCoords(t *testing.T) {
	for _, tc := range []struct{ ra, dec, wantRA, wantDec float64 }{
		{10, 20, 10, 20},
		{10.000001, -20.000001, 10, -20}, // 0.0036" rounds away
		{10.00002, 0, 10 + 1.0/36000, 0}, // 0.072" rounds to 0.1"
		{360, 0, 0, 0},                   // RA wraps
		{359.999999, 90, 0, 90},          // rounds up to 360, wraps
		{0, -90, 0, -90},
	} {
		ra, dec := RoundCoords(tc.ra, tc.dec)
		if ra != tc.wantRA || dec != tc.wantDec {
			t.Errorf("RoundCoords(%v, %v) = %v, %v; want %v, %v", tc.ra, tc.dec, ra, dec, tc.wantRA, tc.wantDec)
		}
	}
}

func params() computeclient.ViewParams {
	return computeclient.ViewParams{
		Target:      computeclient.Target{RA: 269.452, Dec: 4.6933},
		FOVArcmin:   3,
		Mode:        "blink",
		Detectors:   []int{1, 2, 3, 4, 5, 6},
		Collections: []string{"spherex_qr2", "spherex_qr3"},
	}
}

func TestViewKey(t *testing.T) {
	base := ViewKey(params())
	if base != ViewKey(params()) {
		t.Fatal("key is not stable")
	}

	named := params()
	named.Target.Name = "Barnard's Star"
	if ViewKey(named) != base {
		t.Error("target name changed the key")
	}

	for name, change := range map[string]func(*computeclient.ViewParams){
		"position":   func(p *computeclient.ViewParams) { p.Target.RA += 1.0 / 36000 },
		"fov":        func(p *computeclient.ViewParams) { p.FOVArcmin = 5 },
		"mode":       func(p *computeclient.ViewParams) { p.Mode = "difference" },
		"detectors":  func(p *computeclient.ViewParams) { p.Detectors = []int{1} },
		"epochs":     func(p *computeclient.ViewParams) { p.Epochs = &computeclient.DateRange{From: "2025-06-01"} },
		"difference": func(p *computeclient.ViewParams) { p.Difference = &computeclient.Difference{A: 0, B: 1} },
		"tracking":   func(p *computeclient.ViewParams) { p.Tracking = true },
		"stretch":    func(p *computeclient.ViewParams) { p.Stretch = &computeclient.Stretch{Type: "log", PMin: 1, PMax: 99} },
		"analysis":   func(p *computeclient.ViewParams) { p.Analysis.Sources = true },
		"releases":   func(p *computeclient.ViewParams) { p.Collections = append(p.Collections, "spherex_qr4") },
	} {
		p := params()
		change(&p)
		if ViewKey(p) == base {
			t.Errorf("changing %s kept the same key", name)
		}
	}
}

func TestNameKeys(t *testing.T) {
	if ResolveKey("  Barnard's   STAR ") != ResolveKey("barnard's star") {
		t.Error("resolve key not normalised")
	}
	if CoverageKey(10.000001, 20) != CoverageKey(10, 20) || CoverageKey(360, 0) != CoverageKey(0, 0) {
		t.Error("coverage key not rounded like view keys")
	}
}
