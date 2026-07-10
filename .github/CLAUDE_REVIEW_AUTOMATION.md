# Claude Review Automation

This repository runs three Claude review workflows:

- `claude-review.yml` reviews every non-draft same-repository PR for code and behavior risks.
- `claude-prd-review.yml` reviews PRD changes under `docs/`.
- `claude-review-fix.yml` runs after either review workflow succeeds, applies actionable Claude review comments, and dispatches another review pass when it pushes fixes.

## Credentials And Model Route

The GitHub reviewer is not a human Claude web account. It is `anthropics/claude-code-action@v1` running in GitHub Actions with:

- `TOKENGATE_CLAUDE_CODE_API_KEY` as the API key secret.
- `ANTHROPIC_BASE_URL=https://gateway.mytokengate.com`.

The actual model is controlled by the TokenGate route or by Claude Code action arguments if a model argument is added. If PRD review must use a specific high-reasoning model such as Opus 4.8, configure the TokenGate route or add an explicit supported Claude Code model argument and verify it in the workflow logs.

## Source-Grounded PRD Review

`claude-prd-review.yml` must behave like a real Claude Code review, not a diff-only wording check:

- It checks out full repository history with `fetch-depth: 0`.
- It allows read/search tools (`Read`, `Grep`, `Glob`, `LS`, `rg`, `sed`, `nl`, `git diff`, and `git show`) so Claude can inspect code, migrations, API handlers, UI state, tests, and workflows behind the PRD claims.
- The prompt requires every actionable finding to cite PRD context plus source evidence.
- The prompt distinguishes supported UI/API options from backend-emitted behavior.
- The workflow requires a top-level comment containing a run-specific marker like `<!-- claude-prd-review-evidence:<github-run-id> -->`.

If the current run's evidence summary comment is missing, the workflow fails. A successful PRD review should therefore leave an auditable trail of what Claude inspected, even when it found no source-grounded issues.

PRs that change `.github/workflows/claude-prd-review.yml` itself are a special case: Claude Code Action validates that the workflow file matches the default branch before it runs. Those self-change PRs skip the evidence gate and leave a GitHub comment explaining that the new review behavior applies after the workflow change is merged.

## Fixer Boundary

The current automated fixer is also Claude Code, not Codex. The workflow name and comments intentionally say `Claude Review Fixer`.

If the desired loop is "Claude Code reviews, Codex fixes, then Claude Code re-reviews", add a separate Codex fixer workflow that:

- Runs only after `Claude PR Review` or `Claude PRD Review` completes with a source-grounded evidence comment.
- Reads GitHub inline comments, review comments, and top-level evidence summaries.
- Applies only comments that are actionable and source-grounded.
- Commits to the PR branch with a Codex identity.
- Dispatches the Claude review workflows again after pushing.
- Stops after a small fixed loop count, matching the current two-pass guard.

Until that workflow exists, review-fix automation in this repository should be described as Claude-to-Claude, not Claude-to-Codex.

The fixer is intentionally bounded:

- It only writes to non-draft PRs whose head branch is in this repository.
- It counts `<!-- claude-review-fix-loop -->` marker comments and stops after two automated passes by default.
- It never merges or closes PRs.

`claude-review-fix.yml` can use the default `GITHUB_TOKEN`. If you want fixer pushes to behave like a normal user push in every GitHub integration, add a repository secret named `REVIEW_FIXER_GITHUB_TOKEN` with contents and pull request write access. The workflow falls back to `GITHUB_TOKEN` when that secret is absent.
