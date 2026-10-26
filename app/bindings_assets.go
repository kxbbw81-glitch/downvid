package main

// 解析后的封面与文本导出。所有写入均由桌面后端完成，避免 WebView 的下载权限差异。
import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type AssetSaveResult struct {
	OK bool `json:"ok"`
	Path string `json:"path,omitempty"`
	Error string `json:"error,omitempty"`
}

var unsafeAssetName = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func assetName(s string) string {
	s = strings.Trim(unsafeAssetName.ReplaceAllString(s, "-"), " .-")
	r := []rune(s)
	if len(r) > 80 {
		s = string(r[:80])
	}
	if s == "" {
		return "video"
	}
	return s
}

func saveAsset(dir, name string, data []byte) AssetSaveResult {
	if dir == "" {
		return AssetSaveResult{Error: "请选择下载目录"}
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return AssetSaveResult{Error: err.Error()}
	}
	// 保留已有文件，为重试与重新识别生成独立文件，避免把旧内容当成本次结果。
	var target string
	var file *os.File
	var err error
	for i := 0; i < 1000; i++ {
		candidate := name
		if i > 0 {
			ext := filepath.Ext(name)
			candidate = strings.TrimSuffix(name, ext) + fmt.Sprintf("-%d", i+1) + ext
		}
		target = filepath.Join(dir, candidate)
		file, err = os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if !errors.Is(err, os.ErrExist) {
			break
		}
	}
	if err != nil {
		return AssetSaveResult{Error: err.Error()}
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(target)
		return AssetSaveResult{Error: fmt.Sprintf("保存失败：%v %v", writeErr, closeErr)}
	}
	return AssetSaveResult{OK: true, Path: target}
}

// SaveCover 保存已经由 FetchImage 抓取的封面。最多接受 8 MiB 图像数据。
func (a *App) SaveCover(dataURL, outputDir, videoID string) AssetSaveResult {
	const maxBytes = 8 << 20
	typeByPrefix := map[string]string{
		"data:image/jpeg;base64,": ".jpg",
		"data:image/png;base64,": ".png",
		"data:image/webp;base64,": ".webp",
		"data:image/gif;base64,": ".gif",
		"data:image/avif;base64,": ".avif",
	}
	for prefix, ext := range typeByPrefix {
		if !strings.HasPrefix(dataURL, prefix) {
			continue
		}
		encoded := strings.TrimPrefix(dataURL, prefix)
		if len(encoded) > base64.StdEncoding.EncodedLen(maxBytes)+4 {
			return AssetSaveResult{Error: "封面超过大小上限"}
		}
		imageBytes, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(imageBytes) == 0 || len(imageBytes) > maxBytes {
			return AssetSaveResult{Error: "封面数据无效"}
		}
		return saveAsset(outputDir, assetName(videoID)+"-cover"+ext, imageBytes)
	}
	return AssetSaveResult{Error: "封面尚未加载或格式不受支持"}
}

// SaveText 保存作者发布正文或转写的纯文本。
func (a *App) SaveText(content, outputDir, videoID, kind string) AssetSaveResult {
	if strings.TrimSpace(content) == "" {
		return AssetSaveResult{Error: "没有可保存的文字"}
	}
	if kind != "post" && kind != "transcript" {
		return AssetSaveResult{Error: "不支持的文本类型"}
	}
	return saveAsset(outputDir, assetName(videoID)+"-"+kind+".txt", []byte(content))
}

// ExtractAudio 从已下载的视频提取完整原声，供「全部下载」复用视频素材。
func (a *App) ExtractAudio(sourceFile, outputDir, videoID string) AssetSaveResult {
	if !fileExists(sourceFile) {
		return AssetSaveResult{Error: "视频文件尚未下载完成"}
	}
	ffmpegPath, ok := a.pathOf("ffmpeg")
	if !ok {
		return AssetSaveResult{Error: "提取原声需要 FFmpeg"}
	}
	if outputDir == "" {
		return AssetSaveResult{Error: "请选择下载目录"}
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return AssetSaveResult{Error: err.Error()}
	}
	name := assetName(videoID) + "-original-audio"
	output := filepath.Join(outputDir, name+".m4a")
	for i := 2; fileExists(output) && i < 1000; i++ {
		output = filepath.Join(outputDir, fmt.Sprintf("%s-%d.m4a", name, i))
	}
	staging := output + ".converting.m4a"
	defer os.Remove(staging)
	cmd := exec.Command(ffmpegPath, "-nostdin", "-loglevel", "error",
		"-i", sourceFile, "-vn", "-c:a", "aac", "-b:a", "192k", staging)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return AssetSaveResult{Error: fmt.Sprintf("提取原声失败：%v %s", err, strings.TrimSpace(string(out)))}
	}
	if err := os.Rename(staging, output); err != nil {
		return AssetSaveResult{Error: fmt.Sprintf("保存原声失败：%v", err)}
	}
	return AssetSaveResult{OK: true, Path: output}
}
