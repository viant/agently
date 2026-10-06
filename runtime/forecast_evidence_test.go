package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/viant/agently-core/app/executor"
	execconfig "github.com/viant/agently-core/app/executor/config"
	appserver "github.com/viant/agently-core/app/server"
	apiconv "github.com/viant/agently-core/app/store/conversation"
	reportstore "github.com/viant/agently-core/app/store/reporting"
	reportmemory "github.com/viant/agently-core/app/store/reporting/memory"
	documents "github.com/viant/agently-core/app/store/reportingevidence"
	"github.com/viant/agently-core/genai/llm"
	runmodel "github.com/viant/agently-core/model/run"
	"github.com/viant/agently-core/protocol/tool"
	evidence "github.com/viant/agently-core/runtime/evidence"
	requestctx "github.com/viant/agently-core/runtime/requestctx"
	"github.com/viant/agently-core/service/agent"
	authsvc "github.com/viant/agently-core/service/auth"
	"github.com/viant/agently-core/service/reporting"
	fb "github.com/viant/agently-core/service/reporting/forecastbinding"
	"github.com/viant/agently-core/service/reportingrun"
	"github.com/viant/agently-core/workspace"
	"time"
)

type forecastCapabilityRegistry struct {
	tool.Registry
	definition *llm.ToolDefinition
}

func (r *forecastCapabilityRegistry) Initialize(context.Context) {}

func (r *forecastCapabilityRegistry) GetDefinition(string) (*llm.ToolDefinition, bool) {
	return r.definition, r.definition != nil
}
func validForecastCapability() *llm.ToolDefinition {
	return &llm.ToolDefinition{Parameters: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"Request": map[string]interface{}{"type": "object", "properties": map[string]interface{}{"evidenceProfile": map[string]interface{}{"type": "object"}, "evidenceAudienceId": map[string]interface{}{"type": "integer"}, "evidenceSourceOpId": map[string]interface{}{"type": "string"}}}}}, OutputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"forecastEvidence": map[string]interface{}{"type": "object"}}}}
}
func writeForecastConfig(t *testing.T, root, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.yaml"), []byte(body), 0644))
}

const enabledForecastConfig = "features:\n  forecastEvidence:\n    enabled: true\n    profile: forecast-daily-v1\n    timeZone: America/Los_Angeles\n    rolloutCutoff: '2026-10-01T00:00:00Z'\n"

func TestForecastEvidenceDefaultDisabledAndConfigurationValidation(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, ConfigureForecastEvidence(context.Background(), nil, root))
	for _, body := range []string{"features:\n  forecastEvidence:\n    enabled: false\n", enabledForecastConfig} {
		writeForecastConfig(t, root, body)
		cfg, err := LoadForecastEvidenceConfig(root)
		require.NoError(t, err)
		require.Equal(t, strings.Contains(body, "true"), cfg.Enabled)
	}
	for _, edit := range [][2]string{{"forecast-daily-v1", "unknown-profile"}, {"America/Los_Angeles", "Unknown/Zone"}, {"'2026-10-01T00:00:00Z'", "''"}, {"timeZone:", "wrongField:"}} {
		writeForecastConfig(t, root, strings.Replace(enabledForecastConfig, edit[0], edit[1], 1))
		_, err := LoadForecastEvidenceConfig(root)
		require.Error(t, err)
	}
}
func TestForecastEvidenceCutoffActivationValidationKeepsPinnedValue(t *testing.T) {
	root := t.TempDir()
	writeForecastConfig(t, root, enabledForecastConfig)
	cutoff, err := time.Parse(time.RFC3339, "2026-10-01T00:00:00Z")
	require.NoError(t, err)
	for _, now := range []time.Time{cutoff, cutoff.Add(time.Nanosecond), cutoff.Add(24 * time.Hour)} {
		cfg, err := loadForecastEvidenceConfigAt(root, now)
		require.NoError(t, err)
		require.Equal(t, cutoff, cfg.cutoff)
	}
	_, err = loadForecastEvidenceConfigAt(root, cutoff.Add(-time.Nanosecond))
	require.Error(t, err, "future configured cutoff must not grandfather new runs")
	_, err = loadForecastEvidenceConfigAt(root, time.Time{})
	require.Error(t, err)
}

