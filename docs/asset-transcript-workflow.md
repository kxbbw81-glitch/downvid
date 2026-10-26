# 链接解析后的素材与完整台词工作流

状态：已实现功能源码，尚未完成构建与真实视频验收。本分支从 Fork `kxbbw81-glitch/downvid` 的 `main` 创建。

## 用户可操作的内容

- 链接解析后显示作者发布正文和封面。可分别复制、保存发布正文 TXT 和封面图片。
- 视频沿用现有下载队列；原声可以单独下载。抖音、快手等仅提供视频直链的来源，会先下载视频，再由 FFmpeg 提取完整音轨为 M4A。此音轨同时包含人声、配乐和背景音，不做声源分离。
- 点击「生成完整台词」，使用本地 whisper.cpp 对音轨转写并保存 TXT 与带时间戳的 SRT；界面中可编辑、复制、另存台词 TXT。发布正文和视频口播分开显示。首次在 Windows x64 使用时自动下载经摘要校验的 whisper.cpp 程序及多语言 base 模型（约 142 MiB）；其他平台可手动选择已安装的程序与模型。需要 FFmpeg。
- 点击「全部下载」，按视频、原声、封面、发布正文、口播转写的顺序执行，并显示各项成功、失败或不可用状态。已有视频文件可复用来提取原声和转写，避免再次下载媒体。某项失败时其余项继续执行。输出都存入用户当前选定的下载目录，同名文本及图片使用递增后缀避免覆盖。

## 实现位置

- Go/Wails：`app/bindings_assets.go` 管理封面、文本及完整音轨；`app/bindings_transcript.go` 获取音频、转 WAV、调用 whisper-cli、保存 TXT/SRT 及取消；`app/bindings_whisper.go` 安装本地程序与模型；`app/bindings_download.go` 修复直链音频内容与扩展名不符的问题。
- 前端：`app/frontend/src/components/ParseResult.svelte` 提供单项按钮、台词编辑和组合流程；`app/frontend/src/lib/backend.ts` 提供 Wails 调用；`app/frontend/src/lib/store.svelte.ts` 支持直链原声选项；`app/frontend/src/lib/i18n.svelte.ts` 提供中英文案。
- `app/bindings_assets_test.go` 包含输出路径及重复转写文件的回归用例。

## 验证关口

当前执行环境启动任何 shell 命令都会因沙盒目录访问被拒绝。因此下述关口**尚未运行，源码和 PR 不视为已验收的安装包**：

1. 在 `app/` 运行 `gofmt -w bindings_assets.go bindings_assets_test.go bindings_transcript.go bindings_whisper.go bindings_download.go`，随后运行 `go test ./...` 和 `go vet ./...`。
2. 安装前端依赖，运行 `pnpm --dir app/frontend check`、项目的 i18n 校验与 Wails Windows 构建，并检查生成绑定包含新增方法。
3. 在 Windows x64 上用可访问的抖音、快手、有口播和无口播的真实素材检查音频可播放、时长和背景声、封面可离线打开、TXT/SRT 内容、取消、重复保存、单项失败继续与批量结果。
4. 核查本 Fork 的构建产物名称、更新地址与安装包来源，再考虑合并和发布。

限制：口播转写是语音识别，识别效果依赖原始声音、语言和模型；没有人声时不会凭发布正文生成台词。某些平台内容需要 Cookie 或最新提取器，解析能力以本仓库已有适配器为准。当前 Windows x64 提供自动依赖下载，其他平台使用手动路径。
