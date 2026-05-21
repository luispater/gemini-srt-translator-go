# Repository Guidelines

## Project Structure & Module Organization
The CLI entrypoint lives in `cmd/`, which wires configuration, providers, and translation flows. Shared but non-exported logic resides in `internal/` with notable packages such as `translator/` for batching requests, `providers/` for Gemini integrations, `helpers/` for retries, `video/` for MKV handling, and `logger/` for progress output. Reusable APIs are exposed from `pkg/`—`config/` parses flags and environment variables, `errors/` standardizes wrapped errors, `languages/` resolves language codes, and `srt/` handles parsing as well as serialization. Tests sit beside their implementation files as `*_test.go`, and release automation lives under `.github/` and `.goreleaser.yml`.

## Build, Test, and Development Commands
Run `go mod tidy` whenever dependencies change to sync `go.mod` and `go.sum`. Build the CLI with `go build -o gst ./cmd`, producing the `gst` binary for local use. Execute `go test ./...` for the full suite, `go test -cover ./...` when you need quick coverage, and `go vet ./...` before submitting work. Use `./gst --help` or `./gst` to review runtime options, and export `GEMINI_API_KEY` in your shell before invoking the tool.

## Coding Style & Naming Conventions
Target Go 1.23+ and always format with `gofmt`, keeping tab indentation and trailing newlines. Prefer short, lowercase package names, `CamelCase` for exported identifiers, and `camelCase` for internal ones. Avoid shadowing by using suffixes on short-lived variables (e.g., `if errDecode := decoder.Run(); errDecode != nil { ... }`). Error values should be last in the signature and wrapped with contextual messages. Comments must be in English doc-comment form above the entities they describe.

## Testing Guidelines
Use the standard `testing` framework with table-driven tests for translators, providers, and SRT helpers. Name tests `TestXxx` and colocate them with the code under test to keep package coverage intuitive. When touching SRT parsing or Gemini request batching, add edge cases that capture malformed files, rate-limited responses, and resume scenarios. Always run `go test ./...` locally; rely on coverage flags for quick regressions rather than full profiling.

## Commit & Pull Request Guidelines
Keep commits scoped and written in present tense; Conventional Commit prefixes such as `feat(subtitles):` or `fix(video):` are welcome but not mandatory. Pull requests should explain motivation, outline the approach, link issues, and mention configuration or documentation updates if behavior changes. Validate that `go vet ./...` and `go test ./...` succeed before pushing, and avoid bundling unrelated refactors. Include CLI usage examples or screenshots when changes affect user workflows.

## Security & Configuration Tips
Never store API keys in the repository—configure `GEMINI_API_KEY` via environment variables (comma-separated keys are supported for higher quota). Prefer environment variables over CLI flags in shared environments, and scrub the `gst` binary before committing. Use GoReleaser and the workflows in `.github/` for packaging; local artifacts and logs should stay out of version control.