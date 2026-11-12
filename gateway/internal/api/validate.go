package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/cache"
	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
)

// Limits from CONTRACT.md (R1, R3).
const (
	minFOV     = 1.0
	defaultFOV = 3.0
	maxFOV     = 30.0
	maxNameLen = 100
)

var allDetectors = []int{1, 2, 3, 4, 5, 6}

// viewRequest is the POST /views body (CONTRACT). Pointers tell "missing" from zero.
type viewRequest struct {
	Target     *viewTarget               `json:"target"`
	FOVArcmin  *float64                  `json:"fov_arcmin"`
	Mode       string                    `json:"mode"`
	Detectors  []int                     `json:"detectors"`
	Epochs     *computeclient.DateRange  `json:"epochs"`
	Difference *computeclient.Difference `json:"difference"`
	Tracking   bool                      `json:"tracking"`
	Stretch    *computeclient.Stretch    `json:"stretch"`
	Analysis   *computeclient.Analysis   `json:"analysis"`
}

type viewTarget struct {
	Name *string  `json:"name"`
	RA   *float64 `json:"ra"`
	Dec  *float64 `json:"dec"`
}

// decodeJSON reads a strict JSON body: no unknown fields, nothing after the value.
func decodeJSON(r io.Reader, out any) *apiError {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	err := dec.Decode(out)
	if err == nil {
		if _, extra := dec.Token(); extra != io.EOF {
			err = errors.New("unexpected data after the JSON value")
		}
	}
	if err == nil {
		return nil
	}
	var tooLarge *http.MaxBytesError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &tooLarge):
		return &apiError{status: http.StatusRequestEntityTooLarge, code: "payload_too_large",
			message: fmt.Sprintf("Request body is larger than %d bytes.", tooLarge.Limit)}
	case errors.As(err, &typeErr):
		return badRequest(typeErr.Field, fmt.Sprintf("%s must be a %s.", typeErr.Field, jsonType(typeErr.Type.String())))
	case errors.Is(err, io.EOF):
		return badRequest("", "Request body is empty.")
	}
	return badRequest("", "Request body is not valid JSON: "+strings.TrimPrefix(err.Error(), "json: "))
}

func jsonType(goType string) string {
	switch strings.TrimLeft(goType, "*[]") {
	case "float64", "int":
		return "number"
	case "bool":
		return "boolean"
	case "string":
		return "string"
	}
	if strings.HasPrefix(goType, "[]") {
		return "list"
	}
	return "object"
}

// checkCoords validates a position (R3). RA 360 is the same as 0.
func checkCoords(ra, dec float64) *apiError {
	if math.IsNaN(ra) || ra < 0 || ra > 360 {
		return badRequest("ra", "ra must be between 0 and 360 degrees.")
	}
	if math.IsNaN(dec) || dec < -90 || dec > 90 {
		return badRequest("dec", "dec must be between -90 and 90 degrees.")
	}
	return nil
}

// queryCoords reads and validates ra and dec query parameters.
func queryCoords(raStr, decStr string) (float64, float64, *apiError) {
	ra, err := strconv.ParseFloat(raStr, 64)
	if err != nil {
		return 0, 0, badRequest("ra", "ra must be a number of degrees.")
	}
	dec, err := strconv.ParseFloat(decStr, 64)
	if err != nil {
		return 0, 0, badRequest("dec", "dec must be a number of degrees.")
	}
	return ra, dec, checkCoords(ra, dec)
}

// checkName trims an object name and validates its length (R3).
func checkName(field, name string) (string, *apiError) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "", badRequest(field, field+" must not be empty.")
	}
	if utf8.RuneCountInString(name) > maxNameLen {
		return "", badRequest(field, fmt.Sprintf("%s must be at most %d characters.", field, maxNameLen))
	}
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", badRequest(field, field+" must not contain control characters.")
	}
	return name, nil
}

