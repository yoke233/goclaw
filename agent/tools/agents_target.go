package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/smallnest/goclaw/config"
)

type agentsTarget struct {
	Scope   string
	Role    string
	RepoDir string
	RootDir string
}

func resolveAgentsTarget(workspaceDir, skillsRoleDir string, params map[string]interface{}, defaultScope string) (agentsTarget, error) {
	workspaceDir = strings.TrimSpace(workspaceDir)
	if workspaceDir == "" {
		return agentsTarget{}, fmt.Errorf("workspaceDir is required")
	}

	scope := strings.ToLower(strings.TrimSpace(asString(params["scope"])))
	if scope == "" {
		scope = strings.ToLower(strings.TrimSpace(defaultScope))
	}

	switch scope {
	case "workspace":
		return agentsTarget{
			Scope:   scope,
			RootDir: workspaceDir,
		}, nil
	case "role":
		role := strings.TrimSpace(asString(params["role"]))
		if role == "" {
			role = "main"
		}
		if !isSafeIdent(role) {
			return agentsTarget{}, fmt.Errorf("invalid role: %s", role)
		}

		base := strings.TrimSpace(skillsRoleDir)
		if base == "" {
			base = "skills"
		}

		root := filepath.Join(workspaceDir, base, role)
		return agentsTarget{
			Scope:   scope,
			Role:    role,
			RootDir: root,
		}, nil
	case "repo":
		repoDir := strings.TrimSpace(asString(params["repo_dir"]))
		if repoDir == "" {
			return agentsTarget{}, fmt.Errorf("repo_dir is required when scope=repo")
		}
		repoDir = config.ExpandUserPath(repoDir)
		if repoDir == "" {
			return agentsTarget{}, fmt.Errorf("repo_dir is empty")
		}
		if !filepath.IsAbs(repoDir) {
			repoDir = filepath.Join(workspaceDir, repoDir)
		}
		repoAbs, err := filepath.Abs(repoDir)
		if err != nil {
			return agentsTarget{}, fmt.Errorf("resolve repo_dir: %w", err)
		}
		repoAbs = filepath.Clean(repoAbs)

		workspaceAbs, err := filepath.Abs(workspaceDir)
		if err != nil {
			return agentsTarget{}, fmt.Errorf("resolve workspaceDir: %w", err)
		}
		workspaceAbs = filepath.Clean(workspaceAbs)

		if !isWithinDir(workspaceAbs, repoAbs) {
			return agentsTarget{}, fmt.Errorf("repo_dir must be within workspace: %s", workspaceAbs)
		}

		info, err := os.Stat(repoAbs)
		if err != nil {
			return agentsTarget{}, fmt.Errorf("repo_dir not found: %w", err)
		}
		if !info.IsDir() {
			return agentsTarget{}, fmt.Errorf("repo_dir is not a directory: %s", repoAbs)
		}

		return agentsTarget{
			Scope:   scope,
			RepoDir: repoAbs,
			RootDir: repoAbs,
		}, nil
	default:
		return agentsTarget{}, fmt.Errorf("scope must be one of: workspace|role|repo")
	}
}

func isWithinDir(baseDir, targetDir string) bool {
	baseDir = filepath.Clean(strings.TrimSpace(baseDir))
	targetDir = filepath.Clean(strings.TrimSpace(targetDir))
	if baseDir == "" || targetDir == "" {
		return false
	}
	rel, err := filepath.Rel(baseDir, targetDir)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, "../"))
}
