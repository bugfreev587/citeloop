// Command api is the CiteLoop service entrypoint: runs migrations, wires
// TokenGate or mock providers, starts the scheduler cron, and serves the HTTP
// API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/citeloop/citeloop/internal/admin"
	"github.com/citeloop/citeloop/internal/api"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/geo"
	"github.com/citeloop/citeloop/internal/githubapp"
	"github.com/citeloop/citeloop/internal/googledata"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/scheduler"
	"github.com/citeloop/citeloop/internal/search"
	"github.com/citeloop/citeloop/internal/seed"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	env := config.FromEnv()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, env.DatabaseURL)
	if err != nil {
		log.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pingWithRetry(ctx, pool, log); err != nil {
		log.Error("db unreachable", "err", err)
		os.Exit(1)
	}
	if err := runMigrations(ctx, pool, log); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}

	q := db.New(pool)
	if shouldSeedPlaceholder(env) {
		if p, err := seed.EnsurePlaceholder(ctx, q); err != nil {
			log.Warn("seed placeholder failed", "err", err)
		} else {
			log.Info("placeholder project ready", "id", p.ID, "slug", p.Slug)
		}
	} else {
		log.Info("placeholder project seed skipped", "environment", env.Environment, "clerk_configured", env.ClerkSecretKey != "")
	}

	// Providers: real when keyed, deterministic mock otherwise (still runs).
	llmP := admin.NewRuntimeProvider(pool, env, selectLLMProvider(env, log))
	var searchP search.Provider = search.NewBrave(env.SearchAPIKey)
	if env.SearchAPIKey == "" {
		log.Warn("SEARCH_API_KEY not set — using mock search provider")
		searchP = search.NewMock()
	}
	blog := publisher.NewBlog(env.GitHubToken, env.BlogRepo, env.BlogBranch, env.BlogBaseURL, env.BlogContentDir, log)
	var seoData seopkg.GoogleDataProvider
	if env.GoogleServiceAccountJSON != "" {
		provider, err := googledata.NewServiceAccountClient(ctx, env.GoogleServiceAccountJSON)
		if err != nil {
			log.Warn("Google data provider disabled", "err", err)
		} else {
			log.Info("Google data provider ready")
			seoData = provider
		}
	}

	sched := scheduler.New(pool, llmP, searchP, blog, log)
	sched.BlogBaseURL = env.BlogBaseURL
	sched.SEOData = seoData
	sched.GEOProviderRunBudgetUSD = env.GEOProviderRunBudgetUSD
	if env.PerplexityAPIKey != "" {
		sched.GEOAnswerProvider = geo.NewPerplexityProvider(env.PerplexityAPIKey, env.PerplexityBaseURL, env.PerplexityModel, nil)
	}
	sched.NotificationSecret = env.NotificationSecretKey
	sched.UniPostDeployHookURL = env.UniPostDeployHookURL
	sched.GitHubApp = githubapp.New(githubapp.Config{
		AppID:         env.GitHubAppID,
		Slug:          env.GitHubAppSlug,
		ClientID:      env.GitHubAppClientID,
		ClientSecret:  env.GitHubAppClientSecret,
		PrivateKeyPEM: env.GitHubAppPrivateKey,
	})
	cron := sched.Start(ctx)
	defer cron.Stop()

	srv := &api.Server{
		Pool: pool, Q: q, LLM: llmP, Search: searchP, Blog: blog, Sched: sched, Env: env, Log: log, SEOData: seoData,
	}
	httpServer := &http.Server{
		Addr:              ":" + env.Port,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

func shouldSeedPlaceholder(env config.Env) bool {
	if env.ClerkSecretKey != "" {
		return false
	}
	return env.Environment != "production"
}

func selectLLMProvider(env config.Env, log *slog.Logger) llm.Provider {
	if env.TokenGateAPIKey != "" {
		log.Info("using TokenGate OpenAI-compatible LLM provider", "base_url", env.TokenGateBaseURL, "model", env.TokenGateModel)
		return llm.NewOpenAIChat(env.TokenGateAPIKey, env.TokenGateBaseURL, env.TokenGateModel)
	}
	log.Warn("TOKENGATE_API_KEY not set — using mock as the runtime fallback; admin-saved TokenGate credentials override this at request time")
	return llm.NewMock()
}

func pingWithRetry(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	var err error
	for i := 0; i < 10; i++ {
		if err = pool.Ping(ctx); err == nil {
			return nil
		}
		log.Info("waiting for database…", "attempt", i+1)
		time.Sleep(time.Second)
	}
	return err
}