// normalize validates a view request and fills defaults. The target is left
// for the caller: a name still has to be resolved. Collections are added later.
func (req *viewRequest) normalize() (computeclient.ViewParams, *apiError) {
	p := computeclient.ViewParams{FOVArcmin: defaultFOV, Mode: req.Mode, Tracking: req.Tracking}

	if req.FOVArcmin != nil {
		p.FOVArcmin = math.Round(*req.FOVArcmin*100) / 100
	}
	if math.IsNaN(p.FOVArcmin) || p.FOVArcmin < minFOV || p.FOVArcmin > maxFOV {
		return p, badRequest("fov_arcmin", fmt.Sprintf("fov_arcmin must be between %g and %g.", minFOV, maxFOV))
	}

	switch p.Mode {
	case "":
		p.Mode = "blink"
	case "blink", "difference":
	default:
		return p, badRequest("mode", "mode must be blink or difference.")
	}

	if req.Detectors == nil {
		p.Detectors = slices.Clone(allDetectors)
	} else {
		for _, d := range req.Detectors {
			if d < 1 || d > 6 {
				return p, badRequest("detectors", "detectors must be numbers from 1 to 6.")
			}
		}
		p.Detectors = slices.Compact(slices.Sorted(slices.Values(req.Detectors)))
		if len(p.Detectors) == 0 {
			return p, badRequest("detectors", "detectors must not be empty.")
		}
	}

	if req.Epochs != nil && (req.Epochs.From != "" || req.Epochs.To != "") {
		var from, to time.Time
		var err error
		if req.Epochs.From != "" {
			if from, err = time.Parse(time.DateOnly, req.Epochs.From); err != nil {
				return p, badRequest("epochs.from", "epochs.from must be a date (YYYY-MM-DD).")
			}
		}
		if req.Epochs.To != "" {
			if to, err = time.Parse(time.DateOnly, req.Epochs.To); err != nil {
				return p, badRequest("epochs.to", "epochs.to must be a date (YYYY-MM-DD).")
			}
		}
		if !from.IsZero() && !to.IsZero() && from.After(to) {
			return p, badRequest("epochs", "epochs.from must not be after epochs.to.")
		}
		p.Epochs = &computeclient.DateRange{From: req.Epochs.From, To: req.Epochs.To}
	}

	switch {
	case p.Mode == "difference" && req.Difference == nil:
		return p, badRequest("difference", "difference {a, b} is required when mode is difference.")
	case p.Mode != "difference" && req.Difference != nil:
		return p, badRequest("difference", "difference is only allowed when mode is difference.")
	case req.Difference != nil:
		d := *req.Difference
		if d.A < 0 || d.B < 0 || d.A == d.B {
			return p, badRequest("difference", "difference.a and difference.b must be two different epoch indices.")
		}
		p.Difference = &d
	}

	if req.Stretch != nil {
		s := *req.Stretch
		if s.Type != "asinh" && s.Type != "log" {
			return p, badRequest("stretch.type", "stretch.type must be asinh or log.")
		}
		if s.PMin < 0 || s.PMax > 100 || s.PMin >= s.PMax {
			return p, badRequest("stretch", "stretch needs 0 <= pmin < pmax <= 100.")
		}
		p.Stretch = &s
	}

	if req.Analysis != nil {
		p.Analysis = *req.Analysis
	}
	return p, nil
}

// targetOf validates the request target: exactly one of a name or ra+dec.
func (req *viewRequest) targetOf() (name string, ra, dec float64, e *apiError) {
	t := req.Target
	if t == nil {
		return "", 0, 0, badRequest("target", "target is required: {name} or {ra, dec}.")
	}
	hasCoords := t.RA != nil || t.Dec != nil
	switch {
	case t.Name != nil && hasCoords:
		return "", 0, 0, badRequest("target", "target takes a name or ra and dec, not both.")
	case t.Name != nil:
		name, e = checkName("target.name", *t.Name)
		return name, 0, 0, e
	case t.RA == nil || t.Dec == nil:
		return "", 0, 0, badRequest("target", "target is required: {name} or {ra, dec}.")
	}
	return "", *t.RA, *t.Dec, checkCoords(*t.RA, *t.Dec)
}

// withTarget sets the rounded position and the release set; the result is ready for cache.ViewKey.
func withTarget(p computeclient.ViewParams, name string, ra, dec float64, releases []computeclient.Release) computeclient.ViewParams {
	p.Target.RA, p.Target.Dec = cache.RoundCoords(ra, dec)
	p.Target.Name = name
	p.Collections = make([]string, 0, len(releases))
	for _, r := range releases {
		p.Collections = append(p.Collections, r.Collection)
	}
	p.Collections = slices.Compact(slices.Sorted(slices.Values(p.Collections)))
	return p
}
