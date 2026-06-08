// Package buildinfo centralizes non-secret deployment metadata used by health
// checks and UI diagnostics.
package buildinfo

import "os"

type Info struct {
	Service      string `json:"service"`
	CommitSHA    string `json:"commit_sha,omitempty"`
	CommitRef    string `json:"commit_ref,omitempty"`
	DeploymentID string `json:"deployment_id,omitempty"`
	Environment  string `json:"environment,omitempty"`
	URL          string `json:"url,omitempty"`
}

func Current(service string) Info {
	return Info{
		Service: service,
		CommitSHA: firstEnv(
			"CITELOOP_GIT_SHA",
			"VERCEL_GIT_COMMIT_SHA",
			"RAILWAY_GIT_COMMIT_SHA",
			"RENDER_GIT_COMMIT",
			"COMMIT_SHA",
			"GIT_SHA",
			"SOURCE_VERSION",
		),
		CommitRef: firstEnv(
			"CITELOOP_GIT_REF",
			"VERCEL_GIT_COMMIT_REF",
			"RAILWAY_GIT_BRANCH",
			"BRANCH_NAME",
			"GIT_BRANCH",
		),
		DeploymentID: firstEnv(
			"CITELOOP_DEPLOYMENT_ID",
			"VERCEL_DEPLOYMENT_ID",
			"RAILWAY_DEPLOYMENT_ID",
			"RENDER_SERVICE_ID",
		),
		Environment: firstEnv(
			"CITELOOP_ENV",
			"VERCEL_ENV",
			"RAILWAY_ENVIRONMENT_NAME",
			"NODE_ENV",
			"GO_ENV",
		),
		URL: firstEnv(
			"CITELOOP_URL",
			"VERCEL_URL",
			"RAILWAY_PUBLIC_DOMAIN",
			"RENDER_EXTERNAL_URL",
		),
	}
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}
