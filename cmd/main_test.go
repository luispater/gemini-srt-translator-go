package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/luispater/gemini-srt-translator-go/internal/providers"
	"github.com/luispater/gemini-srt-translator-go/internal/translator"
	"github.com/luispater/gemini-srt-translator-go/pkg/config"
)

func TestEnvironmentConfigurationOverridesDefaults(t *testing.T) {
	t.Setenv(config.EnvProvider, "openai")
	t.Setenv(config.EnvOpenAIProtocol, "responses")
	t.Setenv(config.EnvBaseURL, "https://environment.example.com")
	t.Setenv(config.EnvAPIKey, "environment-key-1, environment-key-2")
	t.Setenv(config.EnvModel, "environment-model")
	t.Setenv("OPENAI_API_KEY", "legacy-key")
	t.Setenv("OPENAI_BASE_URL", "https://legacy.example.com")

	commandConfig := config.NewConfig()
	command := newRootCommandWithConfig(commandConfig)
	if errPreRun := command.PreRunE(command, nil); errPreRun != nil {
		t.Fatalf("PreRunE returned an error: %v", errPreRun)
	}

	if commandConfig.Provider != "openai" {
		t.Errorf("Expected provider 'openai', got %q", commandConfig.Provider)
	}
	if commandConfig.OpenAIProtocol != "responses" {
		t.Errorf("Expected OpenAI protocol 'responses', got %q", commandConfig.OpenAIProtocol)
	}
	if commandConfig.BaseURL != "https://environment.example.com" {
		t.Errorf("Expected environment base URL, got %q", commandConfig.BaseURL)
	}
	if commandConfig.ModelName != "environment-model" {
		t.Errorf("Expected environment model, got %q", commandConfig.ModelName)
	}
	if len(commandConfig.APIKeys) != 2 || commandConfig.APIKeys[0] != "environment-key-1" || commandConfig.APIKeys[1] != "environment-key-2" {
		t.Errorf("Expected environment API keys, got %v", commandConfig.APIKeys)
	}
}

func TestCommandLineConfigurationOverridesEnvironment(t *testing.T) {
	t.Setenv(config.EnvProvider, "gemini")
	t.Setenv(config.EnvOpenAIProtocol, "chat-completions")
	t.Setenv(config.EnvBaseURL, "https://environment.example.com")
	t.Setenv(config.EnvAPIKey, "environment-key")
	t.Setenv(config.EnvModel, "environment-model")

	commandConfig := config.NewConfig()
	command := newRootCommandWithConfig(commandConfig)
	errParse := command.Flags().Parse([]string{
		"-p", "openai",
		"--openai-protocol", "responses",
		"--base-url", "https://command-line.example.com",
		"-k", "command-key-1, command-key-2",
		"-m", "command-model",
	})
	if errParse != nil {
		t.Fatalf("Failed to parse command-line flags: %v", errParse)
	}
	if errPreRun := command.PreRunE(command, nil); errPreRun != nil {
		t.Fatalf("PreRunE returned an error: %v", errPreRun)
	}

	if commandConfig.Provider != "openai" {
		t.Errorf("Expected provider 'openai', got %q", commandConfig.Provider)
	}
	if commandConfig.OpenAIProtocol != "responses" {
		t.Errorf("Expected OpenAI protocol 'responses', got %q", commandConfig.OpenAIProtocol)
	}
	if commandConfig.BaseURL != "https://command-line.example.com" {
		t.Errorf("Expected command-line base URL, got %q", commandConfig.BaseURL)
	}
	if commandConfig.ModelName != "command-model" {
		t.Errorf("Expected command-line model, got %q", commandConfig.ModelName)
	}
	if len(commandConfig.APIKeys) != 2 || commandConfig.APIKeys[0] != "command-key-1" || commandConfig.APIKeys[1] != "command-key-2" {
		t.Errorf("Expected command-line API keys, got %v", commandConfig.APIKeys)
	}
}

