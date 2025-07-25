# Gemini SRT Translator Go

English | [中文](README_CN.md)

Gemini SRT Translator Go - a powerful tool to translate subtitle files using Google Gemini AI.

## Features

- 🔤 **SRT Translation**: Translate `.srt` subtitle files to a wide range of languages supported by Google Gemini AI
- ⏱️ **Timing & Format**: Maintains exact timestamps and basic SRT formatting of the original file
- 💾 **Quick Resume**: Easily resume interrupted translations from where you left off
- 🧠 **Advanced AI**: Leverages thinking and reasoning capabilities for more contextually accurate translations
- 🖥️ **CLI Support**: Full command-line interface for easy automation and scripting
- ⚙️ **Customizable**: Tune model parameters, adjust batch size, and access other advanced settings
- 📜 **Description Support**: Add description to guide AI in using specific terminology or context
- 📋 **Interactive Features**: Interactive model selection and automatic help display
- 📝 **Logging**: Optional saving of progress and thinking process logs for review

## Installation

### Prerequisites

- Go 1.23 or later

### Build from Source

```bash
git clone https://github.com/luispater/gemini-srt-translator-go.git
cd gemini-srt-translator-go
go mod tidy
go build -o gst ./cmd
```

## Configuration

### Get Your API Key

1. Go to [Google AI Studio](https://aistudio.google.com/apikey)
2. Sign in with your Google account
3. Click on **Generate API Key**
4. Copy and keep your key safe

### Set Your API Key

Set the `GEMINI_API_KEY` environment variable with comma-separated keys for additional quota:

**macOS/Linux:**
```bash
export GEMINI_API_KEY="your_first_api_key_here,your_second_api_key_here"
```

**Windows (Command Prompt):**
```bash
set GEMINI_API_KEY=your_first_api_key_here,your_second_api_key_here
```

**Windows (PowerShell):**
```powershell
$env:GEMINI_API_KEY="your_first_api_key_here,your_second_api_key_here"
```

## Usage

### Help and Usage

```bash
# Show help
./gst --help
# or simply run without arguments
./gst
```

### Command Line Interface

#### Basic Translation

```bash
# Using environment variable (recommended)
export GEMINI_API_KEY="your_api_key_here"
./gst subtitle.srt -l "Simplified Chinese"

# Using command line argument with multiple keys
./gst subtitle.srt -l "Simplified Chinese" -k "YOUR_FIRST_API_KEY,YOUR_SECOND_API_KEY"

# Set output file name
./gst subtitle.srt -l "Simplified Chinese" -o translated_subtitle.srt

# Interactive model selection
./gst subtitle.srt -l "Brazilian Portuguese" --interactive

# Resume translation from a specific line
./gst subtitle.srt -l "Simplified Chinese" --start-line 20

# Suppress output
./gst subtitle.srt -l "Simplified Chinese" --quiet
```

#### Advanced Options

```bash
# Full-featured translation with custom settings
./gst input.srt \
  -l "Simplified Chinese" \
  -k YOUR_API_KEY \
  -o output_french.srt \
  --model gemini-2.5-pro \
  --batch-size 150 \
  --temperature 0.7 \
  --description "Medical TV series, use medical terminology" \
  --progress-log
```

#### Interactive Model Selection

Use interactive mode to see and select from available models:

```bash
./gst subtitle.srt -l "Simplified Chinese" --interactive
```

## Configuration Options

### Core Parameters

- `GeminiAPIKeys`: Array of Gemini API keys (parsed from comma-separated string)
- `TargetLanguage`: Target language for translation
- `InputFile`: Path to input SRT file
- `OutputFile`: Path to output translated SRT file
- `StartLine`: Line number to start translation from
- `Description`: Additional instructions for translation
- `BatchSize`: Number of subtitles to process in each batch

### Model Parameters

- `ModelName`: Gemini model to use (default: "gemini-2.5-flash")
- `Temperature`: Controls randomness in output (0.0-2.0)
- `TopP`: Nucleus sampling parameter (0.0-1.0)
- `TopK`: Top-k sampling parameter (>=0)
- `Streaming`: Enable streamed responses (default: true)
- `Thinking`: Enable thinking capability (default: true)
- `ThinkingBudget`: Token budget for thinking process (0-24576)

### User Options

- `FreeQuota`: Signal that you're using free quota (affects rate limiting)
- `UseColors`: Enable colored terminal output
- `ProgressLog`: Enable progress logging to file
- `QuietMode`: Suppress all output
- `Resume`: Automatically resume interrupted translations

## Project Structure

```
gemini-srt-translator-go/
├── cmd/                  # Command-line interface
│   └── main.go
├── internal/             # Internal packages
│   ├── translator/       # Core translation logic
│   ├── logger/           # Logging and progress display
│   └── helpers/          # Gemini API helpers
├── pkg/                  # Public packages
│   ├── config/           # Configuration management
│   ├── errors/           # Error handling
│   └── srt/              # SRT parsing and formatting
└── test/                 # Test files
```

## Main Dependencies

- `github.com/spf13/cobra`: CLI framework
- `google.golang.org/genai`: Official Gemini AI client
- `golang.org/x/term`: Terminal password input

## Testing

Run the test suite:

```bash
go test ./...
```

Run tests with coverage:

```bash
go test -cover ./...
```

## Development

### Building

```bash
go build -o gst ./cmd
```

### Cross-compilation

```bash
# Windows
GOOS=windows GOARCH=amd64 go build -o gst.exe ./cmd

# macOS
GOOS=darwin GOARCH=amd64 go build -o gst-macos ./cmd

# Linux
GOOS=linux GOARCH=amd64 go build -o gst-linux ./cmd
```

## Features Explained

### Translation Workflow

1. **Input Validation**: Check files, API keys, and parameters
2. **Model Validation**: Verify selected Gemini model availability
3. **Token Management**: Get model token limits and validate batch sizes
4. **Batch Translation**: Process subtitles in configurable batches
5. **Progress Tracking**: Save progress for resumable translations
6. **Output Generation**: Write translated SRT with proper formatting

### Error Handling

- API errors trigger key rotation when multiple keys available
- File validation occurs before processing begins
- Progress is saved after each successful batch
- Cleanup of temporary files on completion

### Multi-API Key Support

- Supports rotation through multiple API keys
- Automatic handling of quota limits and error recovery
- Enhanced processing capability for free quota users

## License

Distributed under the MIT License. See the [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Support

If you encounter any issues or have questions, please check the documentation or create an issue in the repository.