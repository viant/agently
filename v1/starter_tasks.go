package v1

import svcworkspace "github.com/viant/agently-core/service/workspace"

func defaultStarterTasks() []svcworkspace.StarterTask {
	return []svcworkspace.StarterTask{
		{
			ID:          "analyze-repo",
			Title:       "Analyze this repo",
			Prompt:      "Analyze this repository and summarize the architecture, key risks, and the first 5 improvements you recommend.",
			Description: "Architecture summary and next steps.",
			Icon:        "tree-structure",
		},
		{
			ID:          "fix-bugs",
			Title:       "Find and fix bugs",
			Prompt:      "Find one or two high-confidence bugs in this codebase, fix them with minimal changes, and explain the impact.",
			Description: "Minimal, high-confidence fixes.",
			Icon:        "bug",
		},
		{
			ID:          "write-tests",
			Title:       "Add missing tests",
			Prompt:      "Find an under-tested area in this project, add focused unit tests, run the relevant test command, and summarize what changed.",
			Description: "Targeted test coverage improvements.",
			Icon:        "flask",
		},
		{
			ID:          "make-plan",
			Title:       "Create a plan",
			Prompt:      "Create a concise implementation plan for the next meaningful improvement in this project, with milestones and risks.",
			Description: "Milestones, risks, and sequence.",
			Icon:        "pencil",
		},
		{
			ID:          "prototype-feature",
			Title:       "Prototype a feature",
			Prompt:      "Propose one high-leverage feature for this application, implement a thin prototype, and explain the tradeoffs.",
			Description: "Ship a thin vertical slice.",
			Icon:        "rocket",
		},
		{
			ID:          "review-architecture",
			Title:       "Review failure modes",
			Prompt:      "Explain the top failure modes in this application's architecture and suggest pragmatic mitigations.",
			Description: "Operational and design risks.",
			Icon:        "shield-warning",
		},
	}
}