func TestProviderSelectionLoadsMatchingLegacyEnvironment(t *testing.T) {
	clearConfigurationEnvironment(t)
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", "https://gemini.example.com")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("OPENAI_BASE_URL", "https://openai.example.com")

	tests := []struct {
		name         string
		envProvider  string
		args         []string
		wantProvider string
		wantAPIKey   string
		wantBaseURL  string
		wantModel    string
	}{
		{
			name:         "normalized OpenAI CLI provider",
			args:         []string{"--provider", " OpenAI "},
			wantProvider: "openai",
			wantAPIKey:   "openai-key",
			wantBaseURL:  "https://openai.example.com",
			wantModel:    "gpt-4o",
		},
		{
			name:         "normalized OpenAI environment provider",
			envProvider:  " OPENAI ",
			wantProvider: "openai",
			wantAPIKey:   "openai-key",
			wantBaseURL:  "https://openai.example.com",
			wantModel:    "gpt-4o",
		},
		{
			name:         "explicit Gemini provider",
			args:         []string{"--provider", "gemini"},
			wantProvider: "gemini",
			wantAPIKey:   "gemini-key",
			wantBaseURL:  "https://gemini.example.com",
		},
		{
			name:         "OpenAI provider inferred from model",
			args:         []string{"--model", "GPT-test"},
			wantProvider: "openai",
			wantAPIKey:   "openai-key",
			wantBaseURL:  "https://openai.example.com",
		},
		{
			name:         "Gemini provider inferred from model",
			args:         []string{"--model", "gemini-test"},
			wantProvider: "gemini",
			wantAPIKey:   "gemini-key",
			wantBaseURL:  "https://gemini.example.com",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.envProvider != "" {
				t.Setenv(config.EnvProvider, testCase.envProvider)
			}
			commandConfig := config.NewConfig()
			command := newRootCommandWithConfig(commandConfig)
			if errParse := command.Flags().Parse(testCase.args); errParse != nil {
				t.Fatalf("Failed to parse command-line flags: %v", errParse)
			}
			if errPreRun := command.PreRunE(command, nil); errPreRun != nil {
				t.Fatalf("PreRunE returned an error: %v", errPreRun)
			}

			if commandConfig.Provider != testCase.wantProvider {
				t.Errorf("Provider = %q, want %q", commandConfig.Provider, testCase.wantProvider)
			}
			if len(commandConfig.APIKeys) != 1 || commandConfig.APIKeys[0] != testCase.wantAPIKey {
				t.Errorf("API keys = %v, want [%q]", commandConfig.APIKeys, testCase.wantAPIKey)
			}
			if commandConfig.BaseURL != testCase.wantBaseURL {
				t.Errorf("Base URL = %q, want %q", commandConfig.BaseURL, testCase.wantBaseURL)
			}
			if testCase.wantModel != "" && commandConfig.ModelName != testCase.wantModel {
				t.Errorf("Model name = %q, want %q", commandConfig.ModelName, testCase.wantModel)
			}

			factory := &providers.ProviderFactory{}
			provider, errNewProvider := factory.NewProvider(commandConfig)
			if errNewProvider != nil {
				t.Fatalf("NewProvider returned an error: %v", errNewProvider)
			}
			if provider.GetName() != testCase.wantProvider {
				t.Errorf("Provider factory created %q, want %q", provider.GetName(), testCase.wantProvider)
			}
		})
	}
}

func TestInvalidUnifiedAPIKeysDoNotFallBackAfterProviderInference(t *testing.T) {
	clearConfigurationEnvironment(t)
	t.Setenv(config.EnvAPIKey, " , ")
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	t.Setenv("OPENAI_API_KEY", "openai-key")

	commandConfig := config.NewConfig()
	command := newRootCommandWithConfig(commandConfig)
	if errParse := command.Flags().Parse([]string{"--model", "GPT-test"}); errParse != nil {
		t.Fatalf("Failed to parse command-line flags: %v", errParse)
	}
	if errPreRun := command.PreRunE(command, nil); errPreRun != nil {
		t.Fatalf("PreRunE returned an error: %v", errPreRun)
	}

	if commandConfig.Provider != "openai" {
		t.Errorf("Provider = %q, want %q", commandConfig.Provider, "openai")
	}
	if len(commandConfig.APIKeys) != 0 {
		t.Errorf("API keys = %v, want no legacy API keys", commandConfig.APIKeys)
	}
}

func TestExplicitEmptyCLIValuesOverrideAllEnvironmentVariables(t *testing.T) {
	clearConfigurationEnvironment(t)
	t.Setenv(config.EnvProvider, "openai")
	t.Setenv(config.EnvBaseURL, "https://unified.example.com")
	t.Setenv(config.EnvAPIKey, "unified-key")
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", "https://gemini.example.com")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("OPENAI_BASE_URL", "https://openai.example.com")

	commandConfig := config.NewConfig()
	command := newRootCommandWithConfig(commandConfig)
	if errParse := command.Flags().Parse([]string{"--base-url=", "--api-key="}); errParse != nil {
		t.Fatalf("Failed to parse command-line flags: %v", errParse)
	}
	if errPreRun := command.PreRunE(command, nil); errPreRun != nil {
		t.Fatalf("PreRunE returned an error: %v", errPreRun)
	}

	if commandConfig.BaseURL != "" {
		t.Errorf("Base URL = %q, want an explicit empty value", commandConfig.BaseURL)
	}
	if len(commandConfig.APIKeys) != 0 {
		t.Errorf("API keys = %v, want an explicit empty value", commandConfig.APIKeys)
	}
}

