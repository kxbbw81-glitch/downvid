package main

// 显式开启时运行真实 FFmpeg 和 whisper.cpp，不把桩程序当语音识别验收。
import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssetsRealMediaIntegration(t *testing.T) {
	if os.Getenv("DOWNVID_ASSET_INTEGRATION") != "1" {
		t.Skip("需要显式开启真实媒体集成测试")
	}
	ffmpeg := os.Getenv("DOWNVID_FFMPEG")
	if !fileExists(ffmpeg) {
		t.Fatal("缺少集成测试 FFmpeg")
	}
	root := t.TempDir()
	a := NewApp()
	a.ctx = context.Background()
	a.dataDir = root
	a.downloadsDir = root
	depsCacheMu.Lock()
	depsCache = []Dependency{{Kind: "ffmpeg", Found: true, Path: ffmpeg}}
	depsCachedAt = time.Now()
	depsCacheMu.Unlock()
	t.Cleanup(invalidateDepsCache)
	setup := a.PrepareWhisper("integration-setup")
	if !setup.OK {
		t.Fatal(setup.Error)
	}
	depsCacheMu.Lock()
	depsCachedAt = time.Now()
	depsCacheMu.Unlock()
	res, err := http.Get("https://raw.githubusercontent.com/ggml-org/whisper.cpp/b5454/samples/jfk.wav")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("语音样本 HTTP %d", res.StatusCode)
	}
	wav := filepath.Join(root, "speech.wav")
	f, err := os.Create(wav)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(f, io.LimitReader(res.Body, 5<<20))
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(root, "speech-with-background.mp4")
	cmd := exec.Command(ffmpeg, "-nostdin", "-y", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=blue:s=160x90:r=10", "-i", wav, "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=16000", "-filter_complex", "[1:a][2:a]amix=inputs=2:duration=first:weights=1 0.1[a]", "-map", "0:v", "-map", "[a]", "-c:v", "mpeg4", "-c:a", "aac", "-shortest", media)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("生成含人声与背景音的真实样本：%v %s", err, out)
	}
	first := a.ExtractAudio(media, root, "sample")
	second := a.ExtractAudio(media, root, "sample")
	if !first.OK || !second.OK || first.Path == second.Path {
		t.Fatalf("单项提取与重复保存：%+v %+v", first, second)
	}
	pcm, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-i", first.Path, "-vn", "-ar", "16000", "-ac", "1", "-f", "s16le", "pipe:1").Output()
	if err != nil || len(pcm) < 16000*2*5 {
		t.Fatalf("导出音频必须真实可解码且保留完整声音：%v (%d bytes)", err, len(pcm))
	}
	if err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-i", first.Path, "-map", "0:v:0", "-f", "null", "-").Run(); err == nil {
		t.Fatal("音频文件不应仍包含视频轨")
	}
	transcript := a.Transcribe(TranscriptRequest{TaskID: "integration-speech", URL: "https://example.test/sample", SourceFile: media, OutputDir: root, VideoID: "sample", WhisperPath: setup.WhisperPath, ModelPath: setup.ModelPath})
	if !transcript.OK || !strings.Contains(strings.ToLower(transcript.Text), "country") {
		t.Fatalf("真实人声转写未通过：%+v", transcript)
	}
	for _, path := range []string{transcript.TxtPath, transcript.SrtPath} {
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			t.Fatalf("转写导出缺失：%q %v", path, err)
		}
	}
	srt, _ := os.ReadFile(transcript.SrtPath)
	if !strings.Contains(string(srt), "-->") {
		t.Fatal("SRT 必须包含真实时间戳")
	}
	server := httptest.NewServer(http.FileServer(http.Dir(root)))
	defer server.Close()
	completed := make(chan DownloadProgress, 1)
	a.emitHook = func(_ string, payload any) {
		if p, ok := payload.(DownloadProgress); ok && (p.Status == "completed" || p.Status == "error") {
			select {
			case completed <- p:
			default:
			}
		}
	}
	depsCacheMu.Lock()
	depsCachedAt = time.Now()
	depsCacheMu.Unlock()
	started := a.startDirectDownload(DownloadRequest{TaskID: "direct-audio", URL: server.URL + "/speech-with-background.mp4", DirectURL: server.URL + "/speech-with-background.mp4", DownloadMode: "audio", OutputDir: root, Title: "direct-audio", FilenameTemplate: "%(title)s.%(ext)s"})
	if !started.OK {
		t.Fatal(started.Error)
	}
	select {
	case result := <-completed:
		if result.Status != "completed" || !fileExists(result.FilePath) {
			t.Fatalf("直链原声下载：%+v", result)
		}
		if _, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-i", result.FilePath, "-f", "null", "-").CombinedOutput(); err != nil {
			t.Fatalf("直链原声不可解码：%v", err)
		}
	case <-time.After(90 * time.Second):
		t.Fatal("直链下载未终结")
	}
}
