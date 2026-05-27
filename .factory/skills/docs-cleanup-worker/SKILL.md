---
name: docs-cleanup-worker
description: Removes stale localized/promotional docs and repairs repository references.
---

# Docs Cleanup Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## Work Procedure

1. Read `mission.md`, mission `AGENTS.md`, `.factory/services.yaml`, `.factory/library/`, and your assigned feature.
2. Preserve mission boundaries: mocked/local validation only, no live provider calls, no fixed ports, no real credentials.
3. Use TDD for behavior changes: add or tighten failing tests first, then implement. For docs/config-only changes, inspect existing patterns first and keep examples placeholder-only.
4. Run targeted checks for touched areas, then run broader checks required by the feature.
5. Before handoff, review `git diff` for secrets/stale references and ensure no generated artifacts remain.

## When to Use This Skill

Use for deleting localized/promotional documentation, sponsor assets, funding/fork metadata, and repairing release/metadata/README links.

## Verification Requirements

- Remove only scoped docs/assets/metadata from the feature description.
- Repair `.goreleaser.yml`, `.dockerignore`, `.gitattributes`, workflows, README, docs, and AGENTS references as needed.
- Run stale-reference and promotional-term greps against user-facing/repository files, excluding historical `.factory/validation` artifacts so prior validation evidence does not produce false positives.
- Run final gates when assigned the final consistency feature.

## Example Handoff

```json
{
  "salientSummary": "Removed localized docs and sponsor assets, then repaired release metadata and README references.",
  "whatWasImplemented": "Deleted README_CN.md, README_JA.md, docs/*_CN.md, sponsor images, FUNDING.yml, and README-ccs-fork.md; updated metadata so no deleted files are referenced.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {"command": "rg 'README_CN|README_JA|README-ccs-fork|packycode|AICodeMirror|BmoPlus|VisionCoder|FUNDING' .", "exitCode": 1, "observation": "No stale user-facing references remained."},
      {"command": "git diff --check", "exitCode": 0, "observation": "No whitespace errors."}
    ],
    "interactiveChecks": [
      {"action": "Reviewed README headings", "observed": "Sponsor and promotional showcase sections were absent."}
    ]
  },
  "tests.added": [],
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- A removed file is still required by build/release automation and the correct replacement is ambiguous.
- A stale reference appears in generated/vendor content outside mission scope.
