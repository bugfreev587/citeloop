# Remove Manual Content Brief Creation Design

## Problem

Content Plan currently exposes a `New Content Brief` form that lets an operator
create a Topic directly. The resulting Topic has no `source_content_action_id`,
so it bypasses the product's AI discovery and decision flow and appears beside
accepted work without Doctor or Opportunity provenance.

This conflicts with the simplified product model:

```text
Doctor
  -> AI finding
  -> Site Fix
  -> Apply and verify

Opportunities
  -> AI growth opportunity
  -> accepted content work
  -> Content Plan
  -> Review, Publish, and Measure
```

Doctor and Opportunities are the only user-visible sources of new work. Content
Plan operates accepted content work; it does not create demand or provide an
alternate manual intake path.

## Considered Approaches

### 1. Hide only the Content Plan form

This is the smallest UI change, but it leaves the frontend API client and
`POST /projects/{id}/topics` capable of creating source-less Topics. The product
would appear simpler while the unsupported workflow remained available.

Rejected because it does not enforce the source boundary.

### 2. Remove the UI and manual HTTP write path

Remove the form, its frontend mutation, and the project-scoped Topic creation
route. Preserve Topic reads and lifecycle mutations needed by accepted
Opportunity work and existing records.

Selected because it prevents new manually authored briefs without deleting
history or disturbing the automatic planning pipeline.

### 3. Require provenance with a database constraint and migrate old Topics

Make `topics.source_content_action_id` mandatory and migrate, archive, or delete
every source-less Topic.

Rejected for this change because source-less rows can represent several
historical or automated paths, including legacy Strategist output and accepted
GEO asset briefs. They cannot be classified safely from the nullable field
alone. A schema migration would expand a focused product simplification into a
provenance migration with data-loss risk.

## Decision

Remove manual Content Brief creation at both exposed layers:

1. Remove `New Content Brief`, its form state, submission handler, and create
   copy from Content Plan.
2. Remove the frontend `createTopic` method and its create-only input type.
3. Remove `POST /projects/{projectID}/topics` and the `createTopic` HTTP handler.
4. Remove the legacy browser `runStrategist` method and
   `POST /projects/{projectID}/strategist` handler. That endpoint creates
   source-less Topics directly and bypasses Opportunity arbitration and
   acceptance.
5. Keep the internal `db.CreateTopic` query. Accepted Opportunity planning and
   scheduled automation still use it to materialize internal generation Topics.
6. Do not add manual Opportunity creation. A user-triggered `Find opportunities`
   run remains valid because AI creates the Opportunities; the user is not
   authoring an Opportunity record.

The empty Content Plan state directs users to Opportunities. Doctor findings
continue to create Site Fix work and do not create Content Briefs.

## Existing Data And Compatibility

Existing Topics remain readable and operational. This change does not delete,
archive, or rewrite them. In particular:

- Content Plan can still list legacy Topics.
- Existing scheduling, editing, drafting, and archiving controls continue to
  work for backward compatibility.
- Accepted Opportunity actions can still create a linked Topic through the
  content-action planning endpoint.
- Auto mode can still create and draft linked Topics through the scheduler.

The legacy section copy should describe these as earlier briefs without an
Opportunity link rather than advertising manual creation as a current feature.

## API Boundary

After this change:

| Capability | Status |
| --- | --- |
| List internal Topics | Preserved |
| Update publish strategy or brief fields | Preserved |
| Schedule an existing Topic | Preserved |
| Generate a draft from an existing Topic | Preserved |
| Archive an existing Topic | Preserved |
| Plan an accepted Opportunity content action | Preserved |
| Create an arbitrary Topic over HTTP | Removed |
| Run legacy Strategist directly over HTTP | Removed |
| Create a Content Brief from the Content Plan UI | Removed |
| Create a manual Opportunity | Not supported |

An old cached frontend that attempts `POST /topics` receives a method-not-
allowed response and cannot create data.

## Documentation

The current product PRDs must stop presenting manual seed creation as an
accepted Content Plan workflow. Historical implementation plans remain
unchanged as records of what was previously built.

The source contract should be stated consistently:

- Doctor owns immediately verifiable Site Fix work.
- Opportunities owns measurable growth work and Content Brief creation.
- Content Plan owns execution after Opportunity acceptance.
- There is no manual Opportunity or Content Brief intake in this phase.

## Non-Goals

- Deleting or migrating existing source-less Topics.
- Removing Topic as an internal persistence and generation object.
- Removing manual draft execution while Auto is off; operators may still click
  `Draft Content` on an accepted AI-generated brief.
- Removing the user-triggered AI Opportunity Finding run.
- Redesigning Doctor, Opportunities, Review, Publish, or Results.
- Completing a full provenance migration for legacy Strategist or GEO records.

## Verification

Automated contracts must prove:

1. Content Plan contains no `New Content Brief` entry, create form, create
   handler, or copy offering manual creation.
2. The web API client exposes no `createTopic` mutation.
3. The Go router exposes no `POST /projects/{projectID}/topics` handler.
4. The web client and Go router expose no legacy direct Strategist execution.
5. Topic list, update, schedule, generate, and archive capabilities remain.
6. Accepted Opportunity planning still creates linked Topics through its own
   endpoint.
7. Existing Go tests, Web tests, type checking, lint-equivalent checks, and
   production builds pass.

Production verification must confirm that Content Plan no longer displays the
manual creation control or form, its empty state points users to Opportunities,
and an accepted Opportunity can still enter Content Plan and draft normally.
