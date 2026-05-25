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

## Whispr Deploy And Logs

- Build output on the Windows side is read from `R:\whispr\build\dictation.exe`.
- Approved launch/deploy location is `T:\whispr\dictation.exe`.
- Standard Windows deploy step:
  1. Build on `stressii-wg`.
     Build command: `GOOS=windows GOARCH=amd64 go build -o build/dictation.exe ./cmd/dictation`
  2. Ensure the new executable is visible at `R:\whispr\build\dictation.exe`.
  3. Copy `R:\whispr\build\dictation.exe` to `T:\whispr\dictation.exe`.
  4. Launch from `T:\whispr\dictation.exe`.
- Runtime log path on `workpc` is `C:\Users\rymax1e\AppData\Local\CorpDictation\logs\app.log`.
- Standard log check command on `workpc`:
  - `ssh workpc powershell -NoProfile -Command Get-Content 'C:\\Users\\rymax1e\\AppData\\Local\\CorpDictation\\logs\\app.log' -Tail 40`
- Default log-reading pattern:
  1. Relaunch or test the app.
  2. Pull the tail of `app.log`.
  3. Confirm the latest `startup build=...` line matches the expected build.
  4. Read the next `record start failed` or success lines from the same tail.
