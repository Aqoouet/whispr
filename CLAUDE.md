# Local Workflow Notes

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

## Whispr

- Source edits should be made locally, not by editing code files directly over SSH on `stressii-wg`.
- Sync source changes through GitHub before building on `stressii-wg`.
- Use `stressii-wg` as the build host after the GitHub sync step.
- Standard Windows build command on `stressii-wg`:
  - `GOOS=windows GOARCH=amd64 go build -o build/dictation.exe ./cmd/dictation`
- Deploy to Windows by copying the built executable from the build output on the Windows side.
- Default `whispr` sequence:
  1. Change source locally.
  2. Commit and push to GitHub.
  3. Pull or update on `stressii-wg` from GitHub.
  4. Build on `stressii-wg`.
  5. Deploy with the Windows copy step.

## Low-Cost Agent Delegation Rules

- Use a separate low-cost Codex subagent every time for all `whispr` commit, push, and deploy work.
- In this repo, the default low-cost analogue to Claude `haiku` is the spark model lane, currently `gpt-5.3-codex-spark`.
- The spawned low-cost subagent owns and executes all `whispr` commit, push, and deploy steps.
- The current agent must never perform `whispr` commit, push, or deploy work itself, even if the steps seem trivial or already prepared.
- Use the spawned low-cost subagent every time for deployment verification that confirms `T:\whispr\dictation.exe` matches the latest built executable.
- The current agent must never perform that deployment verification itself.
- Use the spawned low-cost subagent every time to read and summarize recent errors from `C:\Users\rymax1e\AppData\Local\CorpDictation\logs\app.log`.
- The current agent must never read or summarize those recent log errors inline.

## Whispr Deploy Details

- Windows-visible build source: `R:\whispr\build\dictation.exe`
- Approved deployed executable path: `T:\whispr\dictation.exe`
- Build on `stressii-wg` with:
  - `GOOS=windows GOARCH=amd64 go build -o build/dictation.exe ./cmd/dictation`
- Deploy by copying `R:\whispr\build\dictation.exe` to `T:\whispr\dictation.exe`
- Launch from `T:\whispr\dictation.exe`
- After every deploy, the spawned low-cost subagent verifies that `T:\whispr\dictation.exe` matches the latest built executable.

## Whispr Logs

- Log file on `workpc`: `C:\Users\rymax1e\AppData\Local\CorpDictation\logs\app.log`
- Read recent logs with:
  - `ssh workpc powershell -NoProfile -Command Get-Content 'C:\\Users\\rymax1e\\AppData\\Local\\CorpDictation\\logs\\app.log' -Tail 40`
- When validating deploys, confirm the latest `startup build=...` line before interpreting later audio errors.
