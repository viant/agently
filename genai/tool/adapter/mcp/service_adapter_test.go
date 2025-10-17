package mcp

import (
	"reflect"
	"testing"

	svc "github.com/viant/agently/genai/tool/service"
)

type testInner struct {
	Street string `json:"street" description:"Street address"`
	Zip    int    `json:"zip"`
}

type testInput struct {
	Name    string            `json:"name" description:"User name"`
	Age     *int              `json:"age,omitempty"`
	Secret  string            `json:"secret" internal:"true"`
	Tags    []string          `json:"tags"`
	Data    []byte            `json:"data"`
	Address testInner         `json:"address"`
	Meta    map[string]string `json:"meta"`
}

type testOutput struct {
	Ok bool `json:"ok"`
}

type fakeService struct{}

func (fakeService) Name() string { return "test/service" }
func (fakeService) Methods() svc.Signatures {
	return []svc.Signature{{
		Name:        "doThing",
		Description: "Does a thing",
		Input:       reflect.TypeOf(&testInput{}),
		Output:      reflect.TypeOf(&testOutput{}),
	}}
}
func (fakeService) Method(string) (svc.Executable, error) { return nil, nil }

func TestFromService_JSONSchemaGeneration(t *testing.T) {
	tools := FromService(fakeService{})
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool := tools[0]
	if tool.Name != "doThing" {
		t.Fatalf("unexpected tool name: %s", tool.Name)
	}
	if tool.Description == nil || *tool.Description != "Does a thing" {
		t.Fatalf("unexpected description: %v", tool.Description)
	}

	// Validate input schema
	props := map[string]map[string]interface{}(tool.InputSchema.Properties)
	required := map[string]bool{}
	for _, r := range tool.InputSchema.Required {
		required[r] = true
	}

	// name
	if p, ok := props["name"]; !ok {
		t.Fatalf("missing property: name")
	} else {
		if p["type"] != "string" {
			t.Errorf("name.type=%v", p["type"])
		}
		if p["description"] != "User name" {
			t.Errorf("name.description=%v", p["description"])
		}
		if !required["name"] {
			t.Errorf("name should be required")
		}
	}
	// age (pointer, omitempty → not required)
	if _, ok := props["age"]; !ok {
		t.Errorf("missing property: age")
	}
	if required["age"] {
		t.Errorf("age should not be required")
	}
	// secret (internal → omitted)
	if _, ok := props["secret"]; ok {
		t.Errorf("secret should be omitted (internal)")
	}
	// tags (array of strings) and required
	if p, ok := props["tags"]; !ok {
		t.Fatalf("missing property: tags")
	} else {
		if p["type"] != "array" {
			t.Errorf("tags.type=%v", p["type"])
		}
		items, _ := p["items"].(map[string]interface{})
		if items["type"] != "string" {
			t.Errorf("tags.items.type=%v", items["type"])
		}
		if !required["tags"] {
			t.Errorf("tags should be required")
		}
	}
	// data ([]byte → string)
	if p, ok := props["data"]; !ok {
		t.Fatalf("missing property: data")
	} else if p["type"] != "string" {
		t.Errorf("data.type expected string, got %v", p["type"])
	}
	if !required["data"] {
		t.Errorf("data should be required")
	}

	// address (object with properties) and required
	if p, ok := props["address"]; !ok {
		t.Fatalf("missing property: address")
	} else {
		if p["type"] != "object" {
			t.Errorf("address.type=%v", p["type"])
		}
		ap, _ := p["properties"].(map[string]map[string]interface{})
		if ap == nil {
			t.Fatalf("address.properties missing or wrong type")
		}
		if ap["street"]["type"] != "string" {
			t.Errorf("address.street.type=%v", ap["street"]["type"])
		}
		if ap["zip"]["type"] != "integer" {
			t.Errorf("address.zip.type=%v", ap["zip"]["type"])
		}
		if !required["address"] {
			t.Errorf("address should be required")
		}
	}

	// meta (map → object) and required
	if p, ok := props["meta"]; !ok {
		t.Fatalf("missing property: meta")
	} else if p["type"] != "object" {
		t.Errorf("meta.type=%v", p["type"])
	}
	if !required["meta"] {
		t.Errorf("meta should be required")
	}

	// Validate output schema
	if tool.OutputSchema == nil {
		t.Fatalf("nil OutputSchema")
	}
	outProps := tool.OutputSchema.Properties
	if outProps["ok"]["type"] != "boolean" {
		t.Errorf("output ok.type=%v", outProps["ok"]["type"])
	}
}
