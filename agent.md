# Development Rules

- 每次代码修改必须先从最新的 `main` branch 创建一条干净的新 branch；所有代码改动都只能在该 branch 上完成。改动完成后 push 到 remote，等待对应环境部署完成，并在真实环境上完成验收。验收通过后再创建 PR 到 `main`；PR 合并进 `main` 后，必须再次在生产环境上验收。
- 不同 conversation 的代码修改不允许复用同一个 branch；每个 conversation 必须使用独立 branch，避免互相覆盖或混入上下文。
