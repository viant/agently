package prompt

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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
	mustWrite(fileVM, "User: ${Task.UserPrompt}")
	mustWrite(fileGO, "User: {{.Task.UserPrompt}}")

	cases := []struct {
		name    string
		prompt  Prompt
		binding *Binding
		want    string
	}{
		{
			name:    "inline-velty",
			prompt:  Prompt{Engine: "vm", Text: "User: ${Task.UserPrompt}"},
			binding: &Binding{Task: Task{UserPrompt: "Hello World"}},
			want:    "User: Hello World",
		},
		{
			name:    "inline-go",
			prompt:  Prompt{Engine: "go", Text: "User: {{.Task.UserPrompt}}"},
			binding: &Binding{Task: Task{UserPrompt: "Hello World"}},
			want:    "User: Hello World",
		},
		{
			name:    "uri-file-velty",
			prompt:  Prompt{Engine: "vm", URI: fileVM},
			binding: &Binding{Task: Task{UserPrompt: "Hello World"}},
			want:    "User: Hello World",
		},
		{
			name:    "uri-file-go",
			prompt:  Prompt{Engine: "go", URI: fileGO},
			binding: &Binding{Task: Task{UserPrompt: "Hello World"}},
			want:    "User: Hello World",
		},
		{
			name:    "uri-file-scheme-velty",
			prompt:  Prompt{Engine: "vm", URI: "file://" + fileVM},
			binding: &Binding{Task: Task{UserPrompt: "Hello World"}},
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
		"T: ${Task.UserPrompt}",
		"T: {{.Task.UserPrompt}}",
		&Binding{Task: Task{UserPrompt: "Compute"}},
		"T: Compute",
	)

	// History messages
	run(
		"history",
		"#foreach($m in $History.Messages)- $m.Role: $m.Content\n#end",
		"{{range .History.Messages}}- {{.Role}}: {{.Content}}\n{{end}}",
		&Binding{History: History{Messages: []*Message{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "hi"}}}},
		"- user: hello\n- assistant: hi\n",
	)

	// Tools signatures
	run(
		"tools-signatures",
		"#foreach($s in $Tools.Signatures)- $s.Name: $s.Description\n#end",
		"{{range .Tools.Signatures}}- {{.Name}}: {{.Description}}\n{{end}}",
		&Binding{Tools: Tools{Signatures: []*ToolDefinition{{Name: "search", Description: "find"}, {Name: "calc", Description: "compute"}}}},
		"- search: find\n- calc: compute\n",
	)

	// Tools executions
	run(
		"tools-executions",
		"#foreach($e in $Tools.Executions)- $e.Name: $e.Status ($e.Result)\n#end",
		"{{range .Tools.Executions}}- {{.Name}}: {{.Status}} ({{.Result}})\n{{end}}",
		&Binding{Tools: Tools{Executions: []*ToolCall{{Name: "search", Status: "completed", Result: "ok"}}}},
		"- search: completed (ok)\n",
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
