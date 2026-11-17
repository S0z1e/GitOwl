# Issue tracker: GitHub Issues

Issues for this repository live in GitHub Issues. Use the `gh` CLI for them.

## Conventions

- Each issue is a single problem or feature, with its acceptance criteria in the body.
- Triage state is recorded with labels. The label names are in `triage-labels.md`.
- Comments and conversation stay on the issue.

## When a skill says "publish to the issue tracker"

Create a GitHub issue with `gh issue create --title "..." --body-file <file>`.

## When a skill says "fetch the relevant ticket"

Run `gh issue view <number> --comments` and use the issue number the user gives.

## Triage state changes

Add or remove labels with `gh issue edit <number> --add-label <label>` or `--remove-label <label>`.
