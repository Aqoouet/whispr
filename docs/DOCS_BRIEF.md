# CorpDictation Brief

Purpose: local offline Windows dictation app in Go that records microphone audio on a hotkey, transcribes with local `whisper.cpp`, copies text to the clipboard, and optionally pastes with `Ctrl+V`.

Current structure:
- `cmd/dictation`: app entrypoint
- `cmd/setup`: staging/setup entrypoint
- `internal/*`: app orchestration, config, runtime validation, Windows platform code, Whisper loading
- `third_party/whisper_shim`: C++ DLL wrapper over `whisper.cpp`

Runtime folder target:
- runtime root precedence: `CORPDICTATION_ROOT`, then machine-wide Windows root when present, then `%LOCALAPPDATA%\CorpDictation`, then staging fallback for dev / non-Windows CLI flows
- deployed Windows machine-wide root helper: `%ProgramData%\CorpDictation`
- runtime root subdirs: `runtime`, `models`, `config`, `logs`

Config:
- Reads `<resolved runtime root>\config\config.json`
- Defaults: `language=ru`, `preferred_device=cuda`, `fallback_device=cpu`, `preferred_model=large-v3-turbo-q5_0`, `fallback_model=medium-q5_0`, `cpu_fallback_model=small-q5_1`, `hotkey=Ctrl+Alt+Space`, `auto_paste=true`, `save_audio=false`, `log_transcripts=false`, `beam_size=1`

Implemented now:
- Go module and app skeleton
- runtime path/config loading
- model/device selection logic
- Linux CLI dev mode: `./dictation --input test.wav --language ru`
- Windows-only files split by build tags
- Windows hotkey/clipboard/paste/status window scaffolding
- Windows WinMM microphone recording scaffolding
- Windows Whisper DLL loader scaffolding
- setup command creates staging tree, config, manifest, and sync/build helper scripts
- official `whisper.cpp` source cloned at commit `0ccd896f5b882628e1c077f9769735ef4ce52860`
- staged model downloads completed: `ggml-large-v3-turbo-q5_0.bin`, `ggml-medium-q5_0.bin`, `ggml-small-q5_1.bin`
- Linux `whisper-cli` built and copied to `staging/linux/bin/whisper-cli`
- Linux CLI transcription validated with upstream `samples/jfk.wav`
- Windows CPU runtime DLLs cross-built and staged in `staging/windows-localappdata/CorpDictation/runtime`
- Windows CLI transcription validated on a real Windows host after bundling the required `llvm-mingw` runtime DLLs

Planned but not finished:
- verified end-to-end Windows run with real `whisper.cpp` DLLs on a Windows machine
- CUDA-enabled Windows runtime build and staging
- polished tray icon; current Windows UI target is a small status window

Downloaded/prepared files during setup:
- staged folder tree under `staging/windows-localappdata/CorpDictation`
- `download-manifest.json` with official source URLs
- helper scripts: `staging/prepare_all.sh`, `staging/download_models.sh`, `staging/build_linux_dev_backend.sh`, `staging/build_windows_runtime_cpu_podman.sh`, `staging/sync_to_localappdata.ps1`
- model files present: `ggml-large-v3-turbo-q5_0.bin`, `ggml-medium-q5_0.bin`, `ggml-small-q5_1.bin`
- runtime DLLs present: `corpdictation_whisper.dll`, `libwhisper.dll`, `ggml.dll`, `ggml-base.dll`, `ggml-cpu.dll`, `libc++.dll`, `libunwind.dll`, `libomp.dll`, `libwinpthread-1.dll`
- portable Linux-side Windows cross-toolchain downloaded: `toolchains/llvm-mingw-20260519-ucrt-ubuntu-22.04-x86_64`

Download sources:
- official `whisper.cpp` repository: `https://github.com/ggml-org/whisper.cpp`
- official model host referenced by `whisper.cpp`: `https://huggingface.co/ggerganov/whisper.cpp`
- official llvm-mingw release metadata and toolchain archive: `https://github.com/mstorsjo/llvm-mingw/releases/tag/20260519`
- official Ubuntu container image used for the successful Windows CPU DLL cross-build: `docker.io/library/ubuntu:24.04`

Checksums:
- `ggml-large-v3-turbo-q5_0.bin`: SHA1 `e050f7970618a659205450ad97eb95a18d69c9ee`, SHA256 `394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2`
- `ggml-medium-q5_0.bin`: SHA1 `7718d4c1ec62ca96998f058114db98236937490e`, SHA256 `19fea4b380c3a618ec4723c3eef2eb785ffba0d0538cf43f8f235e7b3b34220f`
- `ggml-small-q5_1.bin`: SHA1 `6fe57ddcfdd1c6b07cdcc73aaf620810ce5fc771`, SHA256 `ae85e4a935d7a567bd102fe55afc16bb595bdb618e11b2fc7591bc08120411bb`

Privacy/runtime behavior:
- no cloud use
- no runtime downloads
- no telemetry
- no transcript logging
- no audio retention by default
- final runtime is intended to work fully offline from the resolved runtime root

Linux vs Windows:
- development environment: Linux
- final runtime target: Windows 10/11 x64
- Linux-tested scope: config loading, runtime validation, CLI path, shared app flow
- Windows-tested scope so far: real DLL loading, model loading, CPU fallback, local CLI transcription
- Windows-only testing still required: global hotkey, mic capture, clipboard, paste, status window, CUDA path, interactive desktop launch

Build status:
- target app build command: `GOOS=windows GOARCH=amd64 go build -o build/dictation.exe ./cmd/dictation`
- pure Go app layer is designed to cross-compile
- Whisper backend uses a separate Windows DLL wrapper, not cgo in the Go executable
- Linux-to-Windows Go `.exe` build works in this workspace
- Windows CPU DLL build works from Linux using a rootless Ubuntu container plus the official llvm-mingw toolchain
- CUDA-enabled DLL builds are not staged yet; they may still need a Windows-native or CUDA-equipped container build path
- reproducible staging entrypoint: `staging/prepare_all.sh`

Current status:
- source tree bootstrapped
- runtime assets for CPU/offline use are staged in this workspace
- first full Windows hotkey/microphone/clipboard/paste run is not yet verified on Windows
- OpenSSH console launch of normal app mode fails with `RegisterHotKey failed: This operation requires an interactive window station`; that is expected for SSH and must be tested from the logged-in desktop session
- current staged runtime is CPU-only; config still prefers CUDA first and should fall back to CPU with the prepared DLL set
