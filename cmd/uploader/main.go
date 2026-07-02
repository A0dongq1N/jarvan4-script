// Package main 本地 .so 编译上传工具
// 用法: go run ./cmd/uploader/
// 环境变量: COS_SECRET_ID, COS_SECRET_KEY, COS_BUCKET, COS_REGION, MASTER_URL
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/A0dongq1N/jarvan4-platform/shared/cos"
)

func main() {
	// 1. 编译 .so
	fmt.Println("=== 1. 编译 .so ===")
	if err := buildSO(); err != nil {
		fmt.Fprintf(os.Stderr, "编译失败: %v\n", err)
		os.Exit(1)
	}

	// 2. 检查是否需要上传
	if os.Getenv("SKIP_UPLOAD") == "1" {
		fmt.Println("\n=== 跳过上传（SKIP_UPLOAD=1）===")
		fmt.Printf("本地 .so 路径: %s/dist/\n", mustGetwd())
		os.Exit(0)
	}

	// 3. 创建 COS client
	secretID := os.Getenv("COS_SECRET_ID")
	secretKey := os.Getenv("COS_SECRET_KEY")
	bucket := envOr("COS_BUCKET", "jarvan4-1257748620")
	region := envOr("COS_REGION", "ap-guangzhou")
	masterURL := envOr("MASTER_URL", "http://localhost:8090")

	if secretID == "" || secretKey == "" {
		fmt.Fprintln(os.Stderr, "Error: COS_SECRET_ID 和 COS_SECRET_KEY 环境变量未设置")
		fmt.Fprintln(os.Stderr, "首次配置: echo 'export COS_SECRET_ID=xxx' >> ~/.bashrc && source ~/.bashrc")
		os.Exit(1)
	}

	cosClient, err := cos.NewClient(cos.Config{
		SecretID:  secretID,
		SecretKey: secretKey,
		Bucket:    bucket,
		Region:    region,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "COS client 创建失败: %v\n", err)
		os.Exit(1)
	}

	// 4. 上传所有 .so
	fmt.Println("\n=== 2. 上传到 COS ===")
	commitHash := getCommitHash()
	commitMsg := getCommitMsg()
	author := getAuthor()

	soFiles, _ := filepath.Glob("dist/*.so")
	for _, soFile := range soFiles {
		name := strings.TrimSuffix(filepath.Base(soFile), ".so")

		// 版本化路径 + 固定 latest 路径
		keyVersioned := fmt.Sprintf("scripts/%s/%s.so", name, commitHash)
		keyLatest := fmt.Sprintf("scripts/%s/%s.so", name, name)

		fmt.Printf("  ⬆️  %s → cos://%s/%s\n", soFile, bucket, keyVersioned)
		if err := cosClient.UploadFile(context.Background(), keyVersioned, soFile); err != nil {
			fmt.Fprintf(os.Stderr, "上传 %s 失败: %v\n", keyVersioned, err)
			os.Exit(1)
		}

		fmt.Printf("  ⬆️  %s → cos://%s/%s (latest)\n", soFile, bucket, keyLatest)
		if err := cosClient.UploadFile(context.Background(), keyLatest, soFile); err != nil {
			fmt.Fprintf(os.Stderr, "上传 %s 失败: %v\n", keyLatest, err)
			os.Exit(1)
		}

		// 5. 通知 Master
		fmt.Printf("  📡 通知 Master: name=%s\n", name)
		notifyMaster(masterURL, name, commitHash, keyLatest, commitMsg, author)
	}

	fmt.Println("\n✓ 全部完成")
}

func buildSO() error {
	entries, err := os.ReadDir("scripts")
	if err != nil {
		return fmt.Errorf("read scripts dir: %w", err)
	}

	os.MkdirAll("dist", 0755)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "_") {
			continue
		}
		mainGo := filepath.Join("scripts", name, "main.go")
		if _, err := os.Stat(mainGo); err != nil {
			continue
		}

		fmt.Printf("  ==> building %s\n", name)
		cmd := exec.Command("go", "build", "-tags", "plugin", "-buildmode=plugin",
			"-o", filepath.Join("dist", name+".so"),
			"./"+filepath.Join("scripts", name))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s: %w", name, err)
		}
	}
	fmt.Println("✓ 编译完成: dist/")
	return nil
}

func notifyMaster(masterURL, name, commitHash, artifactURL, commitMsg, author string) {
	body := fmt.Sprintf(`{"name":"%s","commitHash":"%s","artifactUrl":"%s","commitMsg":"%s","author":"%s","sourceRepo":"https://github.com/A0dongq1N/jarvan4-script","sourcePath":"scripts/%s/main.go"}`,
		name, commitHash, artifactURL, commitMsg, author, name)

	resp, err := http.Post(masterURL+"/api/internal/scripts/publish", "application/json", strings.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ 通知 Master 失败: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		fmt.Printf("  ✓ Master 已更新脚本 %s\n", name)
	} else {
		fmt.Fprintf(os.Stderr, "  ✗ Master 返回 HTTP %d\n", resp.StatusCode)
	}
}

func getCommitHash() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "local"
	}
	return strings.TrimSpace(string(out))
}

func getCommitMsg() string {
	out, err := exec.Command("git", "log", "-1", "--pretty=format:%s").Output()
	if err != nil {
		return "local build"
	}
	return strings.TrimSpace(string(out))
}

func getAuthor() string {
	out, err := exec.Command("git", "log", "-1", "--pretty=format:%an").Output()
	if err != nil {
		return "local"
	}
	return strings.TrimSpace(string(out))
}

func mustGetwd() string {
	wd, _ := os.Getwd()
	return wd
}

func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
