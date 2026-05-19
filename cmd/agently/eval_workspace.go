package agently

import (
	"fmt"
	"os"

	"github.com/viant/agently-core/workspaceeval"
)

type EvalWorkspaceCmd struct {
	Workspace         string `long:"workspace" description:"Workspace root to evaluate"`
	Behavioral        bool   `long:"behavioral" description:"Execute selected eval prompts through the live Agently runtime and assert transcript behavior"`
	BehavioralCases   string `long:"behavioral-cases" description:"Comma-separated eval ids or yaml basenames to execute in behavioral mode"`
	BehavioralTimeout int    `long:"behavioral-timeout" description:"Per-eval timeout in seconds for behavioral mode" default:"180"`
	BehavioralAPI     string `long:"behavioral-api" description:"Agently base URL for behavioral mode"`
	BehavioralOOB     string `long:"behavioral-oob" description:"OOB secrets URL for behavioral mode transcript/auth access"`
	BehavioralToken   string `long:"behavioral-token" description:"Bearer token for behavioral mode transcript/auth access"`
	BehavioralBin     string `long:"behavioral-agently-bin" description:"Path to agently binary used for behavioral sub-runs (defaults to current executable)"`
}

func (c *EvalWorkspaceCmd) Execute(_ []string) error {
	bin := c.BehavioralBin
	if bin == "" {
		if exe, err := os.Executable(); err == nil {
			bin = exe
		}
	}
	err := workspaceeval.Run(workspaceeval.Options{
		Workspace:            c.Workspace,
		ContractTests:        workspaceeval.DefaultContractTests(),
		RequiredProfiles:     workspaceeval.DefaultRequiredEvidenceContractProfiles(),
		Behavioral:           c.Behavioral,
		BehavioralCases:      c.BehavioralCases,
		BehavioralTimeoutSec: c.BehavioralTimeout,
		BehavioralAPI:        c.BehavioralAPI,
		BehavioralOOB:        c.BehavioralOOB,
		BehavioralToken:      c.BehavioralToken,
		BehavioralAgentlyBin: bin,
	})
	if err != nil {
		return err
	}
	if c.Behavioral {
		fmt.Println("workspace eval gate ✓ catalog, public-agent coverage, contract tests, and behavioral transcript checks passed")
		return nil
	}
	fmt.Println("workspace eval gate ✓ catalog, public-agent coverage, and contract tests passed")
	return nil
}
