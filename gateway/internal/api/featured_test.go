package api

import (
	"encoding/json"
	"strings"
	"testing"
)

var testFeatured = []FeaturedTarget{
	{Slug: "barnards-star", Name: "Barnard's Star", Type: "High proper motion star", RA: 269.4520769, Dec: 4.6933649},
	{Slug: "r-draconis", Name: "R Draconis", Type: "Variable star", RA: 245.4486, Dec: 66.7518},
}

func TestEmbeddedFeaturedLoads(t *testing.T) {
	if _, err := DefaultFeatured(); err != nil {
		t.Fatal(err)
	}
}

func TestCheckFeatured(t *testing.T) {
	if err := checkFeatured(testFeatured); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		edit func(*FeaturedTarget)
		want string
	}{
		"bad slug": {func(f *FeaturedTarget) { f.Slug = "Barnard's Star" }, "slug"},
		"no name":  {func(f *FeaturedTarget) { f.Name = "" }, "name"},
		"bad ra":   {func(f *FeaturedTarget) { f.RA = 400 }, "ra"},
		"bad dec":  {func(f *FeaturedTarget) { f.Dec = -91 }, "dec"},
	} {
		t.Run(name, func(t *testing.T) {
			f := testFeatured[0]
			tc.edit(&f)
			if err := checkFeatured([]FeaturedTarget{f}); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want mention of %q", err, tc.want)
			}
		})
	}
	dup := []FeaturedTarget{testFeatured[0], testFeatured[0]}
	if err := checkFeatured(dup); err == nil || !strings.Contains(err.Error(), "unique") {
		t.Fatalf("duplicate slug: err = %v", err)
	}
}

func TestNewRejectsBadFeatured(t *testing.T) {
	cfg := testConfig()
	cfg.Featured = []FeaturedTarget{{Slug: "x", Name: "X", RA: -1}}
	if _, err := New(cfg, Deps{Log: quiet}); err == nil {
		t.Fatal("New accepted an invalid featured target")
	}
}

func TestGetObject(t *testing.T) {
	cfg := testConfig()
	cfg.Featured = testFeatured
	r := bareRouter(t, cfg)

	rec := do(t, r, "GET", "/api/v1/objects/r-draconis", "")
	var got FeaturedTarget
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got != testFeatured[1] {
		t.Fatalf("GET object = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, r, "GET", "/api/v1/objects/m31", ""); rec.Code != 404 || errorCode(t, rec) != "not_found" {
		t.Fatalf("unknown slug = %d %s", rec.Code, rec.Body)
	}
}
