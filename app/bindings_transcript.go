package main

// 本地口播转写：把可访问的视频音轨转换为 16 kHz 单声道 WAV，再交给 whisper.cpp。
// 不上传音频。用户首次使用需要选择 whisper-cli 可执行文件和多语言 ggml 模型。
import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Downvid/downvid/app/internal/fetch"
	"github.com/Downvid/downvid/app/internal/netx"
)

type TranscriptRequest struct {
	TaskID      string `json:"taskId"`
	URL         string `json:"url"`
	DirectURL   string `json:"directUrl,omitempty"`
	SourceFile  string `json:"sourceFile,omitempty"`
	OutputDir   string `json:"outputDir"`
	VideoID     string `json:"videoId"`
	WhisperPath string `json:"whisperPath"`
	ModelPath   string `json:"modelPath"`
}

type TranscriptResult struct {
	OK      bool   `json:"ok"`
	Text    string `json:"text,omitempty"`
	TxtPath string `json:"txtPath,omitempty"`
	SrtPath string `json:"srtPath,omitempty"`
	Error   string `json:"error,omitempty"`
}

var transcriptTasks = struct {
	sync.Mutex
	cancels map[string]context.CancelFunc
}{cancels: make(map[string]context.CancelFunc)}

func (a *App) CancelTranscript(taskID string) bool {
	transcriptTasks.Lock()
	cancel := transcriptTasks.cancels[taskID]
	transcriptTasks.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (a *App) Transcribe(req TranscriptRequest) TranscriptResult {
	if req.TaskID == "" || req.URL == "" || req.OutputDir == "" {
		return TranscriptResult{Error: "缺少视频链接、任务编号或保存目录"}
	}
	if req.WhisperPath == "" {
		if p, err := exec.LookPath("whisper-cli"); err == nil {
			req.WhisperPath = p
		}
	}
	if !fileExists(req.WhisperPath) || !fileExists(req.ModelPath) {
		return TranscriptResult{Error: "请先选择 whisper-cli 程序和多语言 ggml 模型"}
	}
	ffmpegPath, ok := a.pathOf("ffmpeg")
	if !ok {
		return TranscriptResult{Error: "转写需要 FFmpeg，请在设置中的运行依赖安装"}
	}
	ctx, cancel := context.WithCancel(context.Background())
	transcriptTasks.Lock()
	if transcriptTasks.cancels[req.TaskID] != nil {
		transcriptTasks.Unlock()
		cancel()
		return TranscriptResult{Error: "任务已经在转写"}
	}
	transcriptTasks.cancels[req.TaskID] = cancel
	transcriptTasks.Unlock()
	defer func() {
		transcriptTasks.Lock()
		delete(transcriptTasks.cancels, req.TaskID)
		transcriptTasks.Unlock()
		cancel()
	}()

	work, err := os.MkdirTemp("", "downvid-transcript-")
	if err != nil {
		return TranscriptResult{Error: err.Error()}
	}
	defer os.RemoveAll(work)

	source := req.SourceFile
	if source != "" && !fileExists(source) {
		source = ""
	}
	if source == "" {
		if req.DirectURL != "" {
			client, clientErr := netx.Client(a.currentProxy(), 0)
			if clientErr != nil {
				return TranscriptResult{Error: clientErr.Error()}
			}
			media := filepath.Join(work, "media.mp4")
			_, err = (fetch.Direct{}).Download(ctx, fetch.DirectOptions{
				CDNURL: req.DirectURL, PageURL: req.URL, OutputPath: media,
				Connections: 4, Client: client,
			})
			source = media
		} else {
			ytdlp := a.ResolveYtdlp()
			if ytdlp == "" {
				return TranscriptResult{Error: "没有找到 yt-dlp"}
			}
			args := []string{"--no-playlist", "--no-warnings", "-f", "bestaudio/best",
				"--print", "after_move:filepath", "-o", filepath.Join(work, "media.%(ext)s")}
			if proxy := a.currentProxy(); proxy != "" {
				args = append(args, "--proxy", proxy)
			}
			args = append(args, req.URL)
			args = append(a.cookieArgs(), args...)
			cmd := exec.CommandContext(ctx, ytdlp, args...)
			raw, downloadErr := cmd.CombinedOutput()
			if downloadErr != nil {
				if ctx.Err() != nil {
					return TranscriptResult{Error: "转写已取消"}
				}
				return TranscriptResult{Error: fmt.Sprintf("获取音频失败：%v %s", downloadErr, strings.TrimSpace(string(raw)))}
			}
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			source = strings.TrimSpace(lines[len(lines)-1])
		}
		if err != nil {
			if ctx.Err() != nil {
				return TranscriptResult{Error: "转写已取消"}
			}
			return TranscriptResult{Error: fmt.Sprintf("获取视频失败：%v", err)}
		}
	}
	if !fileExists(source) {
		return TranscriptResult{Error: "没有获得可用的媒体文件"}
	}

	wav := filepath.Join(work, "speech.wav")
	cmd := exec.CommandContext(ctx, ffmpegPath, "-nostdin", "-loglevel", "error",
		"-i", source, "-vn", "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav)
	if out, convErr := cmd.CombinedOutput(); convErr != nil {
		if ctx.Err() != nil {
			return TranscriptResult{Error: "转写已取消"}
		}
		return TranscriptResult{Error: fmt.Sprintf("提取音轨失败：%v %s", convErr, strings.TrimSpace(string(out)))}
	}

	base := filepath.Join(work, "result")
	cmd = exec.CommandContext(ctx, req.WhisperPath, "-m", req.ModelPath, "-f", wav,
		"-l", "auto", "-otxt", "-osrt", "-of", base)
	if out, transErr := cmd.CombinedOutput(); transErr != nil {
		if ctx.Err() != nil {
			return TranscriptResult{Error: "转写已取消"}
		}
		return TranscriptResult{Error: fmt.Sprintf("语音识别失败：%v %s", transErr, strings.TrimSpace(string(out)))}
	}
	rawText, err := os.ReadFile(base + ".txt")
	if err != nil || strings.TrimSpace(string(rawText)) == "" {
		return TranscriptResult{Error: "未识别到口播内容"}
	}
	rawSrt, err := os.ReadFile(base + ".srt")
	if err != nil {
		return TranscriptResult{Error: fmt.Sprintf("字幕生成失败：%v", err)}
	}

	name := assetName(req.VideoID)
	txt := saveAsset(req.OutputDir, name+"-transcript.txt", rawText)
	if !txt.OK {
		return TranscriptResult{Error: txt.Error}
	}
	srt := saveAsset(req.OutputDir, name+"-transcript.srt", rawSrt)
	if !srt.OK {
		_ = os.Remove(txt.Path)
		return TranscriptResult{Error: srt.Error}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		_ = os.Remove(txt.Path)
		_ = os.Remove(srt.Path)
		return TranscriptResult{Error: "转写已取消"}
	}
	return TranscriptResult{OK: true, Text: string(rawText), TxtPath: txt.Path, SrtPath: srt.Path}
}
