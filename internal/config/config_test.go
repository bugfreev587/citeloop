package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNextPublishSlotStaggersAndModes(t *testing.T) {
	now := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)

	scheduled := Default()
	scheduled.PublishMode = PublishModeScheduled
	if got, ok := scheduled.NextPublishSlot(time.Time{}, now); !ok || !got.Equal(now) {
		t.Fatalf("scheduled first = %v ok=%v, want %v", got, ok, now)
	}
	if got, ok := scheduled.NextPublishSlot(now, now); !ok || !got.Equal(now.AddDate(0, 0, 2)) {
		t.Fatalf("scheduled next = %v ok=%v, want %v", got, ok, now.AddDate(0, 0, 2))
	}

	legacyAuto := Default()
	legacyAuto.PublishMode = PublishModeAuto
	legacyAuto.PublishIntervalDays = 1
	if got, ok := legacyAuto.NextPublishSlot(now, now); !ok || !got.Equal(now.AddDate(0, 0, 1)) {
		t.Fatalf("legacy auto = %v ok=%v, want scheduled next day", got, ok)
	}

	manual := Default()
	manual.PublishMode = PublishModeManual
	if _, ok := manual.NextPublishSlot(time.Time{}, now); ok {
		t.Fatal("manual should not auto-schedule")
	}
}

