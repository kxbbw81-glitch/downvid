package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveCoverRejectsNonImageAndPathTraversal(t *testing.T) {
	a := &App{}
	dir := t.TempDir()
	if got := a.SaveCover("data:text/html;base64,PGgxPmJvb208L2gxPg==", dir, "../evil"); got.OK {
		t.Fatal("HTML 不可保存为封面")
	}
	data := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("sample"))
	got := a.SaveCover(data, dir, "../evil")
	if !got.OK {
		t.Fatal(got.Error)
	}
	relative, err := filepath.Rel(dir, got.Path)
	if err != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		t.Fatalf("封面写出了下载目录：%q", got.Path)
	}
}

func TestSaveTextDoesNotConfuseRetranscriptionWithOldFile(t *testing.T) {
	a := &App{}
	dir := t.TempDir()
	first := a.SaveText("first", dir, "video-1", "transcript")
	second := a.SaveText("second", dir, "video-1", "transcript")
	if !first.OK || !second.OK || first.Path == second.Path {
		t.Fatalf("应保留两次不同的识别结果：%+v %+v", first, second)
	}
	content, err := os.ReadFile(second.Path)
	if err != nil || string(content) != "second" {
		t.Fatalf("新文件内容错误：%q, %v", content, err)
	}
}

func TestForkKeepsDataAndUpdatesSeparate(t *testing.T) {
	if distribution != "mediaharvest" {
		t.Skip("仅检查素材增强版分发")
	}
	if filepath.Base(resolveDataDir()) == "downvid" {
		t.Fatal("增强版不应修改官方用户数据")
	}
	a := NewApp()
	if result := a.DownloadUpdate("1.5.0", "github"); result.OK || !strings.Contains(result.Error, "官方更新") {
		t.Fatalf("不应下载覆盖增强功能的官方更新：%+v", result)
	}
	if result := a.InstallUpdate(); result.OK || !strings.Contains(result.Error, "官方更新") {
		t.Fatalf("不应安装官方更新：%+v", result)
	}
}
