# Gemini SRT Translator Go

[English](README.md) | 中文

Gemini SRT Translator Go - 一个使用 Google Gemini AI 翻译字幕文件的强大工具。

## 功能特性

- 🔤 **SRT 翻译**: 将 `.srt` 字幕文件翻译成 Google Gemini AI 支持的多种语言
- ⏱️ **时间和格式**: 保持原始文件的精确时间戳和基本的 SRT 格式
- 💾 **快速恢复**: 轻松从上次中断的地方恢复翻译
- 🧠 **高级 AI**: 利用思考和推理能力，实现更符合上下文的准确翻译
- 🖥️ **CLI 支持**: 功能齐全的命令行界面，便于自动化和脚本编写
- ⚙️ **可定制**: 可调整模型参数、批量大小，并可访问其他高级设置
- 📜 **描述支持**: 添加描述以指导 AI 使用特定的术语或上下文
- 📋 **交互功能**: 交互式模型选择和自动帮助显示
- 📝 **日志记录**: 可选择保存进度和思考过程日志以供审查

## 安装

### 前提条件

- Go 1.23 或更高版本

### 从源码构建

```bash
git clone https://github.com/luispater/gemini-srt-translator-go.git
cd gemini-srt-translator-go
go mod tidy
go build -o gst ./cmd
```

## 配置

### 获取您的 API 密钥

1. 前往 [Google AI Studio](https://aistudio.google.com/apikey)
2. 使用您的 Google 帐户登录
3. 点击 **Generate API Key**
4. 复制并妥善保管您的密钥

### 设置您的 API 密钥

设置 `GEMINI_API_KEY` 环境变量，可以使用逗号分隔的密钥以获得额外的配额：

**macOS/Linux:**
```bash
export GEMINI_API_KEY="your_first_api_key_here,your_second_api_key_here"
```

**Windows (命令提示符):**
```bash
set GEMINI_API_KEY=your_first_api_key_here,your_second_api_key_here
```

**Windows (PowerShell):**
```powershell
$env:GEMINI_API_KEY="your_first_api_key_here,your_second_api_key_here"
```

## 使用方法

### 帮助和用法

```bash
# 显示帮助
./gst --help
# 或者直接运行而不输入参数
./gst
```

### 命令行界面

#### 基本翻译

```bash
# 使用环境变量 (推荐)
export GEMINI_API_KEY="your_api_key_here"
./gst subtitle.srt -l "Simplified Chinese"

# 使用命令行参数和多个密钥
./gst subtitle.srt -l "Simplified Chinese" -k "YOUR_FIRST_API_KEY,YOUR_SECOND_API_KEY"

# 设置输出文件名
./gst subtitle.srt -l "Simplified Chinese" -o translated_subtitle.srt

# 交互式模型选择
./gst subtitle.srt -l "Brazilian Portuguese" --interactive

# 从指定行开始恢复翻译
./gst subtitle.srt -l "Simplified Chinese" --start-line 20

# 禁止输出
./gst subtitle.srt -l "Simplified Chinese" --quiet
```

#### 高级选项

```bash
# 功能齐全的翻译，使用自定义设置
./gst input.srt \
  -l "Simplified Chinese" \
  -k YOUR_API_KEY \
  -o output_french.srt \
  --model gemini-2.5-pro \
  --batch-size 150 \
  --temperature 0.7 \
  --description "医疗电视剧，请使用医疗术语" \
  --progress-log
```

#### 交互式模型选择

使用交互模式查看并选择可用模型：

```bash
./gst subtitle.srt -l "Simplified Chinese" --interactive
```

## 配置选项

### 核心参数

- `GeminiAPIKeys`: Gemini API 密钥数组 (从逗号分隔的字符串解析)
- `TargetLanguage`: 翻译的目标语言
- `InputFile`: 输入 SRT 文件的路径
- `OutputFile`: 输出已翻译 SRT 文件的路径
- `StartLine`: 开始翻译的行号
- `Description`: 翻译的附加说明
- `BatchSize`: 每个批次处理的字幕数量

### 模型参数

- `ModelName`: 要使用的 Gemini 模型 (默认: "gemini-2.5-flash")
- `Temperature`: 控制输出的随机性 (0.0-2.0)
- `TopP`: Nucleus 采样参数 (0.0-1.0)
- `TopK`: Top-k 采样参数 (>=0)
- `Streaming`: 启用流式响应 (默认: true)
- `Thinking`: 启用思考能力 (默认: true)
- `ThinkingBudget`: 思考过程的令牌预算 (0-24576)

### 用户选项

- `FreeQuota`: 表明您正在使用免费配额 (影响速率限制)
- `UseColors`: 启用彩色终端输出
- `ProgressLog`: 启用将进度记录到文件
- `QuietMode`: 禁止所有输出
- `Resume`: 自动恢复中断的翻译

## 项目结构

```
gemini-srt-translator-go/
├── cmd/                  # 命令行界面
│   └── main.go
├── internal/             # 内部包
│   ├── translator/       # 核心翻译逻辑
│   ├── logger/           # 日志和进度显示
│   └── helpers/          # Gemini API 助手
├── pkg/                  # 公共包
│   ├── config/           # 配置管理
│   ├── errors/           # 错误处理
│   └── srt/              # SRT 解析和格式化
└── test/                 # 测试文件
```

## 主要依赖

- `github.com/spf13/cobra`: CLI 框架
- `google.golang.org/genai`: 官方 Gemini AI 客户端
- `golang.org/x/term`: 终端密码输入

## 测试

运行测试套件：

```bash
go test ./...
```

运行带覆盖率的测试：

```bash
go test -cover ./...
```

## 开发

### 构建

```bash
go build -o gst ./cmd
```

### 交叉编译

```bash
# Windows
GOOS=windows GOARCH=amd64 go build -o gst.exe ./cmd

# macOS
GOOS=darwin GOARCH=amd64 go build -o gst-macos ./cmd

# Linux
GOOS=linux GOARCH=amd64 go build -o gst-linux ./cmd
```

## 特性说明

### 翻译流程

1. **输入验证**: 检查文件、API 密钥和参数
2. **模型验证**: 验证选定的 Gemini 模型可用性
3. **令牌管理**: 获取模型令牌限制并验证批量大小
4. **批量翻译**: 以可配置的批次处理字幕
5. **进度跟踪**: 保存进度以便可恢复翻译
6. **输出生成**: 写入具有正确格式的翻译 SRT

### 错误处理

- API 错误会在有多个密钥可用时触发密钥轮换
- 文件验证在处理开始前进行
- 每次成功批处理后保存进度
- 完成后清理临时文件

### 多 API 密钥支持

- 支持多个 API 密钥轮换使用
- 自动处理配额限制和错误恢复
- 提高免费用户的处理能力

## 许可证

根据 MIT 许可证分发。有关详细信息，请参阅 [LICENSE](LICENSE) 文件。

## 贡献

欢迎贡献！请随时提交拉取请求。

## 支持

如果您遇到任何问题或有疑问，请查看文档或在存储库中创建问题。