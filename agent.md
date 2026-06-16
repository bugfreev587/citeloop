# Development Rules

1. Before any code change, checkout a clean branch from the latest `main` branch and make all code changes on that branch.
2. After the code change is complete, merge the work to the main branch, create a PR to `origin/main`, and merge the PR.
3. Wait for deployment to finish, then verify the code change in the production environment.
4. If there is any gap between production behavior and the expectation, fix it, push the fix, and verify again.
5. Report the finish status with the PR link only after all verification has passed.
