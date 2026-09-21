// Package main provides the tavernctl CLI toolchain for TavernAgent.
// It supports database audits, prompt dumping, archive import/export, and MCP server launching.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"tavernagent/internal/adapters/mcp"
	"tavernagent/internal/app"
	ctxpkg "tavernagent/internal/context"
	"tavernagent/internal/domain"
)

var (
	cliVersion = "tavernctl/v1.0"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "audit":
		err = runAudit(ctx, args)
	case "prompt":
		err = runPrompt(ctx, args)
	case "mcp":
		err = runMCP(ctx, args)
	case "pack":
		err = runPack(ctx, args)
	case "version", "--version", "-v":
		fmt.Printf("%s\n", cliVersion)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %q\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`tavernctl - TavernAgent 角色演义运行时命令行工具链

用法:
  tavernctl <command> [options]

子命令:
  audit     对数据目录与 SQLite 数据库执行健康与完整性审计
  prompt    编译并预览指定会话的完整 LLM 提示词上下文与 Token 估算
  mcp       启动 Model Context Protocol (MCP) 标准 JSON-RPC 2.0 服务 (stdio)
  pack      导入或导出 .tavernpack 剧情归档包
  version   显示工具链版本

选项:
  --data <path>  指定数据目录 (默认: data)
  -h, --help     显示帮助信息`)
}

func initApp(dataDir string) (*app.App, error) {
	if dataDir == "" {
		dataDir = "data"
	}
	abs, err := filepath.Abs(dataDir)
	if err == nil {
		dataDir = abs
	}
	return app.Bootstrap(app.Config{
		DataDir:      dataDir,
		FallbackKind: "mock",
	})
}

// -----------------------------------------------------------------------------
// 1. tavernctl audit
// -----------------------------------------------------------------------------

func runAudit(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	dataDir := fs.String("data", "data", "数据目录")
	_ = fs.Parse(args)

	a, err := initApp(*dataDir)
	if err != nil {
		return fmt.Errorf("加载应用实例失败: %w", err)
	}
	defer a.Close()

	fmt.Println("==================================================")
	fmt.Println("         TavernAgent 存储与状态健康审计")
	fmt.Println("==================================================")
	fmt.Printf("数据目录: %s\n", *dataDir)

	hasErrors := false

	// 1. SQLite 引擎与完整性检查
	issues, err := a.Store.IntegrityCheck()
	if err != nil {
		fmt.Printf("[FAIL] SQLite 完整性检查错误: %v\n", err)
		hasErrors = true
	} else if len(issues) > 0 {
		fmt.Printf("[FAIL] 发现 %d 处 SQLite 完整性问题:\n", len(issues))
		for _, issue := range issues {
			fmt.Printf("  - %s\n", issue)
		}
		hasErrors = true
	} else {
		fmt.Println("[PASS] SQLite PRAGMA integrity_check & foreign_key_check: 正常 (0 错误)")
	}

	// 2. 会话与分支扫描
	sessions, err := a.Store.ListSessions()
	if err != nil {
		return fmt.Errorf("读取会话列表失败: %w", err)
	}
	fmt.Printf("[INFO] 登记会话数: %d\n", len(sessions))

	brokenBranches := 0
	for _, s := range sessions {
		branches, err := a.Sessions.Branches(s.SessionID)
		if err != nil || len(branches) == 0 {
			fmt.Printf("  [WARN] 会话 %s (%s) 缺少活跃分支: %v\n", s.SessionID, s.Title, err)
			brokenBranches++
			continue
		}
		for _, b := range branches {
			node, err := a.Store.GetNode(b.HeadNodeID)
			if err != nil || node == nil {
				fmt.Printf("  [WARN] 分支 %s 挂载的游标节点不存在: %s\n", b.BranchID, b.HeadNodeID)
				brokenBranches++
			}
		}
	}
	if brokenBranches == 0 {
		fmt.Println("[PASS] 会话分支游标与节点引用完整")
	} else {
		hasErrors = true
	}

	// 3. 角色卡库扫描
	if a.Cards != nil {
		cards, err := a.Cards.List()
		if err != nil {
			fmt.Printf("[FAIL] 读取角色卡库失败: %v\n", err)
			hasErrors = true
		} else {
			fmt.Printf("[PASS] 角色卡库正常 (已安装 %d 张卡片)\n", len(cards))
		}
	}

	fmt.Println("--------------------------------------------------")
	if hasErrors {
		fmt.Println("审计结论: 存在异常项，请检查上方日志。")
		return fmt.Errorf("audit detected inconsistencies")
	}
	fmt.Println("审计结论: 全部存储与分支数据通过自检，状态完好。")
	return nil
}

// -----------------------------------------------------------------------------
// 2. tavernctl prompt
// -----------------------------------------------------------------------------