func TestForecastEvidenceCapabilityRejectsUndeployedInputsAndOutput(t *testing.T) {
	registry := &forecastCapabilityRegistry{definition: validForecastCapability()}
	require.NoError(t, validateForecastEvidenceCapability(context.Background(), registry))
	raw, _ := json.Marshal(registry.definition)
	var clone llm.ToolDefinition
	require.NoError(t, json.Unmarshal(raw, &clone))
	clone.Parameters["properties"].(map[string]interface{})["Request"].(map[string]interface{})["properties"] = map[string]interface{}{"inclusion": map[string]interface{}{"type": "string"}, "to": map[string]interface{}{"type": "string"}}
	clone.OutputSchema = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	registry.definition = &clone
	require.Error(t, validateForecastEvidenceCapability(context.Background(), registry), "current deployed catalog cannot enable rollout")
	registry.definition = validForecastCapability()
	registry.definition.OutputSchema = map[string]interface{}{"type": "object"}
	require.Error(t, validateForecastEvidenceCapability(context.Background(), registry))
	registry.definition = nil
	require.Error(t, validateForecastEvidenceCapability(context.Background(), registry))
}
func TestForecastEvidenceCapabilityResolvesLocalSchemaReferencesWithoutTrustingExternalRefs(t *testing.T) {
	definition := validForecastCapability()
	request := definition.Parameters["properties"].(map[string]interface{})["Request"]
	definition.Parameters["$defs"] = map[string]interface{}{"Request": request}
	definition.Parameters["properties"].(map[string]interface{})["Request"] = map[string]interface{}{"$ref": "#/$defs/Request"}
	registry := &forecastCapabilityRegistry{definition: definition}
	require.NoError(t, validateForecastEvidenceCapability(context.Background(), registry))
	definition.Parameters["properties"].(map[string]interface{})["Request"] = map[string]interface{}{"$ref": "https://untrusted.invalid/Request"}
	require.Error(t, validateForecastEvidenceCapability(context.Background(), registry))
	definition.Parameters["properties"].(map[string]interface{})["Request"] = map[string]interface{}{"$ref": "#/$defs/Loop"}
	definition.Parameters["$defs"].(map[string]interface{})["Loop"] = map[string]interface{}{"$ref": "#/$defs/Loop"}
	require.Error(t, validateForecastEvidenceCapability(context.Background(), registry))
}

