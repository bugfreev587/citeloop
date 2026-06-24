# Analysis Workflow Phase 6 Permission Guidance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development for UI behavior contracts and superpowers:executing-plans for task tracking.

**Goal:** Make Settings answer "what do I do now?" for users who only have a domain, do not yet have Search Console set up, or use a CMS publisher that CiteLoop cannot publish to directly yet.

**Scope:** Add honest self-serve guidance and roadmap affordances. Do not fake WordPress/CMS OAuth connectors or claim live publishing support before the backend connector exists.

---

## File Structure

- Modify: `web/app/lib/dashboard-ux-phase1-contract.test.mjs`
  - Contract-test GSC setup guidance and CMS connector roadmap copy.
- Modify: `web/app/projects/[id]/settings/settings-client.tsx`
  - Add a compact Search Console setup guide.
  - Add a publisher connector roadmap section for WordPress/CMS platforms.

## Task 1: Write UI Contract RED Test

- [x] Require Settings to include:
  - `Set up Search Console property`
  - `Open Search Console`
  - `Verify DNS ownership`
  - `Connect after verification`
- [x] Require Publisher settings to include:
  - `WordPress`
  - `CMS connector roadmap`
  - `Draft-only until OAuth connector is ready`
- [x] Verify RED against the current Settings page.

## Task 2: Add Search Console Setup Guidance

- [x] Show a compact setup guide when there is no connection, no selected property, or no authorized properties.
- [x] Explain the Domain property and DNS TXT ownership step without exposing raw Google API details.
- [x] Link directly to Google Search Console.
- [x] Keep OAuth connection as the next action after verification.

## Task 3: Add CMS Connector Roadmap

- [x] Keep GitHub/Next.js as the only live publisher configuration.
- [x] Show WordPress, Webflow, Shopify, and Custom CMS as roadmap connectors.
- [x] Mark them draft-only until OAuth connector support is ready.
- [x] Avoid fake buttons that imply live OAuth publishing.

## Task 4: Verify

- [x] Run targeted dashboard UX contract test.
- [x] Run full web tests.
- [x] Run frontend typecheck.
- [x] Run full Go test suite through `make test`.
- [x] Run preview production build.
- [x] Run whitespace diff check.