func runPrompt(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("prompt", flag.ExitOnError)
	dataDir := fs.String("data", "data", "数据目录")
	sessionID := fs.String("session", "", "目标会话 ID (必填)")
	inputText := fs.String("input", "（继续推演）", "本轮模拟输入的玩家文本")
	outputJSON := fs.Bool("json", false, "以原始 JSON 形式输出")
	_ = fs.Parse(args)

	if *sessionID == "" {
		return fmt.Errorf("请通过 --session <id> 指定会话 ID")
	}

	a, err := initApp(*dataDir)
	if err != nil {
		return fmt.Errorf("加载应用实例失败: %w", err)
	}
	defer a.Close()

	branches, err := a.Sessions.Branches(*sessionID)
	if err != nil || len(branches) == 0 {
		return fmt.Errorf("获取分支失败: %w", err)
	}
	branch := branches[0]

	state := domain.NewWorldState()
	if snap, err := a.Store.StateAt(branch.HeadNodeID); err == nil && snap != nil {
		if ws, err := domain.UnmarshalWorld(snap.StateJSON); err == nil && ws != nil {
			state = ws
		}
	}

	req, err := a.Compiler.Compile(ctx, *sessionID, branch.HeadNodeID, *inputText, ctxpkg.TurnDirectives{}, state, nil)
	if err != nil {
		return fmt.Errorf("编译上下文提示词失败: %w", err)
	}

	if *outputJSON {
		data, _ := json.MarshalIndent(req, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Println("==================================================")
	fmt.Println("           TavernAgent 提示词编译器结果")
	fmt.Println("==================================================")
	fmt.Printf("会话 ID:     %s\n", *sessionID)
	fmt.Printf("基准节点:   %s\n", branch.HeadNodeID)
	fmt.Printf("模拟输入:   %s\n", *inputText)
	fmt.Printf("消息总段数: %d\n", len(req.Messages))
	fmt.Printf("注入记忆数: %d\n", len(req.InjectedMemoryIDs))
	fmt.Println("--------------------------------------------------")

	for idx, msg := range req.Messages {
		fmt.Printf("\n[Message #%d] 角色: %s (字符数: %d)\n", idx+1, strings.ToUpper(msg.Role), len(msg.Content))
		lines := strings.Split(msg.Content, "\n")
		if len(lines) > 20 {
			for i := 0; i < 10; i++ {
				fmt.Printf("  %s\n", lines[i])
			}
			fmt.Printf("  ... [略过中间 %d 行] ...\n", len(lines)-20)
			for i := len(lines) - 10; i < len(lines); i++ {
				fmt.Printf("  %s\n", lines[i])
			}
		} else {
			for _, line := range lines {
				fmt.Printf("  %s\n", line)
			}
		}
	}

	return nil
}

// -----------------------------------------------------------------------------
// 3. tavernctl mcp
// -----------------------------------------------------------------------------

func runMCP(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	dataDir := fs.String("data", "data", "数据目录")
	_ = fs.Parse(args)

	a, err := initApp(*dataDir)
	if err != nil {
		return fmt.Errorf("初始化 MCP 服务失败: %w", err)
	}
	defer a.Close()

	srv := mcp.New(mcp.Config{
		Store:    a.Store,
		Sessions: a.Sessions,
		Turns:    a.Turns,
		Memories: a.Memories,
		Cards:    a.Cards,
		Compiler: a.Compiler,
	})

	return srv.ServeStdio(ctx, os.Stdin, os.Stdout)
}

// -----------------------------------------------------------------------------
// 4. tavernctl pack
// -----------------------------------------------------------------------------

func runPack(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("请指定 pack 子操作: export 或 import")
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "export":
		fs := flag.NewFlagSet("pack export", flag.ExitOnError)
		dataDir := fs.String("data", "data", "数据目录")
		sessionID := fs.String("session", "", "导出目标会话 ID (必填)")
		outFile := fs.String("out", "", "输出归档文件路径 (如: my_story.tavernpack)")
		_ = fs.Parse(subArgs)

		if *sessionID == "" || *outFile == "" {
			return fmt.Errorf("pack export 需要 --session 与 --out 参数")
		}

		a, err := initApp(*dataDir)
		if err != nil {
			return err
		}
		defer a.Close()

		res, err := a.Archive.Export(ctx, *sessionID, "")
		if err != nil {
			return fmt.Errorf("导出剧情包失败: %w", err)
		}

		if err := os.WriteFile(*outFile, res.Data, 0o644); err != nil {
			return fmt.Errorf("写入文件失败: %w", err)
		}
		fmt.Printf("✓ 成功导出剧情包到: %s (大小: %d 字节)\n", *outFile, len(res.Data))
		return nil

	case "import":
		fs := flag.NewFlagSet("pack import", flag.ExitOnError)
		dataDir := fs.String("data", "data", "数据目录")
		inFile := fs.String("in", "", "输入归档文件路径 (如: my_story.tavernpack)")
		_ = fs.Parse(subArgs)

		if *inFile == "" {
			return fmt.Errorf("pack import 需要 --in 参数")
		}

		a, err := initApp(*dataDir)
		if err != nil {
			return err
		}
		defer a.Close()

		zipData, err := os.ReadFile(*inFile)
		if err != nil {
			return fmt.Errorf("读取归档文件失败: %w", err)
		}

		res, err := a.Archive.Import(ctx, zipData)
		if err != nil {
			return fmt.Errorf("导入剧情包失败: %w", err)
		}

		fmt.Printf("✓ 成功导入会话: %s\n", res.Session.SessionID)
		fmt.Printf("  标题:   %s\n", res.Session.Title)
		if res.Manifest != nil {
			fmt.Printf("  版本:   %s\n", res.Manifest.AppVersion)
		}
		return nil

	default:
		return fmt.Errorf("未知 pack 操作: %s (支持 export 或 import)", sub)
	}
}
