package agently

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/viant/agently-core/sdk"
	"github.com/viant/agently-core/sdk/api"
	datasources "github.com/viant/agently-core/service/datasource"
)

type nativeDeliverySDKBackend struct {
	sdk.Backend
	service *datasources.Service
}

func (b nativeDeliverySDKBackend) FetchDatasource(ctx context.Context, in *api.FetchDatasourceInput) (*api.FetchDatasourceOutput, error) {
	result, err := b.service.Fetch(ctx, in.ID, in.Inputs, datasources.FetchOptions{SelectionBinding: in.SelectionBinding, Resource: in.Resource, Target: in.Target, ResourceProof: in.ResourceProof, WindowParameters: in.WindowParameters})
	if err != nil {
		return nil, err
	}
	return &api.FetchDatasourceOutput{Rows: result.Rows}, nil
}

func TestNativeSignedDeliverySDKHTTPPreservesExactParametersAndArgs(t *testing.T) {
	// SDK startup staging cleanup is confined to this test's own temporary root.
	t.Setenv("TMPDIR", t.TempDir())
	f := newNativeDeliveryFixture(t)
	f.parameters["Selected"] = []any{json.Number("9007199254740993")}
	binding := f.bind(t)
	calls := 0
	service := f.service(t, nativePermissionExecutor(func(ctx context.Context, name string, inputs map[string]any) (string, error) {
		calls++
		if ctx.Value(nativeDeliveryOperation{}) != "original-operation" || name != "arbitrary:Patch" {
			t.Fatal("managing MCP lost operation identity")
		}
		values, err := nativeSelectionValues(inputs, []string{"Objects", "*", "exact"})
		if err != nil || len(values) != 1 || !nativeJSONEqual(values[0], json.Number("9007199254740993")) {
			t.Fatalf("mutable managingMCP input rounded: %v", values)
		}
		return `{"data":[{"id":42}]}`, nil
	}))
	handler, err := sdk.NewHandlerWithContext(context.Background(), nativeDeliverySDKBackend{service: service})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(context.WithValue(r.Context(), nativeDeliveryIngress{}, "canonical-user"), nativeDeliveryOperation{}, "original-operation")
		handler.ServeHTTP(w, r.WithContext(ctx))
	}))
	defer server.Close()
	client, err := sdk.NewHTTP(server.URL, sdk.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	input := &api.FetchDatasourceInput{ID: "change", Resource: f.window.Resource, Target: f.window.ResourceTarget, SelectionBinding: binding, WindowParameters: f.parameters, Inputs: map[string]any{"Objects": []map[string]any{{"Id": json.Number("9007199254740993"), "opaqueValue": "item:alpha", "exact": json.Number("9007199254740993")}}}}
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := client.FetchDatasource(f.ctx, input)
	if err != nil || output == nil || calls != 1 {
		t.Fatalf("signed SDK HTTP delivery dispatch=%d error=%v", calls, err)
	}
	for _, change := range []string{"foreign body", "full selection", "signature", "disabled mapping"} {
		t.Run(change, func(t *testing.T) {
			var request api.FetchDatasourceInput
			if err := json.Unmarshal(before, &request); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "foreign body":
				request.Inputs = map[string]any{"Objects": []map[string]any{{"Id": "foreign"}}}
			case "full selection":
				request.WindowParameters = map[string]any{"Selected": []any{"foreign"}}
			case "signature":
				request.SelectionBinding.Token = "forged"
			case "disabled mapping":
				f.users.record.Disabled = true
				defer func() { f.users.record.Disabled = false }()
			}
			body, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			response, err := server.Client().Post(server.URL+"/v1/api/datasources/change/fetch", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var failure map[string]any
			if err := json.NewDecoder(response.Body).Decode(&failure); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != 403 || failure["code"] != "resource_denied" || failure["scope"] != "window" || calls != 1 {
				t.Fatalf("wrong root/fidelity boundary: status=%d response=%v dispatches=%d", response.StatusCode, failure, calls)
			}
		})
	}
	after, err := json.Marshal(input)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("SDK/host altered original request body")
	}
}
