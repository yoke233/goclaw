package skills

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/smallnest/goclaw/config"
)

// GoInstaller implements Go module installations
type GoInstaller struct {
	// GOBIN is the Go bin directory to use
	GOBIN string
}

// CanInstall checks if go is available
func (g *GoInstaller) CanInstall(spec *SkillInstallSpec) bool {
	if spec.Kind != "go" {
		return false
	}
	return HasBinary("go") && spec.Module != ""
}

// Install installs a Go module
func (g *GoInstaller) Install(ctx context.Context, spec *SkillInstallSpec) InstallResult {
	if !HasBinary("go") {
		return InstallResult{
			Success: false,
			Message: "go not available (install via: brew install go)",
		}
	}

	// Validate module
	if spec.Module == "" {
		return InstallResult{
			Success: false,
			Message: "missing go module",
		}
	}

	// Prepare environment
	env := make(map[string]string)

	// Set GOBIN if provided or try to detect brew GOBIN
	if g.GOBIN != "" {
		env["GOBIN"] = g.GOBIN
	} else {
		// Try to detect brew's bin directory
		brew := &BrewInstaller{}
		if brewBin, _ := brew.ResolveBrewBinDir(ctx); brewBin != "" {
			env["GOBIN"] = brewBin
		}
	}

	// Check if already installed
	if !spec.Extract {
		binName := g.extractBinaryName(spec.Module)
		if binName != "" && (HasBinary(binName) || g.isInstalledInGOBIN(binName)) {
			return InstallResult{
				Success: true,
				Message: fmt.Sprintf("Already installed: %s", spec.Module),
			}
		}
	}

	// Build command
	argv := []string{"go", "install", spec.Module}

	// Run installation
	stdout, stderr, exitCode, _ := RunCommandWithTimeout(ctx, argv, env)

	success := exitCode != nil && *exitCode == 0
	var message string
	if success {
		message = fmt.Sprintf("Installed: %s", spec.Module)
	} else {
		message = FormatInstallFailureMessage(stdout, stderr, exitCode)
	}

	return InstallResult{
		Success:  success,
		Message:  message,
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
	}
}

// extractBinaryName extracts the binary name from a Go module path
// E.g., "github.com/user/tool" -> "tool"
func (g *GoInstaller) extractBinaryName(module string) string {
	module = strings.TrimSpace(module)

	// If module has version suffix like "@latest", remove it
	if idx := strings.Index(module, "@"); idx > 0 {
		module = module[:idx]
	}

	// Extract last component
	parts := strings.Split(module, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// isInstalledInGOBIN checks if a binary exists in the GOBIN directory
func (g *GoInstaller) isInstalledInGOBIN(binName string) bool {
	// Check environment variable
	gobin := os.Getenv("GOBIN")
	if gobin == "" {
		// Default to Go's default location
		home, err := config.ResolveUserHomeDir()
		if err != nil {
			return false
		}
		gobin = filepath.Join(home, "go", "bin")
	}

	// Check if binary exists in GOBIN
	path := filepath.Join(gobin, binName)
	if _, err := os.Stat(path); err == nil {
		return true
	}

	// Check if it's executable
	if _, err := exec.LookPath(binName); err == nil {
		// Verify it's the same as GOBIN version
		if path, err := exec.LookPath(binName); err == nil {
			return strings.HasPrefix(filepath.Clean(path), filepath.Clean(gobin))
		}
	}

	return false
}