func TestRootCommandInstancesKeepIndependentConfiguration(t *testing.T) {
	clearConfigurationEnvironment(t)
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	t.Setenv("GOOGLE_GEMINI_BASE_URL", "https://gemini.example.com")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("OPENAI_BASE_URL", "https://openai.example.com")

	firstCommand := newRootCommand()
	secondCommand := newRootCommand()

	if errParse := firstCommand.Flags().Parse([]string{"--provider", "openai"}); errParse != nil {
		t.Fatalf("Failed to parse first command flags: %v", errParse)
	}
	if errPreRun := firstCommand.PreRunE(firstCommand, nil); errPreRun != nil {
		t.Fatalf("First PreRunE returned an error: %v", errPreRun)
	}

	firstBaseURL, errFirstBaseURL := firstCommand.Flags().GetString("base-url")
	if errFirstBaseURL != nil {
		t.Fatalf("Failed to read first command base URL: %v", errFirstBaseURL)
	}
	secondBaseURL, errSecondBaseURL := secondCommand.Flags().GetString("base-url")
	if errSecondBaseURL != nil {
		t.Fatalf("Failed to read second command base URL: %v", errSecondBaseURL)
	}
	if firstBaseURL != "https://openai.example.com" {
		t.Errorf("First command base URL = %q, want OpenAI legacy URL", firstBaseURL)
	}
	if secondBaseURL != "https://gemini.example.com" {
		t.Errorf("Second command base URL = %q, want unchanged Gemini legacy URL", secondBaseURL)
	}

	if errParse := secondCommand.Flags().Parse([]string{"--base-url", "https://second.example.com"}); errParse != nil {
		t.Fatalf("Failed to parse second command flags: %v", errParse)
	}
	if errPreRun := secondCommand.PreRunE(secondCommand, nil); errPreRun != nil {
		t.Fatalf("Second PreRunE returned an error: %v", errPreRun)
	}

	firstBaseURL, errFirstBaseURL = firstCommand.Flags().GetString("base-url")
	if errFirstBaseURL != nil {
		t.Fatalf("Failed to reread first command base URL: %v", errFirstBaseURL)
	}
	if firstBaseURL != "https://openai.example.com" {
		t.Errorf("First command base URL changed to %q after running second command", firstBaseURL)
	}
}

func clearConfigurationEnvironment(t *testing.T) {
	t.Helper()
	for _, environmentName := range []string{
		config.EnvProvider,
		config.EnvOpenAIProtocol,
		config.EnvBaseURL,
		config.EnvAPIKey,
		config.EnvModel,
		"GEMINI_API_KEY",
		"GOOGLE_GEMINI_BASE_URL",
		"OPENAI_API_KEY",
		"OPENAI_BASE_URL",
	} {
		t.Setenv(environmentName, "")
	}
}

func TestExpandInputFilesSupportsGlobsAndExplicitFiles(t *testing.T) {
	tempDir := t.TempDir()
	firstFile := filepath.Join(tempDir, "episode-01.srt")
	secondFile := filepath.Join(tempDir, "episode-02.srt")
	mkvFile := filepath.Join(tempDir, "movie.mkv")
	for _, filePath := range []string{firstFile, secondFile, mkvFile} {
		if errWrite := os.WriteFile(filePath, []byte("test"), 0644); errWrite != nil {
			t.Fatalf("Failed to create test input %s: %v", filePath, errWrite)
		}
	}

	inputFiles, errExpand := expandInputFiles([]string{
		filepath.Join(tempDir, "episode-*.srt"),
		mkvFile,
		firstFile,
	})
	if errExpand != nil {
		t.Fatalf("expandInputFiles() error = %v", errExpand)
	}

	wantFiles := []string{firstFile, secondFile, mkvFile}
	if len(inputFiles) != len(wantFiles) {
		t.Fatalf("expandInputFiles() returned %d files, want %d: %v", len(inputFiles), len(wantFiles), inputFiles)
	}
	for fileIndex, wantFile := range wantFiles {
		if inputFiles[fileIndex] != wantFile {
			t.Errorf("input file %d = %q, want %q", fileIndex, inputFiles[fileIndex], wantFile)
		}
	}
}

func TestExpandInputFilesRejectsUnmatchedGlob(t *testing.T) {
	_, errExpand := expandInputFiles([]string{filepath.Join(t.TempDir(), "*.mkv")})
	if errExpand == nil {
		t.Fatal("expandInputFiles() returned no error for an unmatched glob")
	}
}

