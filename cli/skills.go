package cli

import (
	"fmt"
	"os"

	"github.com/smallnest/dogclaw/goclaw/agent"
	"github.com/smallnest/dogclaw/goclaw/config"
	"github.com/smallnest/dogclaw/goclaw/internal/logger"
	"github.com/spf13/cobra"
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Skills management",
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all discovered skills",
	Run:   runSkillsList,
}

func init() {
	rootCmd.AddCommand(skillsCmd)
	skillsCmd.AddCommand(skillsListCmd)
}

func runSkillsList(cmd *cobra.Command, args []string) {
	// 加载配置（主要为了初始化日志等）
	_, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to load config: %v\n", err)
	}

	// 初始化日志
	if err := logger.Init("warn", false); err != nil { // 使用 warn 级别减少干扰
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// 创建工作区
	workspace := os.Getenv("HOME") + "/.goclaw/workspace"

	// 创建技能加载器
	skillsLoader := agent.NewSkillsLoader(workspace, []string{})
	if err := skillsLoader.Discover(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to discover skills: %v\n", err)
		os.Exit(1)
	}

	skills := skillsLoader.List()
	if len(skills) == 0 {
		fmt.Println("No skills found.")
		return
	}

	fmt.Printf("Found %d skills:\n\n", len(skills))
	for _, skill := range skills {
		fmt.Printf("📦 %s\n", skill.Name)
		if skill.Description != "" {
			fmt.Printf("   %s\n", skill.Description)
		}
		
		// 显示元数据信息
		emoji := skill.Metadata.OpenClaw.Emoji
		if emoji != "" {
			fmt.Printf("   Icon: %s\n", emoji)
		}
		
		requires := skill.Metadata.OpenClaw.Requires
		if len(requires.Bins) > 0 {
			fmt.Printf("   Requires: %v\n", requires.Bins)
		}
		
		fmt.Println()
	}
}
