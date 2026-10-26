package main

// Windows x64 本地语音识别依赖：官方 whisper.cpp 构建 + 多语言 base 模型。
// 下载后校验官方摘要；不执行下载过程中的任何脚本。
import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Downvid/downvid/app/internal/netx"
)

const whisperArchiveURL = "https://github.com/ggml-org/whisper.cpp/releases/download/b5454/whisper-bin-x64.zip"
const whisperArchiveSHA256 = "6ba69e3482d7826214f90a6a9c84ca07782aec1e1d0c6a7c30c994fd5d816ccb"
const whisperModelURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin"
const whisperModelSHA1 = "465707469ff3a37a2b9b8d8f89f2f99de7299dac"

type WhisperSetup struct {
	OK bool `json:"ok"`
	WhisperPath string `json:"whisperPath,omitempty"`
	ModelPath string `json:"modelPath,omitempty"`
	Error string `json:"error,omitempty"`
}

func verifiedDownload(client *http.Client, url, destination, expected string, limit int64, sha256sum bool) error {
	res, err := client.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败：HTTP %d", res.StatusCode)
	}
	tmp := destination + ".partial"
	defer os.Remove(tmp)
	file, err := os.Create(tmp)
	if err != nil {
		return err
	}
	hasher := sha1.New()
	if sha256sum {
		hasher = sha256.New()
	}
	n, err := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(res.Body, limit+1))
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return fmt.Errorf("下载中断：%v %v", err, closeErr)
	}
	if n > limit {
		return errors.New("下载内容超过大小上限")
	}
	if hex.EncodeToString(hasher.Sum(nil)) != expected {
		return errors.New("下载文件校验失败")
	}
	return os.Rename(tmp, destination)
}

func unpackWhisper(archive, target string) (string, error) {
	raw, err := os.ReadFile(archive)
	if err != nil { return "", err }
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil { return "", err }
	stage := target + ".partial"
	if err := os.RemoveAll(stage); err != nil { return "", err }
	if err := os.MkdirAll(stage, 0755); err != nil { return "", err }
	defer os.RemoveAll(stage)
	relativeBinary := ""
	for _, entry := range reader.File {
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 { continue }
		clean := filepath.Clean(filepath.FromSlash(entry.Name))
		if filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return "", errors.New("程序包包含不安全路径")
		}
		dest := filepath.Join(stage, clean)
		if entry.FileInfo().IsDir() { if err := os.MkdirAll(dest, 0755); err != nil { return "", err }; continue }
		if entry.UncompressedSize64 > 100<<20 { return "", errors.New("程序包文件过大") }
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil { return "", err }
		rc, err := entry.Open()
		if err != nil { return "", err }
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
		if err != nil { _ = rc.Close(); return "", err }
		n, copyErr := io.Copy(out, io.LimitReader(rc, 100<<20+1))
		_ = rc.Close()
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil || n > 100<<20 {
			return "", errors.New("解压程序包失败")
		}
		if strings.EqualFold(filepath.Base(clean), "whisper-cli.exe") { relativeBinary = clean }
	}
	if relativeBinary == "" { return "", errors.New("程序包缺少 whisper-cli.exe") }
	if err := os.Rename(stage, target); err != nil { return "", err }
	return filepath.Join(target, relativeBinary), nil
}

// PrepareWhisper 首次使用时安装经校验的本地转写依赖。
func (a *App) PrepareWhisper() WhisperSetup {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return WhisperSetup{Error: "当前平台请手动选择 whisper-cli 和多语言 ggml 模型"}
	}
	root := filepath.Join(a.dataDir, "runtime", "whisper")
	if err := os.MkdirAll(root, 0755); err != nil { return WhisperSetup{Error: err.Error()} }
	client, err := netx.Client(a.currentProxy(), 0)
	if err != nil { return WhisperSetup{Error: err.Error()} }
	client.Timeout = 15 * time.Minute
	model := filepath.Join(root, "ggml-base.bin")
	if !fileExists(model) {
		if err := verifiedDownload(client, whisperModelURL, model, whisperModelSHA1, 200<<20, false); err != nil {
			return WhisperSetup{Error: "下载多语言模型失败：" + err.Error()}
		}
	}
	binDir := filepath.Join(root, "b5454-win-x64")
	binary := ""
	_ = filepath.Walk(binDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() && strings.EqualFold(info.Name(), "whisper-cli.exe") {
			binary = path
			return filepath.SkipAll
		}
		return nil
	})
	if binary == "" {
		archive := filepath.Join(root, "whisper-b5454-x64.zip")
		if !fileExists(archive) {
			if err := verifiedDownload(client, whisperArchiveURL, archive, whisperArchiveSHA256, 32<<20, true); err != nil {
				return WhisperSetup{Error: "下载识别程序失败：" + err.Error()}
			}
		}
		binary, err = unpackWhisper(archive, binDir)
		if err != nil { return WhisperSetup{Error: "安装识别程序失败：" + err.Error()} }
	}
	return WhisperSetup{OK: true, WhisperPath: binary, ModelPath: model}
}