func TestExpandInputFilesPrefersExistingLiteralPathAndDeduplicatesSymlink(t *testing.T) {
	tempDir := t.TempDir()
	literalPath := filepath.Join(tempDir, "episode[1].srt")
	if errWrite := os.WriteFile(literalPath, []byte("test"), 0644); errWrite != nil {
		t.Fatalf("Failed to create literal input: %v", errWrite)
	}
	symlinkPath := filepath.Join(tempDir, "episode-link.srt")
	if errSymlink := os.Symlink(literalPath, symlinkPath); errSymlink != nil {
		t.Fatalf("Failed to create symlink: %v", errSymlink)
	}

	inputFiles, errExpand := expandInputFiles([]string{literalPath, symlinkPath})
	if errExpand != nil {
		t.Fatalf("expandInputFiles() error = %v", errExpand)
	}
	if len(inputFiles) != 1 || inputFiles[0] != literalPath {
		t.Errorf("expandInputFiles() = %v, want [%s]", inputFiles, literalPath)
	}
}

func TestCanonicalPathResolvesSymlinkedParentForMissingArtifact(t *testing.T) {
	tempDir := t.TempDir()
	realDir := filepath.Join(tempDir, "real")
	if errMkdir := os.Mkdir(realDir, 0755); errMkdir != nil {
		t.Fatalf("Failed to create real directory: %v", errMkdir)
	}
	aliasDir := filepath.Join(tempDir, "alias")
	if errSymlink := os.Symlink(realDir, aliasDir); errSymlink != nil {
		t.Fatalf("Failed to create directory symlink: %v", errSymlink)
	}

	realArtifact, errCanonicalReal := canonicalPath(filepath.Join(realDir, "future.srt"))
	if errCanonicalReal != nil {
		t.Fatalf("Failed to canonicalize real artifact: %v", errCanonicalReal)
	}
	aliasArtifact, errCanonicalAlias := canonicalPath(filepath.Join(aliasDir, "future.srt"))
	if errCanonicalAlias != nil {
		t.Fatalf("Failed to canonicalize alias artifact: %v", errCanonicalAlias)
	}
	if realArtifact != aliasArtifact {
		t.Errorf("Canonical paths differ: real=%q alias=%q", realArtifact, aliasArtifact)
	}
}

func TestValidateTranslationTaskPathsRejectsSharedArtifacts(t *testing.T) {
	tempDir := t.TempDir()
	srtPath := filepath.Join(tempDir, "movie.srt")
	mkvPath := filepath.Join(tempDir, "movie.mkv")
	for _, inputPath := range []string{srtPath, mkvPath} {
		if errWrite := os.WriteFile(inputPath, []byte("test"), 0644); errWrite != nil {
			t.Fatalf("Failed to create input: %v", errWrite)
		}
	}

	tasks := []translationTask{
		{filename: srtPath, translator: translator.NewTranslator(&config.Config{InputFile: srtPath, TargetLanguage: "Simplified Chinese"})},
		{filename: mkvPath, translator: translator.NewTranslator(&config.Config{InputFile: mkvPath, TargetLanguage: "Simplified Chinese"})},
	}
	if errValidate := validateTranslationTaskPaths(tasks); errValidate == nil {
		t.Fatal("validateTranslationTaskPaths() accepted tasks with shared artifacts")
	}
}

func TestValidateTranslationTaskPathsRejectsInputOverwrite(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "episode.srt")
	if errWrite := os.WriteFile(inputPath, []byte("test"), 0644); errWrite != nil {
		t.Fatalf("Failed to create input: %v", errWrite)
	}
	tasks := []translationTask{{
		filename: inputPath,
		translator: translator.NewTranslator(&config.Config{
			InputFile:      inputPath,
			OutputFile:     inputPath,
			TargetLanguage: "French",
		}),
	}}
	if errValidate := validateTranslationTaskPaths(tasks); errValidate == nil {
		t.Fatal("validateTranslationTaskPaths() accepted an output that overwrites its input")
	}
}

func TestDefaultModelForProvider(t *testing.T) {
	tests := []struct {
		name           string
		provider       string
		openAIProtocol string
		want           string
	}{
		{name: "OpenAI Responses", provider: "openai", openAIProtocol: "responses", want: "gpt-5"},
		{name: "OpenAI Chat Completions", provider: "openai", openAIProtocol: "chat-completions", want: "gpt-4o"},
		{name: "OpenAI legacy empty protocol", provider: "openai", want: "gpt-4o"},
		{name: "Gemini", provider: "gemini", openAIProtocol: "responses", want: "gemini-3.5-flash"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := defaultModelForProvider(testCase.provider, testCase.openAIProtocol)
			if got != testCase.want {
				t.Errorf("defaultModelForProvider() = %q, want %q", got, testCase.want)
			}
		})
	}
}
