package agently

import (
	"context"
	"fmt"
	"testing"

	"github.com/viant/authz"
)

func TestConfiguredAccountProjectionComposesTrustedContextAndPreservesID(t *testing.T) {
	for _, mode := range []string{"", "numeric", "opaque"} {
		project, err := configuredAccountProjection(mode, func(_ context.Context, expected authz.Facts, account string) (map[string]any, error) {
			if expected.Subject != "verified" || account != "21" {
				t.Fatal("verified binding changed")
			}
			return map[string]any{"id": "21", "businessModelContext": 1}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		output, err := project(context.Background(), authz.Facts{Subject: "verified"}, "21")
		if err != nil || output["businessModelContext"] != 1 || fmt.Sprint(output["id"]) != "21" {
			t.Fatal("projection lost", output, err)
		}
		if mode == "opaque" {
			if _, ok := output["id"].(string); !ok {
				t.Fatal("opaque ID format changed")
			}
		} else {
			if _, ok := output["id"].(int64); !ok {
				t.Fatal("numeric ID format changed")
			}
		}
	}
	project, _ := configuredAccountProjection("opaque", func(context.Context, authz.Facts, string) (map[string]any, error) {
		return map[string]any{"id": "22", "businessModelContext": 1}, nil
	})
	if result, err := project(context.Background(), authz.Facts{}, "21"); err == nil || result != nil {
		t.Fatal("provider ID mismatch accepted")
	}
}
