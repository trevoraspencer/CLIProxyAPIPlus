---
name: config-docs-worker
description: Updates provider config examples and factual auth documentation.
---

# Config Docs Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## Work Procedure

1. Read `mission.md`, mission `AGENTS.md`, `.factory/services.yaml`, `.factory/library/`, and your assigned feature.
2. Preserve mission boundaries: mocked/local validation only, no live provider calls, no fixed ports, no real credentials.
3. Use TDD for behavior changes: add or tighten failing tests first, then implement. For docs/config-only changes, inspect existing patterns first and keep examples placeholder-only.
4. Run targeted checks for touched areas, then run broader checks required by the feature.
5. Before handoff, review `git diff` for secrets/stale references and ensure no generated artifacts remain.

## When to Use This Skill

Use for `config.example.yaml`, README/auth documentation, and docs that explain provider setup without runtime implementation changes.

## Verification Requirements

- Match existing YAML and Markdown style, but remove promotional language when touching affected sections.
- Parse `config.example.yaml` after changes.
- Ensure examples use placeholders only and OpenCode remains `openai-compatibility` config/docs-only.
- Check docs/config aliases match.

## Example Handoff

```json
{
  "salientSummary": "Documented OpenCode Zen as an OpenAI-compatible chat-completions provider with distinct aliases and placeholder credentials.",
  "whatWasImplemented": "Added an opencode-zen openai-compatibility example to config.example.yaml and matching README setup notes that state OpenCode Zen is not OAuth or first-class provider support.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {"command": "python3 -c "import yaml, pathlib; yaml.safe_load(pathlib.Path('config.example.yaml').read_text())"", "exitCode": 0, "observation": "Config example parsed successfully."}
    ],
    "interactiveChecks": [
      {"action": "Searched diff for real secrets and OpenCode runtime code", "observed": "Only placeholder OPENCODE_API_KEY and docs/config changes were present."}
    ]
  },
  "tests.added": [],
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- Docs cannot accurately describe a behavior without additional implementation scope.
- Existing documentation conflicts with the accepted mission requirements.