func TestForecastEvidenceRealStartupServicesShareNativeAuthority(t *testing.T) {
	ctx := authsvc.InjectUser(context.Background(), "fixture-owner")
	root := t.TempDir()
	writeForecastConfig(t, root, enabledForecastConfig)
	previousWorkspace := workspace.Root()
	t.Cleanup(func() { workspace.SetRoot(previousWorkspace) })
	defaults := &execconfig.Defaults{Reporting: execconfig.ReportingDefaults{Enabled: true, QueueIntervalMs: int(time.Hour / time.Millisecond), TransitionalWithUI: execconfig.ReportingTransitionalWithUIDefaults{Admission: "open", Persistence: "enabled"}}}
	rt, _, _, err := appserver.BuildWorkspaceRuntime(ctx, appserver.RuntimeOptions{WorkspaceRoot: root, Defaults: defaults, SkipRegistryInitialize: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, rt.Close(context.Background())) })
	rt.Registry = &forecastCapabilityRegistry{definition: validForecastCapability()}
	require.NoError(t, validateForecastEvidenceCapability(ctx, rt.Registry))
	cfg, err := LoadForecastEvidenceConfig(root)
	require.NoError(t, err)
	factory, err := installForecastEvidence(rt, cfg)
	require.NoError(t, err)
	// All three real services now resolve through the installed authority, before
	// any business/provider call. Invalid intent is rejected at capture.
	err = rt.Agent.Query(ctx, &agent.QueryInput{Context: map[string]interface{}{"client": map[string]interface{}{"forecastIntent": map[string]interface{}{"version": 99}}}}, &agent.QueryOutput{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "forecast")
	_, err = rt.ReportRuns.Begin(ctx, &reportingrun.BeginInput{UIRunRequestID: "fixture-command", ConversationID: "fixture-conversation", BuilderRef: "builder", ReportAdmissionRef: "bad-reference"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid command reference")
	_, err = rt.Reporting.CompileFencedReport(ctx, &reporting.CompileFencedReportRequest{ReportID: "fixture-command", ReportAdmissionRef: "bad-reference"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid command reference")
	// An owned pre-cutoff execution without admission retains ordinary report
	// behavior through the real newly configured services.
	conversation := apiconv.NewConversation()
	conversation.SetId("legacy-conversation")
	conversation.SetCreatedByUserID("fixture-owner")
	conversation.SetStatus("completed")
	require.NoError(t, rt.Conversation.PatchConversations(ctx, conversation))
	turn := apiconv.NewTurn()
	turn.SetId("legacy-turn")
	turn.SetConversationID("legacy-conversation")
	turn.SetStatus("completed")
	require.NoError(t, rt.Conversation.PatchTurn(ctx, turn))
	run := &runmodel.MutableRunView{}
	run.SetId("legacy-turn")
	run.SetTurnID("legacy-turn")
	run.SetConversationID("legacy-conversation")
	run.SetEffectiveUserID("fixture-owner")
	run.SetStatus("completed")
	run.SetCreatedAt(cfg.cutoff.Add(-time.Second))
	_, err = rt.Data.PatchRuns(ctx, []*runmodel.MutableRunView{run})
	require.NoError(t, err)
	legacyCtx, err := factory.Restore(ctx, evidence.Turn{ConversationID: "legacy-conversation", TurnID: "legacy-turn", LeaseOwner: func() string { return "lease" }})
	require.NoError(t, err)
	require.Nil(t, evidence.PublicationFromContext(legacyCtx))
	ordinaryReportRoundTrip(t, legacyCtx, rt, "legacy-conversation", "legacy-report")
	for _, testCase := range []struct {
		name              string
		at                time.Time
		conversationOwner string
	}{{"equal-cutoff", cfg.cutoff, "fixture-owner"}, {"after-cutoff", cfg.cutoff.Add(time.Second), "fixture-owner"}, {"foreign-owner", cfg.cutoff.Add(-time.Second), "foreign"}} {
		c := apiconv.NewConversation()
		c.SetId(testCase.name)
		c.SetCreatedByUserID(testCase.conversationOwner)
		c.SetStatus("completed")
		require.NoError(t, rt.Conversation.PatchConversations(ctx, c))
		tr := apiconv.NewTurn()
		tr.SetId(testCase.name)
		tr.SetConversationID(testCase.name)
		tr.SetStatus("completed")
		require.NoError(t, rt.Conversation.PatchTurn(ctx, tr))
		r := &runmodel.MutableRunView{}
		r.SetId(testCase.name)
		r.SetTurnID(testCase.name)
		r.SetConversationID(testCase.name)
		r.SetEffectiveUserID("fixture-owner")
		r.SetStatus("completed")
		r.SetCreatedAt(testCase.at)
		_, err = rt.Data.PatchRuns(ctx, []*runmodel.MutableRunView{r})
		require.NoError(t, err)
		_, err = factory.Restore(ctx, evidence.Turn{ConversationID: testCase.name, TurnID: testCase.name, LeaseOwner: func() string { return "lease" }})
		require.Error(t, err, testCase.name)
	}
	// The configured Factory is dormant for a newly admitted ordinary turn.
	// Its publication guard must not classify unrelated data by content/name.
	newTurn := apiconv.NewTurn()
	newTurn.SetId("ordinary-turn")
	newTurn.SetConversationID("legacy-conversation")
	newTurn.SetStatus("running")
	require.NoError(t, rt.Conversation.PatchTurn(ctx, newTurn))
	starter := apiconv.NewMessage()
	starter.SetId("ordinary-starter")
	starter.SetConversationID("legacy-conversation")
	starter.SetTurnID("ordinary-turn")
	starter.SetRole("user")
	starter.SetType("text")
	starter.SetContent("Ordinary fixture request")
	require.NoError(t, rt.Conversation.PatchMessage(ctx, starter))
	currentRun := &runmodel.MutableRunView{}
	currentRun.SetId("ordinary-turn")
	currentRun.SetTurnID("ordinary-turn")
	currentRun.SetConversationID("legacy-conversation")
	currentRun.SetEffectiveUserID("fixture-owner")
	currentRun.SetStatus("running")
	currentRun.SetLeaseOwner("ordinary-lease")
	currentRun.SetLeaseUntil(time.Now().Add(time.Minute))
	_, err = rt.Data.PatchRuns(ctx, []*runmodel.MutableRunView{currentRun})
	require.NoError(t, err)
	pending, err := factory.Capture(ctx, evidence.Input{ReceivedAt: time.Now()})
	require.NoError(t, err)
	scopedCtx := requestctx.WithTurnMeta(ctx, requestctx.TurnMeta{ConversationID: "legacy-conversation", TurnID: "ordinary-turn"})
	ordinaryCtx, err := pending.Begin(scopedCtx, evidence.Turn{ConversationID: "legacy-conversation", TurnID: "ordinary-turn", StarterMessageID: "ordinary-starter", LeaseOwner: func() string { return "ordinary-lease" }})
	require.NoError(t, err)
	require.NotNil(t, evidence.PublicationFromContext(ordinaryCtx))
	ordinaryReportRoundTrip(t, ordinaryCtx, rt, "legacy-conversation", "ordinary-report")
	restartedStore := fb.NewNativeSourceStore(rt.Conversation, rt.Data, documents.New(rt.Native))
	restartedFactory, err := fb.NewFactory(fb.ProjectionPolicyProducer{}, restartedStore, "Asia/Tokyo", fb.WithRolloutCutoff(cfg.cutoff))
	require.NoError(t, err)
	resumed, err := restartedFactory.Restore(scopedCtx, evidence.Turn{ConversationID: "legacy-conversation", TurnID: "ordinary-turn", LeaseOwner: func() string { return "ordinary-lease" }})
	require.NoError(t, err)
	require.NotNil(t, evidence.PublicationFromContext(resumed))
	// Remove opt-in and construct fresh services on the same native authority.
	// No new capture or catalog preflight is allowed; recorded admission remains
	// strict and ordinary unadmitted continuation retains report functionality.
	disabledReportStore := reportmemory.New()
	disabledRuns, ok := disabledReportStore.(reportstore.RunClient)
	require.True(t, ok)
	disabledRt := &executor.Runtime{Native: rt.Native, Conversation: rt.Conversation, Data: rt.Data, Agent: agent.New(nil, nil, nil, nil, nil, rt.Conversation), Reporting: reporting.New(reporting.Options{Store: reporting.NewStoreAdapter(disabledReportStore)}), ReportRuns: reportingrun.New(reportingrun.Options{Store: disabledRuns})}
	disabledFactory, err := installForecastEvidence(disabledRt, ForecastEvidenceConfig{})
	require.NoError(t, err)
	pendingDisabled, err := disabledFactory.Capture(ctx, evidence.Input{ReceivedAt: time.Now(), Context: json.RawMessage(`{"client":{"forecastIntent":{"version":99}}}`)})
	require.NoError(t, err)
	require.Nil(t, pendingDisabled)
	strictDisabled, err := disabledFactory.Restore(scopedCtx, evidence.Turn{ConversationID: "legacy-conversation", TurnID: "ordinary-turn", LeaseOwner: func() string { return "ordinary-lease" }})
	require.NoError(t, err)
	require.NotNil(t, evidence.PublicationFromContext(strictDisabled))
	noAdmission, err := disabledFactory.Restore(ctx, evidence.Turn{ConversationID: "legacy-conversation", TurnID: "legacy-turn", LeaseOwner: func() string { return "lease" }})
	require.NoError(t, err)
	require.Nil(t, evidence.PublicationFromContext(noAdmission))
	ordinaryReportRoundTrip(t, noAdmission, disabledRt, "legacy-conversation", "disabled-ordinary-report")
	ordinaryAfterCutoff, err := disabledFactory.Restore(ctx, evidence.Turn{ConversationID: "after-cutoff", TurnID: "after-cutoff", LeaseOwner: func() string { return "lease" }})
	require.NoError(t, err)
	require.Nil(t, evidence.PublicationFromContext(ordinaryAfterCutoff))
	_, err = disabledFactory.Restore(ctx, evidence.Turn{ConversationID: "foreign-owner", TurnID: "foreign-owner", LeaseOwner: func() string { return "lease" }})
	require.Error(t, err, "disabled state cannot bypass native ownership")

	_, err = disabledRt.ReportRuns.Begin(ctx, &reportingrun.BeginInput{UIRunRequestID: "old-command", ConversationID: "legacy-conversation", BuilderRef: "builder", ReportAdmissionRef: "bad-reference"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid command reference")

	require.Error(t, ConfigureForecastEvidence(ctx, rt, root), "startup cannot replace command authority")
}

func ordinaryReportRoundTrip(t *testing.T, ctx context.Context, rt *executor.Runtime, conversation, id string) {
	t.Helper()
	begun, err := rt.ReportRuns.Begin(ctx, &reportingrun.BeginInput{UIRunRequestID: id, ConversationID: conversation, Origin: "manual", BuilderRef: "ordinary"})
	require.NoError(t, err)
	start, _ := json.Marshal(map[string]interface{}{"version": 1, "id": id, "sequence": 1, "mode": "start", "grammar": "report-document-v1", "title": "Ordinary report", "blocks": []interface{}{map[string]interface{}{"id": "note", "kind": "markdownBlock", "markdown": "Unrelated report content"}}})
	commit, _ := json.Marshal(map[string]interface{}{"version": 1, "id": id, "sequence": 2, "mode": "commit"})
	content := "```forge-report\n" + string(start) + "\n```\n```forge-report\n" + string(commit) + "\n```"
	compiled, err := rt.Reporting.CompileFencedReport(ctx, &reporting.CompileFencedReportRequest{ReportID: id, Content: content})
	require.NoError(t, err)
	completed, err := rt.ReportRuns.Complete(ctx, &reportingrun.CompleteInput{ReportRunID: begun.Run.ReportRunID, ConversationID: conversation, ExpectedRevision: begun.Run.Revision, ReportSpec: compiled.ReportSpec, ReportFill: compiled.ReportFill, ReportPrint: compiled.ReportPrint})
	require.NoError(t, err)
	require.Equal(t, "completed", string(completed.Status))
}
