# Claude Review Automation

This repository runs three Claude review workflows:

- `claude-review.yml` reviews every non-draft same-repository PR for code and behavior risks.
- `claude-prd-review.yml` reviews PRD changes under `docs/`.
- `claude-review-fix.yml` runs after either review workflow succeeds, applies actionable Claude review comments, and dispatches another review pass when it pushes fixes.

The fixer is intentionally bounded:

- It only writes to non-draft PRs whose head branch is in this repository.
- It counts `<!-- claude-review-fix-loop -->` marker comments and stops after two automated passes by default.
- It never merges or closes PRs.

`claude-review-fix.yml` can use the default `GITHUB_TOKEN`. If you want fixer pushes to behave like a normal user push in every GitHub integration, add a repository secret named `REVIEW_FIXER_GITHUB_TOKEN` with contents and pull request write access. The workflow falls back to `GITHUB_TOKEN` when that secret is absent.
