# Workspace Guidance

## Meta

- When updating `CLAUDE.md`, apply the same changes to `AGENTS.md`.

## Infrastructure

- **Local source:** `/home/aqouet/whispr_src` — GitHub: `git@github.com:Aqoouet/whispr.git`
- **Build host:** `stressii-wg` — repo at `/mnt/ii_models/whispr` (same share as `R:\whispr` on `workpc`)
- **Shared network drive:**
  - Linux path on `stressii-wg`: `/mnt/ii_models`
  - UNC path: `\\e0-filer03\ii_models\`
  - Windows drive letter on `workpc`: `R:\`
- **Deploy target:** `T:\whispr\dictation.exe` — only valid location for the running executable; no other path is accepted.

## Whispr Workflow

- For the `whispr` project, edit source files locally first, using the local source copy as the primary editing workspace.
- Do not rely on direct source-file editing over SSH on `stressii-wg`; code-file writes there are difficult and fragile.
- After local source changes are ready, sync them to `stressii-wg` through GitHub.
- Build on `stressii-wg`, not on the local machine.
- Standard Windows build command on `stressii-wg`:
  - `GOOS=windows GOARCH=amd64 go build -o build/dictation.exe ./cmd/dictation`
- After a successful build on `stressii-wg`, deploy to Windows by copying the built executable through the Windows-visible copy path.
- Preferred deployment flow for `whispr`:
  1. Edit code locally.
  2. Commit and push to GitHub.
  3. Update the repo on `stressii-wg` from GitHub.
  4. Build on `stressii-wg`.
  5. Copy the resulting Windows executable to the approved Windows launch location.
- Treat this as the default process unless the user explicitly asks for a different workflow.

## Code Review Delegation Rules

- After writing or modifying code, spawn an OpenAI Codex agent using model `gpt-5.4-medium` to review the changes.
- The spawned Codex agent reviews either uncommitted changes or the last commit — whichever is relevant.
- Invoke via `/codex:review` (uncommitted changes) or `/codex:review --commit HEAD` (last commit).
- The current agent must never skip this review step before committing code changes.
- Address any CRITICAL or HIGH findings before proceeding.

## Low-Cost Agent Delegation Rules

- Use a separate low-cost Codex subagent every time for all `whispr` commit, push, and deploy work.
- In this repo, the default low-cost analogue to Claude `haiku` is the spark model lane, currently `gpt-5.3-codex-spark`.
- The spawned low-cost subagent owns and executes all `whispr` commit, push, and deploy steps.
- The current agent must never perform `whispr` commit, push, or deploy work itself, even if the steps seem trivial or already prepared.
- Use the spawned low-cost subagent every time for deployment verification that confirms `T:\whispr\dictation.exe` matches the latest built executable.
- The current agent must never perform that deployment verification itself.
- Use the spawned low-cost subagent every time to read and summarize recent errors from `C:\Users\rymax1e\AppData\Local\CorpDictation\logs\app.log`.
- The current agent must never read or summarize those recent log errors inline.

## Whispr Deploy And Logs

- Build output on the Windows side is read from `R:\whispr\build\dictation.exe`.
- Approved launch/deploy location is `T:\whispr\dictation.exe`.
- Standard Windows deploy step:
  1. Build on `stressii-wg`.
     Build command: `GOOS=windows GOARCH=amd64 go build -o build/dictation.exe ./cmd/dictation`
  2. Ensure the new executable is visible at `R:\whispr\build\dictation.exe`.
  3. Copy `R:\whispr\build\dictation.exe` to `T:\whispr\dictation.exe`.
  4. Launch from `T:\whispr\dictation.exe`.
  5. After every deploy, the spawned low-cost subagent verifies that `T:\whispr\dictation.exe` matches the latest built executable.
- Runtime log path on `workpc` is `C:\Users\rymax1e\AppData\Local\CorpDictation\logs\app.log`.
- Standard log check command on `workpc`:
  - `ssh workpc powershell -NoProfile -Command Get-Content 'C:\\Users\\rymax1e\\AppData\\Local\\CorpDictation\\logs\\app.log' -Tail 40`
- Default log-reading pattern:
  1. Relaunch or test the app.
  2. Pull the tail of `app.log`.
  3. Confirm the latest `startup build=...` line matches the expected build.
  4. Read the next `record start failed` or success lines from the same tail.
