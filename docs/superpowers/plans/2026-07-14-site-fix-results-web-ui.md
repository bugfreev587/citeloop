# Site Fix Results Web UI Implementation Plan

**Goal:** Show real Site Fix measurements in Results while keeping the Doctor repair lifecycle visibly complete at Verified.

**Architecture:** Normalize the Results API as a discriminated union. Keep existing Content Action rendering paths intact, add dedicated Site Fix summary/detail rendering, and use the measurement ID deep link to focus the correct Results card. Site Fix pages render measurement policy and handoff state without adding a measuring lifecycle milestone.

### Task 1: API types and normalization

- [ ] Add typed Site Fix classification, measurement summary, checkpoint, terminal, and Results detail contracts.
- [ ] Add `ResultsFeedItem = ResultsContentAction | ResultsSiteFixSummary` with strict `source_type` narrowing.
- [ ] Normalize Site Fix detail/list fields and expose the project-scoped measurement detail API.
- [ ] Add contract tests for unknown-safe defaults, legacy Content Action compatibility, and Site Fix discrimination.

### Task 2: Site Fix lifecycle presentation

- [ ] Render Verification only, Measurement pending/started/failed, and Results-link states from the server summary.
- [ ] Keep the four existing repair milestones unchanged and keep Verified as the terminal Site Fix state.
- [ ] Show prospective/low-confidence wording without implying directional attribution.
- [ ] Add contract tests for policy and handoff states.

### Task 3: Results cards, drawer, and deep links

- [ ] Render Content Action and Site Fix cards from the discriminated union with an explicit source badge.
- [ ] Fetch the redacted Site Fix measurement detail only when its card opens.
- [ ] Render independent measurement status, outcome taxonomy, checkpoints, and prospective warning.
- [ ] Support `/results?source_type=site_fix&measurement={id}` focus/open behavior while preserving `?action=` and `?article=`.
- [ ] Add accessible keyboard/focus and contract coverage.

### Task 4: Verification

- [ ] Run the React best-practices checklist on the edited TSX files.
- [ ] Run `npm test`, `npm run typecheck`, and `npm run build`.
- [ ] Commit the frontend slice.
