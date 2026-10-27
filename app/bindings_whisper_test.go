package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifiedDownloadRejectsCorruptionAndSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("verified")) }))
	defer server.Close()
	sum := sha256.Sum256([]byte("verified"))
	path := filepath.Join(t.TempDir(), "asset.bin")
	for _, test := range []struct {
		digest string
		limit  int64
	}{{"invalid", 100}, {hex.EncodeToString(sum[:]), 2}} {
		if err := verifiedDownload(context.Background(), server.Client(), server.URL, path, test.digest, test.limit, true); err == nil {
			t.Fatal("损坏或超限内容不应安装")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("失败后不应留下可执行资产")
		}
		if _, err := os.Stat(path + ".partial"); !os.IsNotExist(err) {
			t.Fatal("失败后应清理临时下载")
		}
	}
	if err := verifiedDownload(context.Background(), server.Client(), server.URL, path, hex.EncodeToString(sum[:]), 100, true); err != nil {
		t.Fatal(err)
	}
}

func TestVerifiedDownloadCanCancelFirstInstall(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	path := filepath.Join(t.TempDir(), "model.bin")
	go func() { done <- verifiedDownload(ctx, server.Client(), server.URL, path, "unused", 100, true) }()
	<-started
	cancel()
	if err := <-done; err == nil {
		t.Fatal("取消首次安装应终止请求")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("取消后不应留下模型文件")
	}
}

func TestUnpackWhisperRejectsTraversalAndCanRetry(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "engine.zip")
	writeZip := func(name string) {
		f, err := os.Create(archive)
		if err != nil {
			t.Fatal(err)
		}
		z := zip.NewWriter(f)
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("fixture"))
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}
	writeZip("../escaped.exe")
	target := filepath.Join(root, "engine")
	if _, err := unpackWhisper(archive, target); err == nil {
		t.Fatal("不应接受包外路径")
	}
	if _, err := os.Stat(filepath.Join(root, "escaped.exe")); !os.IsNotExist(err) {
		t.Fatal("写出了安装目录")
	}
	writeZip("bin/whisper-cli.exe")
	for range 2 {
		path, err := unpackWhisper(archive, target)
		if err != nil || !fileExists(path) {
			t.Fatalf("失败后应能重试：%q %v", path, err)
		}
	}
}
