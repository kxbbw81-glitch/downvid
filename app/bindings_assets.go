package main

// 解析后的封面与文本导出。所有写入均由桌面后端完成，避免 WebView 的下载权限差异。
import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
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
	target := filepath.Join(dir, name)
	// 不覆盖同名素材。由上层传入稳定 ID；同一视频重复保存时保留原文件。
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(err, os.ErrExist) {
		return AssetSaveResult{OK: true, Path: target}
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
