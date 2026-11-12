package computeclient

import (
	_ "embed"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// B6 / H3: the Go types must match the pinned compute schema.

//go:embed openapi.json
var pinnedSchema []byte

type schema struct {
	Type       string            `json:"type"`
	Ref        string            `json:"$ref"`
	AnyOf      []schema          `json:"anyOf"`
	Properties map[string]schema `json:"properties"`
	Required   []string          `json:"required"`
}

type openAPI struct {
	Paths map[string]map[string]struct {
		RequestBody struct {
			Content map[string]struct{ Schema schema } `json:"content"`
		} `json:"requestBody"`
		Responses map[string]struct {
			Content map[string]struct{ Schema schema } `json:"content"`
		} `json:"responses"`
	} `json:"paths"`
	Components struct {
		Schemas map[string]schema `json:"schemas"`
	} `json:"components"`
}

func loadSchema(t *testing.T) openAPI {
	t.Helper()
	var doc openAPI
	if err := json.Unmarshal(pinnedSchema, &doc); err != nil {
		t.Fatalf("parse openapi.json: %v", err)
	}
	return doc
}

func refName(s schema) string { return strings.TrimPrefix(s.Ref, "#/components/schemas/") }

func TestContractEndpoints(t *testing.T) {
	doc := loadSchema(t)
	for _, tc := range []struct{ path, req, resp string }{
		{"/internal/resolve", "ResolveRequest", "ResolveResponse"},
		{"/internal/coverage", "CoverageRequest", "CoverageResponse"},
		{"/internal/process", "ProcessRequest", "ProcessResponse"},
	} {
		op, ok := doc.Paths[tc.path]["post"]
		if !ok {
			t.Errorf("POST %s missing from pinned schema", tc.path)
			continue
		}
		if got := refName(op.RequestBody.Content["application/json"].Schema); got != tc.req {
			t.Errorf("POST %s request = %q, want %q", tc.path, got, tc.req)
		}
		if got := refName(op.Responses["200"].Content["application/json"].Schema); got != tc.resp {
			t.Errorf("POST %s 200 response = %q, want %q", tc.path, got, tc.resp)
		}
	}
}

func TestContractTypes(t *testing.T) {
	doc := loadSchema(t)
	types := map[string]any{
		"ResolveRequest": ResolveRequest{}, "ResolveResponse": ResolveResponse{},
		"ProperMotion": ProperMotion{}, "Link": Link{},
		"CoverageRequest": CoverageRequest{}, "CoverageResponse": CoverageResponse{},
		"Epoch": Epoch{}, "Release": Release{},
		"ProcessRequest": ProcessRequest{}, "ViewParams": ViewParams{}, "Target": Target{},
		"DateRange": DateRange{}, "Difference": Difference{}, "Stretch": Stretch{},
		"Analysis": Analysis{}, "ProcessResponse": ProcessResponse{}, "Frame": Frame{},
		"ErrorBody": ErrorBody{}, "ErrorDetail": ErrorDetail{},
	}
	for name := range doc.Components.Schemas {
		if _, ok := types[name]; !ok {
			t.Errorf("schema %s has no Go type", name)
		}
	}
	for name, v := range types {
		s, ok := doc.Components.Schemas[name]
		if !ok {
			t.Errorf("Go type for %s missing from pinned schema", name)
			continue
		}
		fields := jsonFields(reflect.TypeOf(v))
		var required []string
		for prop, f := range fields {
			ps, ok := s.Properties[prop]
			if !ok {
				t.Errorf("%s.%s: not in schema", name, prop)
				continue
			}
			if !f.omitempty {
				required = append(required, prop)
			}
			if want := schemaKind(ps); want != "" && want != goKind(f.typ) {
				t.Errorf("%s.%s: Go kind %s, schema type %s", name, prop, goKind(f.typ), want)
			}
		}
		for prop := range s.Properties {
			if _, ok := fields[prop]; !ok {
				t.Errorf("%s.%s: in schema, missing from Go type", name, prop)
			}
		}
		slices.Sort(required)
		want := slices.Clone(s.Required)
		slices.Sort(want)
		if !slices.Equal(required, want) {
			t.Errorf("%s: required fields (no omitempty) %v, schema %v", name, required, want)
		}
	}
}

type field struct {
	typ       reflect.Type
	omitempty bool
}

// jsonFields returns a struct's JSON fields, flattening embedded structs.
func jsonFields(t reflect.Type) map[string]field {
	out := map[string]field{}
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if f.Anonymous && tag == "" {
			for k, v := range jsonFields(f.Type) {
				out[k] = v
			}
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = field{typ: f.Type, omitempty: strings.Contains(opts, "omitempty")}
	}
	return out
}

// schemaKind maps a property schema to a comparable kind; nullable anyOf uses its non-null branch.
func schemaKind(s schema) string {
	for _, a := range s.AnyOf {
		if a.Type != "null" {
			return schemaKind(a)
		}
	}
	switch {
	case s.Ref != "":
		return "object"
	case s.Type == "integer", s.Type == "number", s.Type == "string", s.Type == "boolean", s.Type == "array", s.Type == "object":
		return s.Type
	}
	return ""
}

func goKind(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int64, reflect.Int32:
		return "integer"
	case reflect.Float64, reflect.Float32:
		return "number"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Slice:
		return "array"
	case reflect.Struct, reflect.Map:
		return "object"
	}
	return t.Kind().String()
}