func TestParseDefaults(t *testing.T) {
	c, err := Parse(json.RawMessage("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if c.BufferDays != 5 || c.Crawl.MaxPages != 200 {
		t.Fatalf("defaults not applied: %+v", c)
	}
	if c.AutoAdvanceEnabled {
		t.Fatal("auto_advance_enabled should default to false")
	}
	if c.PublishMode != PublishModeManual {
		t.Fatalf("publish_mode default = %q, want manual", c.PublishMode)
	}
	if _, ok := c.NextPublishSlot(time.Time{}, time.Now()); ok {
		t.Fatal("default config must not schedule publishing automatically")
	}
	if c.OpportunityFindingSourceMix != OpportunityFindingSourceAll {
		t.Fatalf("opportunity_finding_source_mix default = %q, want all", c.OpportunityFindingSourceMix)
	}
	if c.AIDiscoveryAutomation != AIDiscoveryAutomationSemiAutomatic {
		t.Fatalf("ai_discovery_automation default = %q, want semi_automatic", c.AIDiscoveryAutomation)
	}
}

func TestParseKeepsExplicitPublishModes(t *testing.T) {
	for _, mode := range []string{PublishModeScheduled, PublishModeManual} {
		c, err := Parse(json.RawMessage(`{"publish_mode":"` + mode + `"}`))
		if err != nil {
			t.Fatal(err)
		}
		if c.PublishMode != mode {
			t.Fatalf("publish_mode = %q, want %q", c.PublishMode, mode)
		}
	}
}

func TestParseNormalizesLegacyAutoPublishModeToScheduled(t *testing.T) {
	c, err := Parse(json.RawMessage(`{"publish_mode":"auto","publish_interval_days":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.PublishMode != PublishModeScheduled {
		t.Fatalf("legacy auto normalized to %q, want scheduled", c.PublishMode)
	}
	if c.PublishIntervalDays != 5 {
		t.Fatalf("publish_interval_days = %d, want preserved 5", c.PublishIntervalDays)
	}
}

func TestParseNormalizesLegacyAutoPublishModeWithInvalidInterval(t *testing.T) {
	c, err := Parse(json.RawMessage(`{"publish_mode":"auto","publish_interval_days":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.PublishMode != PublishModeScheduled {
		t.Fatalf("legacy auto normalized to %q, want scheduled", c.PublishMode)
	}
	if c.PublishIntervalDays != 1 {
		t.Fatalf("legacy auto interval = %d, want 1", c.PublishIntervalDays)
	}
}

func TestParseKeepsExplicitOpportunityFindingSettings(t *testing.T) {
	for _, mode := range []string{OpportunityFindingSourceAll, OpportunityFindingSourceSignalScan, OpportunityFindingSourceAIDiscovery} {
		c, err := Parse(json.RawMessage(`{"opportunity_finding_source_mix":"` + mode + `"}`))
		if err != nil {
			t.Fatal(err)
		}
		if c.OpportunityFindingSourceMix != mode {
			t.Fatalf("opportunity_finding_source_mix = %q, want %q", c.OpportunityFindingSourceMix, mode)
		}
	}
	for _, automation := range []string{AIDiscoveryAutomationAutomatic, AIDiscoveryAutomationSemiAutomatic, AIDiscoveryAutomationManual} {
		c, err := Parse(json.RawMessage(`{"ai_discovery_automation":"` + automation + `"}`))
		if err != nil {
			t.Fatal(err)
		}
		if c.AIDiscoveryAutomation != automation {
			t.Fatalf("ai_discovery_automation = %q, want %q", c.AIDiscoveryAutomation, automation)
		}
	}
}

func TestOpportunityFindingStagesRespectAutomaticEligibility(t *testing.T) {
	cfg := Default()
	stages := cfg.OpportunityFindingStages(true)
	if !stages.SignalScan {
		t.Fatal("default automatic run should keep deterministic Signal Scan scheduled")
	}
	if stages.AIDiscovery {
		t.Fatal("default semi-automatic AI Discovery should not spend provider tokens on the daily automatic tick")
	}

	cfg.AIDiscoveryAutomation = AIDiscoveryAutomationAutomatic
	stages = cfg.OpportunityFindingStages(true)
	if !stages.SignalScan || !stages.AIDiscovery {
		t.Fatalf("automatic all-mode stages = %+v, want Signal Scan and AI Discovery", stages)
	}

	cfg.OpportunityFindingSourceMix = OpportunityFindingSourceAIDiscovery
	stages = cfg.OpportunityFindingStages(true)
	if stages.SignalScan || !stages.AIDiscovery {
		t.Fatalf("automatic AI-only stages = %+v, want AI Discovery only", stages)
	}

	cfg.OpportunityFindingSourceMix = OpportunityFindingSourceSignalScan
	stages = cfg.OpportunityFindingStages(true)
	if !stages.SignalScan || stages.AIDiscovery {
		t.Fatalf("signal-scan stages = %+v, want Signal Scan only", stages)
	}
}

func TestOpportunityFindingStagesManualRunTriggersEnabledAIDiscovery(t *testing.T) {
	cfg := Default()
	cfg.AIDiscoveryAutomation = AIDiscoveryAutomationManual
	stages := cfg.OpportunityFindingStages(false)
	if !stages.SignalScan || !stages.AIDiscovery {
		t.Fatalf("manual all-mode run stages = %+v, want Signal Scan and AI Discovery", stages)
	}

	cfg.AIDiscoveryAutomation = AIDiscoveryAutomationSemiAutomatic
	cfg.OpportunityFindingSourceMix = OpportunityFindingSourceAIDiscovery
	stages = cfg.OpportunityFindingStages(false)
	if stages.SignalScan || !stages.AIDiscovery {
		t.Fatalf("manual semi-automatic AI-only run stages = %+v, want AI Discovery only", stages)
	}
}

func TestParseNormalizesInvalidOpportunityFindingSettings(t *testing.T) {
	c, err := Parse(json.RawMessage(`{"opportunity_finding_source_mix":"unknown","ai_discovery_automation":"always"}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.OpportunityFindingSourceMix != OpportunityFindingSourceAll {
		t.Fatalf("invalid opportunity_finding_source_mix normalized to %q, want all", c.OpportunityFindingSourceMix)
	}
	if c.AIDiscoveryAutomation != AIDiscoveryAutomationSemiAutomatic {
		t.Fatalf("invalid ai_discovery_automation normalized to %q, want semi_automatic", c.AIDiscoveryAutomation)
	}
}

// Regression: an explicit buffer_days:0 must be honored, not coerced to default.
func TestParseExplicitZeroBuffer(t *testing.T) {
	c, err := Parse(json.RawMessage(`{"buffer_days":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.BufferDays != 0 {
		t.Fatalf("buffer_days:0 was coerced to %d", c.BufferDays)
	}
	// absent crawl bounds still keep defaults
	if c.Crawl.MaxPages != 200 {
		t.Fatalf("absent crawl.max_pages lost its default: %d", c.Crawl.MaxPages)
	}
}

// Partial nested config must preserve sibling defaults.
func TestParsePartialCrawl(t *testing.T) {
	c, err := Parse(json.RawMessage(`{"crawl":{"max_pages":50}}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Crawl.MaxPages != 50 {
		t.Fatalf("explicit max_pages lost: %d", c.Crawl.MaxPages)
	}
	if c.Crawl.MaxDepth != 3 {
		t.Fatalf("sibling crawl.max_depth default lost: %d", c.Crawl.MaxDepth)
	}
}

func TestParseExplicitAutoAdvanceDisabled(t *testing.T) {
	c, err := Parse(json.RawMessage(`{"auto_advance_enabled":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.AutoAdvanceEnabled {
		t.Fatal("auto_advance_enabled:false should disable workflow advancement")
	}
}

func TestParseExplicitAutoAdvanceEnabled(t *testing.T) {
	c, err := Parse(json.RawMessage(`{"auto_advance_enabled":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !c.AutoAdvanceEnabled {
		t.Fatal("auto_advance_enabled:true should enable workflow advancement")
	}
}

func TestFromEnvReadsTokenGateDefaults(t *testing.T) {
	t.Setenv("TOKENGATE_API_KEY", "tg-test-key")
	t.Setenv("TOKENGATE_BASE_URL", "")
	t.Setenv("TOKENGATE_MODEL", "")

	env := FromEnv()
	if env.TokenGateAPIKey != "tg-test-key" {
		t.Fatalf("TokenGateAPIKey = %q", env.TokenGateAPIKey)
	}
	if env.TokenGateBaseURL != "https://tokengate-production.up.railway.app/v1" {
		t.Fatalf("TokenGateBaseURL = %q", env.TokenGateBaseURL)
	}
	if env.TokenGateModel != "claude-sonnet-4-6" {
		t.Fatalf("TokenGateModel = %q", env.TokenGateModel)
	}
}

func TestFromEnvReadsClerkSecretKeyAndAdmins(t *testing.T) {
	t.Setenv("CLERK_SECRET_KEY", "sk_test_clerk")
	t.Setenv("ADMINS", "owner@example.com,admin@example.com")

	env := FromEnv()
	if env.ClerkSecretKey != "sk_test_clerk" {
		t.Fatalf("ClerkSecretKey = %q", env.ClerkSecretKey)
	}
	if env.AdminEmails != "owner@example.com,admin@example.com" {
		t.Fatalf("AdminEmails = %q", env.AdminEmails)
	}
}

func TestFromEnvReadsRailwayEnvironmentName(t *testing.T) {
	t.Setenv("RAILWAY_ENVIRONMENT_NAME", "production")
	t.Setenv("RAILWAY_ENVIRONMENT", "")

	env := FromEnv()
	if env.Environment != "production" {
		t.Fatalf("Environment = %q, want production", env.Environment)
	}
}

func TestFromEnvReadsBlogContentDirDefaultAndOverride(t *testing.T) {
	t.Setenv("BLOG_CONTENT_DIR", "")
	env := FromEnv()
	if env.BlogContentDir != "content/citeloop/blog" {
		t.Fatalf("default BlogContentDir = %q", env.BlogContentDir)
	}

	t.Setenv("BLOG_CONTENT_DIR", "custom/generated")
	env = FromEnv()
	if env.BlogContentDir != "custom/generated" {
		t.Fatalf("override BlogContentDir = %q", env.BlogContentDir)
	}
}

func TestFromEnvReadsUniPostDeployHookURL(t *testing.T) {
	t.Setenv("UNIPOST_DEPLOY_HOOK_URL", "https://api.vercel.com/v1/integrations/deploy/example")

	env := FromEnv()
	if env.UniPostDeployHookURL != "https://api.vercel.com/v1/integrations/deploy/example" {
		t.Fatalf("UniPostDeployHookURL = %q", env.UniPostDeployHookURL)
	}
}

func TestFromEnvReadsNotificationSecretKey(t *testing.T) {
	t.Setenv("NOTIFICATION_SECRET_KEY", "notification-secret")

	env := FromEnv()
	if env.NotificationSecretKey != "notification-secret" {
		t.Fatalf("NotificationSecretKey = %q", env.NotificationSecretKey)
	}
}

func TestFromEnvReadsResendNotificationConfig(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "resend-key")
	t.Setenv("NOTIFICATION_EMAIL_FROM", "CiteLoop <notifications@citeloop.app>")
	t.Setenv("NOTIFICATION_EMAIL_REPLY_TO", "support@citeloop.app")

	env := FromEnv()
	if env.ResendAPIKey != "resend-key" {
		t.Fatalf("ResendAPIKey = %q", env.ResendAPIKey)
	}
	if env.NotificationEmailFrom != "CiteLoop <notifications@citeloop.app>" {
		t.Fatalf("NotificationEmailFrom = %q", env.NotificationEmailFrom)
	}
	if env.NotificationEmailReplyTo != "support@citeloop.app" {
		t.Fatalf("NotificationEmailReplyTo = %q", env.NotificationEmailReplyTo)
	}
}

func TestFromEnvReadsGoogleOAuthConfig(t *testing.T) {
	t.Setenv("GOOGLE_OAUTH_CLIENT_ID", "google-client-id")
	t.Setenv("GOOGLE_OAUTH_CLIENT_SECRET", "google-client-secret")
	t.Setenv("PUBLIC_APP_URL", "https://app.citeloop.test")

	env := FromEnv()
	if env.GoogleOAuthClientID != "google-client-id" {
		t.Fatalf("GoogleOAuthClientID = %q", env.GoogleOAuthClientID)
	}
	if env.GoogleOAuthClientSecret != "google-client-secret" {
		t.Fatalf("GoogleOAuthClientSecret = %q", env.GoogleOAuthClientSecret)
	}
	if env.PublicAppURL != "https://app.citeloop.test" {
		t.Fatalf("PublicAppURL = %q", env.PublicAppURL)
	}
}

func TestFromEnvReadsGEOProviderConfig(t *testing.T) {
	t.Setenv("PERPLEXITY_API_KEY", "pplx-test")
	t.Setenv("PERPLEXITY_BASE_URL", "")
	t.Setenv("PERPLEXITY_MODEL", "")
	t.Setenv("GEO_PROVIDER_RUN_BUDGET_USD", "")

	env := FromEnv()
	if env.PerplexityAPIKey != "pplx-test" {
		t.Fatalf("PerplexityAPIKey = %q", env.PerplexityAPIKey)
	}
	if env.PerplexityBaseURL != "https://api.perplexity.ai" {
		t.Fatalf("PerplexityBaseURL = %q", env.PerplexityBaseURL)
	}
	if env.PerplexityModel != "sonar-pro" {
		t.Fatalf("PerplexityModel = %q", env.PerplexityModel)
	}
	if env.GEOProviderRunBudgetUSD != 1 {
		t.Fatalf("GEOProviderRunBudgetUSD = %f, want 1", env.GEOProviderRunBudgetUSD)
	}
}

func TestEnvExampleDocumentsGEOProviderConfig(t *testing.T) {
	raw, err := os.ReadFile("../../.env.example")
	if err != nil {
		t.Fatalf("read .env.example: %v", err)
	}
	body := string(raw)
	for _, want := range []string{
		"PERPLEXITY_API_KEY=",
		"PERPLEXITY_BASE_URL=https://api.perplexity.ai",
		"PERPLEXITY_MODEL=sonar-pro",
		"GEO_PROVIDER_RUN_BUDGET_USD=1",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf(".env.example missing %q", want)
		}
	}
}
