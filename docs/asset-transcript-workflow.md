# MediaHarvest Windows 素材完整版

基于 DownVid 的非官方 Wails 增强版，代码位于个人 Fork 的 `feat/asset-transcript-workflow` 分支。保留上游 MIT 版权与许可，不接收官方应用更新，使用独立数据目录。

## 使用

1. 解压 Windows x64 程序包，运行 `MediaHarvest.exe`。
2. 首次打开，在「设置 → 运行依赖」检测并安装缺失的 yt-dlp / FFmpeg。
3. 粘贴链接，点击「解析」。解析成功后选择视频清晰度。
4. 视频、音频可以分别下载；封面可以单独保存。
5. 点击「提取完整台词」。首次使用会安装官方 whisper.cpp 和多语言 base 模型（约 142 MiB），之后在本机识别。支持取消、复制、编辑和 TXT/SRT 导出。
6. 点击「全部下载」，逐项保存视频、完整原声、封面、发布正文和完整台词。失败不阻断后续素材，失败项可单独重试。

语音转写需要处理音频，不能保证在链接解析完成的瞬间产生台词。发布正文与完整台词分开展示。音频包含原视频的人声、音乐和环境声，不做人声分离。

## 实际验证

Windows 验收工作流负责前端类型检查、123 项前端测试、前端构建、Wails 绑定生成、Go 格式/静态检查和全部包测试。真实媒体集成测试实际安装 whisper.cpp 和多语言模型，使用公开语音混合背景音生成 MP4，检查音频提取、无视频轨、真实转写、TXT/SRT 和直链音频下载。

已成功完成的首轮完整验收：[Windows run 40477120109](https://github.com/kxbbw81-glitch/downvid/actions/runs/40477120109)，对应提交 `8dfc5762ccd87f1e240118385b6f99117b770306`。后续修复以分支最新 Windows 验收记录为准。

公开平台的在线适配与桌面交互仍需实际链接/设备验证：尚未逐条确认抖音、快手、小红书、B 站等平台的登录限制和解析成功率。公开语音样本通过不代表所有平台链接均已通过。自动识别可能漏字、错字，不承诺逐字准确。

## 实现入口

- `app/bindings_assets.go`：封面、发布正文、TXT 和真实原声保存；同名导出不覆盖。
- `app/bindings_transcript.go`：取媒体、转 WAV、本地转写、TXT/SRT 和取消。
- `app/bindings_whisper.go`：首次安装及官方摘要校验，支持取消网络下载。
- `app/frontend/src/components/ParseResult.svelte`：单项保存、台词编辑、复制及组合任务。
- `.github/workflows/assets-windows.yml`：严格逐步失败检查、真实媒体验收和独立 Windows 产物。

## 构建

需要 Node.js 22、pnpm、Go 1.25+ 和 Wails CLI 2.16.0。先在 `app/frontend` 安装冻结依赖，再执行 `bash app/scripts/build.sh -platform windows/amd64`。必须保留内嵌依赖边车；仅执行裸 `go build` 不等于完整桌面分发包。

其他平台允许手动选择 whisper-cli 和多语言 ggml 模型；本次交付与真实识别验收针对 Windows x64。
