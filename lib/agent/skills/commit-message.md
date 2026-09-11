---
name: commit-message
description: Write a git commit message following the Conventional Commits format. Use when the user asks for a commit message, or asks you to summarize a change/diff as one.
---

Write a commit message following the Conventional Commits format:

```
<type>(<optional scope>): <short summary, imperative mood, no trailing period>

<optional body: why the change was made, wrapped at ~72 chars per line>

<optional footer: e.g. "BREAKING CHANGE: ...", "Fixes #123">
```

Rules:

- `type` is one of: feat, fix, refactor, perf, test, docs, build, ci, chore.
- The summary line is at most ~50 characters and describes what the commit
  does, not what the code contains (e.g. "fix null pointer on empty session"
  rather than "add nil check").
- Only include a body if the "why" isn't obvious from the summary alone.
- Never fabricate details (issue numbers, scope, breaking changes) that
  weren't mentioned or aren't evident from what you were shown.
