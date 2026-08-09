// Package main 本地 .so 编译上传工具
// 用法: go run ./cmd/uploader/
// COS 密钥优先从环境变量读，其次从 /root/.cos.conf（coscmd 配置）读
package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/A0dongq1N/jarvan4-platform/shared/cos"
	"github.com/A0dongq1N/jarvan4-platform/spec"
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

	// 3. 获取 COS 配置（优先环境变量，其次 coscmd 配置文件）
	secretID := os.Getenv("COS_SECRET_ID")
	secretKey := os.Getenv("COS_SECRET_KEY")
	bucket := envOr("COS_BUCKET", "")
	region := envOr("COS_REGION", "")
	masterURL := envOr("MASTER_URL", "http://localhost:8090")

	// 环境变量没设全，从 coscmd 配置读
	if secretID == "" || secretKey == "" || bucket == "" || region == "" {
		cosConf := readCoscmdConfig("/root/.cos.conf")
		if secretID == "" {
			secretID = cosConf["secret_id"]
		}
		if secretKey == "" {
			secretKey = cosConf["secret_key"]
		}
		if bucket == "" {
			bucket = cosConf["bucket"]
		}
		if region == "" {
			region = cosConf["region"]
		}
	}

	if secretID == "" || secretKey == "" {
		fmt.Fprintln(os.Stderr, "Error: COS 密钥未找到")
		fmt.Fprintln(os.Stderr, "设置方式: export COS_SECRET_ID=xxx && export COS_SECRET_KEY=xxx")
		fmt.Fprintln(os.Stderr, "或用 coscmd 配置: coscmd config -a xxx -s xxx -b xxx -r xxx")
		os.Exit(1)
	}
	if bucket == "" {
		bucket = "jarvan4-1257748620"
	}
	if region == "" {
		region = "ap-guangzhou"
	}

	fmt.Printf("COS: bucket=%s region=%s secretID=%s...\n", bucket, region, secretID[:8])

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
	fmt.Printf("Plugin ABI: v%d（须与 Worker spec.PluginABIVersion 一致）\n", spec.PluginABIVersion)
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

// readCoscmdConfig 从 coscmd 配置文件读 COS 配置
// 格式是 INI: [common]\nsecret_id = xxx\nsecret_key = xxx
func readCoscmdConfig(path string) map[string]string {
	result := make(map[string]string)
	f, err := os.Open(path)
	if err != nil {
		return result
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") || line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		result[key] = val
	}
	return result
}
