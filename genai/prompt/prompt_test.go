package prompt

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/viant/agently/genai/llm"
)

func TestPrompt_Generate_DataDriven(t *testing.T) {
	ctx := context.Background()

	// Prepare a temp file for URI-based template
	tmpDir := t.TempDir()
	fileVM := filepath.Join(tmpDir, "tmpl.vm")
	fileGO := filepath.Join(tmpDir, "tmpl.tmpl")
	mustWrite := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	mustWrite(fileVM, "User: ${Task.Prompt}")
	mustWrite(fileGO, "User: {{.Task.Prompt}}")

	cases := []struct {
		name    string
		prompt  Prompt
		binding *Binding
		want    string
	}{
		{
			name:    "inline-velty",
			prompt:  Prompt{Engine: "vm", Text: "User: ${Task.Prompt}"},
			binding: &Binding{Task: Task{Prompt: "Hello World"}},
			want:    "User: Hello World",
		},
		{
			name:    "inline-go",
			prompt:  Prompt{Engine: "go", Text: "User: {{.Task.Prompt}}"},
			binding: &Binding{Task: Task{Prompt: "Hello World"}},
			want:    "User: Hello World",
		},
		{
			name:    "uri-file-velty",
			prompt:  Prompt{Engine: "vm", URI: fileVM},
			binding: &Binding{Task: Task{Prompt: "Hello World"}},
			want:    "User: Hello World",
		},
		{
			name:    "uri-file-go",
			prompt:  Prompt{Engine: "go", URI: fileGO},
			binding: &Binding{Task: Task{Prompt: "Hello World"}},
			want:    "User: Hello World",
		},
		{
			name:    "uri-file-scheme-velty",
			prompt:  Prompt{Engine: "vm", URI: "file://" + fileVM},
			binding: &Binding{Task: Task{Prompt: "Hello World"}},
			want:    "User: Hello World",
		},
	}

	for _, tc := range cases {
		got, err := tc.prompt.Generate(ctx, tc.binding)
		assert.NoError(t, err, tc.name)
		assert.EqualValues(t, tc.want, got, tc.name)
	}
}

func TestPrompt_Generate_BindingCoverage(t *testing.T) {
	ctx := context.Background()

	run := func(name, vmTpl, goTpl string, binding *Binding, want string) {
		t.Run(name+"/vm", func(t *testing.T) {
			p := Prompt{Engine: "vm", Text: vmTpl}
			got, err := p.Generate(ctx, binding)
			assert.NoError(t, err)
			assert.EqualValues(t, want, got)
		})
		t.Run(name+"/go", func(t *testing.T) {
			p := Prompt{Engine: "go", Text: goTpl}
			got, err := p.Generate(ctx, binding)
			assert.NoError(t, err)
			assert.EqualValues(t, want, got)
		})
	}

	// Task only
	run(
		"task",
		"T: ${Task.Prompt}",
		"T: {{.Task.Prompt}}",
		&Binding{Task: Task{Prompt: "Compute"}},
		"T: Compute",
	)

	// History messages
	msgs := []*Message{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "hi"}}
	run(
		"history",
		"#foreach($m in $History.Messages)- $m.Role: $m.Content\n#end",
		"{{range .History.Messages}}- {{.Role}}: {{.Content}}\n{{end}}",
		&Binding{History: History{Past: []*Turn{{Messages: msgs}}, Messages: msgs}},
		"- user: hello\n- assistant: hi\n",
	)

	// Tool signatures
	run(
		"tools-signatures",
		"#foreach($s in $Tool.Signatures)- $s.Name: $s.Description\n#end",
		"{{range .Tools.Signatures}}- {{.Name}}: {{.Description}}\n{{end}}",
		&Binding{Tools: Tools{Signatures: []*llm.ToolDefinition{{Name: "search", Description: "find"}, {Name: "calc", Description: "compute"}}}},
		"- search: find\n- calc: compute\n",
	)

	// Tool executions (status removed in llm.ToolCall)
	run(
		"tools-executions",
		"#foreach($e in $Tool.Executions)- $e.Name: ($e.Result)\n#end",
		"{{range .Tools.Executions}}- {{.Name}}: ({{.Result}})\n{{end}}",
		&Binding{Tools: Tools{Executions: []*llm.ToolCall{{Name: "search", Result: "ok"}}}},
		"- search: (ok)\n",
	)

	// Documents
	run(
		"documents",
		"#foreach($d in $Documents.Items)- $d.Title ($d.SourceURI)\n#end",
		"{{range .Documents.Items}}- {{.Title}} ({{.SourceURI}})\n{{end}}",
		&Binding{Documents: Documents{Items: []*Document{{Title: "Guide", SourceURI: "uri://a"}, {Title: "Spec", SourceURI: "uri://b"}}}},
		"- Guide (uri://a)\n- Spec (uri://b)\n",
	)

	// Flags
	run(
		"flags",
		"#if($Flags.CanUseTool)CAN#elseCANNOT#end",
		"{{if .Flags.CanUseTool}}CAN{{else}}CANNOT{{end}}",
		&Binding{Flags: Flags{CanUseTool: true}},
		"CAN",
	)
}

func TestPrompt_Generate_ReloadsURI(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	fileVM := filepath.Join(tmpDir, "sys.vm")
	fileGO := filepath.Join(tmpDir, "sys.tmpl")

	mustWrite := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	// Initial content
	mustWrite(fileVM, "[VM] ${Task.Prompt}")
	mustWrite(fileGO, "[GO] {{.Task.Prompt}}")

	binding := &Binding{Task: Task{Prompt: "Hello"}}

	// First generate
	pVM := Prompt{Engine: "vm", URI: fileVM}
	pGO := Prompt{Engine: "go", URI: fileGO}

	got1VM, err := pVM.Generate(ctx, binding)
	assert.NoError(t, err)
	assert.EqualValues(t, "[VM] Hello", got1VM)

	got1GO, err := pGO.Generate(ctx, binding)
	assert.NoError(t, err)
	assert.EqualValues(t, "[GO] Hello", got1GO)

	// Update file contents to simulate hot-swap edits
	mustWrite(fileVM, "[VM2] ${Task.Prompt}")
	mustWrite(fileGO, "[GO2] {{.Task.Prompt}}")

	got2VM, err := pVM.Generate(ctx, binding)
	assert.NoError(t, err)
	assert.EqualValues(t, "[VM2] Hello", got2VM)

	got2GO, err := pGO.Generate(ctx, binding)
	assert.NoError(t, err)
	assert.EqualValues(t, "[GO2] Hello", got2GO)
}
