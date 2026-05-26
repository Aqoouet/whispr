package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"corpdictation/internal/config"
	"corpdictation/internal/runtimecheck"
)

const whisperCommit = "0ccd896f5b882628e1c077f9769735ef4ce52860"

type DownloadItem struct {
	Name          string `json:"name"`
	Source        string `json:"source"`
	Target        string `json:"target"`
	SHA1          string `json:"sha1,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	RequiredAtRun bool   `json:"required_at_runtime"`
	Notes         string `json:"notes,omitempty"`
}

type Manifest struct {
	WhisperRepo string         `json:"whisper_repo"`
	Commit      string         `json:"commit"`
	Items       []DownloadItem `json:"items"`
}

func Run(root string) error {
	paths := runtimecheck.ResolvePaths(root)
	for _, dir := range []string{paths.Root, paths.RuntimeDir, paths.ModelsDir, paths.ConfigDir, paths.LogsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	if err := config.Write(filepath.Join(paths.ConfigDir, "config.json"), config.Default()); err != nil {
		return err
	}

	manifest := Manifest{
		WhisperRepo: "https://github.com/ggml-org/whisper.cpp.git",
		Commit:      whisperCommit,
		Items: []DownloadItem{
			{Name: "ggml-large-v3-turbo-q5_0.bin", Source: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo-q5_0.bin", Target: filepath.Join("models", "ggml-large-v3-turbo-q5_0.bin"), SHA1: "e050f7970618a659205450ad97eb95a18d69c9ee", SHA256: "394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2", RequiredAtRun: true},
			{Name: "ggml-medium-q5_0.bin", Source: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium-q5_0.bin", Target: filepath.Join("models", "ggml-medium-q5_0.bin"), SHA1: "7718d4c1ec62ca96998f058114db98236937490e", SHA256: "19fea4b380c3a618ec4723c3eef2eb785ffba0d0538cf43f8f235e7b3b34220f", RequiredAtRun: true, Notes: "Checksum verified from the downloaded file because the quantized SHA1 is not listed in the upstream README table."},
			{Name: "ggml-small-q5_1.bin", Source: "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small-q5_1.bin", Target: filepath.Join("models", "ggml-small-q5_1.bin"), SHA1: "6fe57ddcfdd1c6b07cdcc73aaf620810ce5fc771", SHA256: "ae85e4a935d7a567bd102fe55afc16bb595bdb618e11b2fc7591bc08120411bb", RequiredAtRun: true, Notes: "Checksum verified from the downloaded file because the quantized SHA1 is not listed in the upstream README table."},
			{Name: "corpdictation_whisper.dll", Source: "built locally from third_party/whisper_shim", Target: filepath.Join("runtime", "corpdictation_whisper.dll"), RequiredAtRun: true, Notes: "The staged CPU runtime is produced by a Linux->Windows cross-build inside a rootless Ubuntu container."},
			{Name: "libwhisper.dll", Source: "built locally from official whisper.cpp", Target: filepath.Join("runtime", "libwhisper.dll"), RequiredAtRun: true},
			{Name: "ggml.dll", Source: "built locally from official whisper.cpp", Target: filepath.Join("runtime", "ggml.dll"), RequiredAtRun: true},
			{Name: "ggml-base.dll", Source: "built locally from official whisper.cpp", Target: filepath.Join("runtime", "ggml-base.dll"), RequiredAtRun: true},
			{Name: "ggml-cpu.dll", Source: "built locally from official whisper.cpp", Target: filepath.Join("runtime", "ggml-cpu.dll"), RequiredAtRun: true, Notes: "CUDA runtime DLLs are not staged yet; current prepared runtime is CPU-only."},
			{Name: "libc++.dll", Source: "bundled from the official llvm-mingw x86_64 runtime", Target: filepath.Join("runtime", "libc++.dll"), RequiredAtRun: true},
			{Name: "libunwind.dll", Source: "bundled from the official llvm-mingw x86_64 runtime", Target: filepath.Join("runtime", "libunwind.dll"), RequiredAtRun: true},
			{Name: "libomp.dll", Source: "bundled from the official llvm-mingw x86_64 runtime", Target: filepath.Join("runtime", "libomp.dll"), RequiredAtRun: true},
			{Name: "libwinpthread-1.dll", Source: "bundled from the official llvm-mingw x86_64 runtime", Target: filepath.Join("runtime", "libwinpthread-1.dll"), RequiredAtRun: true},
		},
	}
	if err := writeJSON(filepath.Join(paths.Root, "download-manifest.json"), manifest); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(paths.Root, "logs", ".gitkeep"), []byte{}, 0o644); err != nil {
		return err
	}

	if err := writeBootstrapScripts(root); err != nil {
		return err
	}
	return nil
}

func writeBootstrapScripts(root string) error {
	syncPS1 := `$ErrorActionPreference = "Stop"
$target = if ($env:CORPDICTATION_ROOT) { $env:CORPDICTATION_ROOT } else { Join-Path $env:LOCALAPPDATA "CorpDictation" }
$source = Join-Path $PSScriptRoot "windows-localappdata\CorpDictation"
New-Item -ItemType Directory -Force -Path $target | Out-Null
Copy-Item -Path (Join-Path $source "*") -Destination $target -Recurse -Force
Write-Host "Synced CorpDictation runtime to $target (set CORPDICTATION_ROOT for machine-wide deployment targets)"
`
	if err := os.MkdirAll("staging", 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("staging", "sync_to_localappdata.ps1"), []byte(syncPS1), 0o644); err != nil {
		return err
	}

	downloadModelsSh := `#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UPSTREAM_DIR="$ROOT/third_party/whisper.cpp"
MODELS_DIR="$ROOT/staging/windows-localappdata/CorpDictation/models"
COMMIT="` + whisperCommit + `"

if [ ! -d "$UPSTREAM_DIR/.git" ]; then
  git clone https://github.com/ggml-org/whisper.cpp.git "$UPSTREAM_DIR"
fi
git -C "$UPSTREAM_DIR" fetch --tags --force
git -C "$UPSTREAM_DIR" checkout "$COMMIT"
mkdir -p "$MODELS_DIR"
sh "$UPSTREAM_DIR/models/download-ggml-model.sh" large-v3-turbo-q5_0 "$MODELS_DIR"
sh "$UPSTREAM_DIR/models/download-ggml-model.sh" medium-q5_0 "$MODELS_DIR"
sh "$UPSTREAM_DIR/models/download-ggml-model.sh" small-q5_1 "$MODELS_DIR"
`

	buildLinuxDevSh := `#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UPSTREAM_DIR="$ROOT/third_party/whisper.cpp"
BUILD_DIR="$ROOT/build/whisper-linux-safe"
OUT_DIR="$ROOT/staging/linux/bin"

cmake -S "$UPSTREAM_DIR" -B "$BUILD_DIR" -DWHISPER_BUILD_EXAMPLES=ON -DWHISPER_SDL2=OFF -DGGML_NATIVE=OFF -DGGML_AVX=ON -DGGML_AVX2=ON -DGGML_FMA=ON -DGGML_F16C=ON -DGGML_AVX_VNNI=OFF -DGGML_AVX512=OFF -DGGML_AVX512_VNNI=OFF -DGGML_AVX512_VBMI=OFF -DGGML_AVX512_BF16=OFF
cmake --build "$BUILD_DIR" -j 4 --target whisper-cli
mkdir -p "$OUT_DIR"
cp "$BUILD_DIR/bin/whisper-cli" "$OUT_DIR/whisper-cli"
`
	if err := os.WriteFile(filepath.Join("staging", "build_linux_dev_backend.sh"), []byte(buildLinuxDevSh), 0o755); err != nil {
		return err
	}

	buildWindowsRuntimeSh := `#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLCHAIN_DIR="$ROOT/toolchains/llvm-mingw-20260519-ucrt-ubuntu-22.04-x86_64"
RUNTIME_DIR="$ROOT/staging/windows-localappdata/CorpDictation/runtime"

if ! command -v podman >/dev/null 2>&1; then
  echo "podman is required for the reproducible Windows CPU runtime build" >&2
  exit 1
fi

mkdir -p "$ROOT/.xdg-runtime" "$RUNTIME_DIR"

env XDG_RUNTIME_DIR="$ROOT/.xdg-runtime" podman run --rm \
  -v "$ROOT:/workspace" \
  -w /workspace \
  ubuntu:24.04 \
  bash -lc '
    apt-get update >/dev/null &&
    DEBIAN_FRONTEND=noninteractive apt-get install -y cmake make gcc g++ >/dev/null &&
    export PATH="/workspace/toolchains/llvm-mingw-20260519-ucrt-ubuntu-22.04-x86_64/bin:$PATH" &&
    cmake -S third_party/whisper.cpp -B build/whisper-windows-cpu-podman3 \
      -DCMAKE_SYSTEM_NAME=Windows \
      -DCMAKE_SYSTEM_PROCESSOR=x86_64 \
      -DCMAKE_C_COMPILER=x86_64-w64-mingw32-clang \
      -DCMAKE_CXX_COMPILER=x86_64-w64-mingw32-clang++ \
      -DCMAKE_BUILD_TYPE=Release \
      -DBUILD_SHARED_LIBS=ON \
      -DWHISPER_BUILD_EXAMPLES=OFF \
      -DWHISPER_BUILD_TESTS=OFF \
      -DGGML_NATIVE=OFF \
      -DGGML_AVX=ON \
      -DGGML_AVX2=ON \
      -DGGML_FMA=ON \
      -DGGML_F16C=ON \
      -DGGML_AVX_VNNI=OFF \
      -DGGML_AVX512=OFF \
      -DGGML_AVX512_VNNI=OFF \
      -DGGML_AVX512_VBMI=OFF \
      -DGGML_AVX512_BF16=OFF &&
    cmake --build build/whisper-windows-cpu-podman3 -j 4 --target whisper &&
    cmake -S third_party/whisper_shim -B build/whisper-shim-windows-podman3 \
      -DWHISPER_CPP_SOURCE_DIR=/workspace/third_party/whisper.cpp \
      -DWHISPER_CPP_BUILD_DIR=/workspace/build/whisper-windows-cpu-podman3 \
      -DCMAKE_SYSTEM_NAME=Windows \
      -DCMAKE_SYSTEM_PROCESSOR=x86_64 \
      -DCMAKE_C_COMPILER=x86_64-w64-mingw32-clang \
      -DCMAKE_CXX_COMPILER=x86_64-w64-mingw32-clang++ \
      -DCMAKE_BUILD_TYPE=Release &&
    cmake --build build/whisper-shim-windows-podman3 -j 4
  '

cp "$ROOT/build/whisper-shim-windows-podman3/libcorpdictation_whisper.dll" "$RUNTIME_DIR/corpdictation_whisper.dll"
cp "$ROOT/build/whisper-windows-cpu-podman3/bin/libwhisper.dll" "$RUNTIME_DIR/libwhisper.dll"
cp "$ROOT/build/whisper-windows-cpu-podman3/bin/ggml.dll" "$RUNTIME_DIR/ggml.dll"
cp "$ROOT/build/whisper-windows-cpu-podman3/bin/ggml-base.dll" "$RUNTIME_DIR/ggml-base.dll"
cp "$ROOT/build/whisper-windows-cpu-podman3/bin/ggml-cpu.dll" "$RUNTIME_DIR/ggml-cpu.dll"
cp "$TOOLCHAIN_DIR/x86_64-w64-mingw32/bin/libc++.dll" "$RUNTIME_DIR/libc++.dll"
cp "$TOOLCHAIN_DIR/x86_64-w64-mingw32/bin/libunwind.dll" "$RUNTIME_DIR/libunwind.dll"
cp "$TOOLCHAIN_DIR/x86_64-w64-mingw32/bin/libomp.dll" "$RUNTIME_DIR/libomp.dll"
cp "$TOOLCHAIN_DIR/x86_64-w64-mingw32/bin/libwinpthread-1.dll" "$RUNTIME_DIR/libwinpthread-1.dll"
`
	if err := os.WriteFile(filepath.Join("staging", "build_windows_runtime_cpu_podman.sh"), []byte(buildWindowsRuntimeSh), 0o755); err != nil {
		return err
	}

	prepareAllSh := `#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

env GOCACHE="$ROOT/.gocache" go run -buildvcs=false ./cmd/setup
"$ROOT/staging/download_models.sh"
"$ROOT/staging/build_linux_dev_backend.sh"
"$ROOT/staging/build_windows_runtime_cpu_podman.sh"
`
	if err := os.WriteFile(filepath.Join("staging", "prepare_all.sh"), []byte(prepareAllSh), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join("staging", "download_models.sh"), []byte(downloadModelsSh), 0o755); err != nil {
		return err
	}

	compatFetchRuntimeSh := `#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
"$ROOT/staging/build_windows_runtime_cpu_podman.sh"
`
	if err := os.WriteFile(filepath.Join("staging", "fetch_and_build_runtime.sh"), []byte(compatFetchRuntimeSh), 0o755); err != nil {
		return err
	}

	return nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func FileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
