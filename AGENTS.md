# Development Rules

These rules are mandatory for all Codex work in this repository.

Before making any code changes, Codex must read and follow these rules. If a rule cannot be completed because of missing access, permissions, deployment state, or user instruction conflict, Codex must stop and report the blocker before continuing.

1. For every new Codex session or task that may modify code, create a fresh isolated git worktree from the latest `origin/main` before making changes.
2. Do not modify code in the primary checkout when it has unrelated or uncommitted work. Keep each Codex task on its own branch and worktree.
3. If the task is only reading, explaining, searching, or reviewing without file changes, a new worktree is not required.
4. If the user explicitly asks to continue an existing branch, worktree, PR, or uncommitted local change set, reuse that context instead of creating another worktree.
5. Before any code change, checkout a clean branch from the latest `main` branch and make all code changes on that branch.
6. After the code change is complete, merge the work to the main branch, create a PR to `origin/main`, and merge the PR.
7. Wait for deployment to finish, then verify the code change in the production environment.
8. If there is any gap between production behavior and the expectation, fix it, push the fix, and verify again.
9. Report the finish status with the PR link only after all verification has passed.
