package agently

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCLIReportSummaryRequiresMatchingCommitAndKeepsAuthoredMeaning(t *testing.T) {
	start := "```forge-report\n{\"scope\":\"audience\",\"id\":\"deals\",\"mode\":\"start\",\"title\":\"Deal usage\",\"blocks\":[{\"kind\":\"markdownBlock\",\"markdown\":\"27 members have no recorded delivery; missing rows are not measured zeros.\"}]}\n```"
	require.Empty(t, committedCLIReportSummaries(start))
	foreign := "\n```forge-report\n{\"scope\":\"other\",\"id\":\"deals\",\"mode\":\"commit\"}\n```"
	require.Empty(t, committedCLIReportSummaries(start+foreign))
	commit := "\n```forge-report\n{\"scope\":\"audience\",\"id\":\"deals\",\"mode\":\"commit\"}\n```"
	got := committedCLIReportSummaries(start + commit)
	require.Len(t, got, 1)
	require.Equal(t, "Deal usage", got[0].title)
	require.Contains(t, got[0].text, "missing rows are not measured zeros")
}
