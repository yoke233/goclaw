package skills

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/smallnest/goclaw/config"
)

// DownloadInstaller implements file download and extraction installations
type DownloadInstaller struct {
	// Client is the HTTP client to use for downloads
	Client *http.Client

	// ConfigDir is the base directory for downloaded tools
	ConfigDir string
}

// CanInstall checks if download installer can handle the spec
func (d *DownloadInstaller) CanInstall(spec *SkillInstallSpec) bool {
	return spec.Kind == "download" && spec.URL != ""
}

// Install downloads and optionally extracts a file
func (d *DownloadInstaller) Install(ctx context.Context, spec *SkillInstallSpec) InstallResult {
	// Validate URL
	downloadURL := strings.TrimSpace(spec.URL)
	if downloadURL == "" {
		return InstallResult{
			Success: false,
			Message: "missing download URL",
		}
	}

	// Extract filename from URL
	filename := d.extractFilename(downloadURL)
	if filename == "" {
		filename = "download"
	}

	// Resolve target directory
	targetDir := d.resolveTargetDir(spec)
	if err := EnsureDir(targetDir); err != nil {
		return InstallResult{
			Success: false,
			Message: fmt.Sprintf("failed to create target directory: %v", err),
		}
	}

	// Download file
	archivePath := filepath.Join(targetDir, filename)
	downloadedBytes, err := d.downloadFile(ctx, downloadURL, archivePath)
	if err != nil {
		return InstallResult{
			Success: false,
			Message: fmt.Sprintf("download failed: %v", err),
		}
	}

	// Check if extraction is needed
	shouldExtract := spec.Extract
	if !shouldExtract {
		// Auto-detect if archive type
		shouldExtract = d.resolveArchiveType(spec, filename) != ""
	}

	if !shouldExtract {
		return InstallResult{
			Success: true,
			Message: fmt.Sprintf("Downloaded to %s", archivePath),
			Stdout:  fmt.Sprintf("downloaded=%d bytes", downloadedBytes),
		}
	}

	// Extract archive
	archiveType := d.resolveArchiveType(spec, filename)
	if archiveType == "" {
		return InstallResult{
			Success: false,
			Message: "extract requested but archive type could not be detected",
		}
	}

	stdout, stderr, exitCode := d.extractArchive(ctx, spec, archivePath, targetDir, archiveType)
	success := exitCode != nil && *exitCode == 0

	var message string
	if success {
		message = fmt.Sprintf("Downloaded and extracted to %s", targetDir)
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

// extractFilename extracts a filename from a URL
func (d *DownloadInstaller) extractFilename(u string) string {
	parsed, err := url.Parse(u)
	if err == nil && parsed.Path != "" {
		return filepath.Base(parsed.Path)
	}

	// Fallback to last part of URL
	parts := strings.Split(u, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return ""
}

// resolveTargetDir resolves the target directory for the download
func (d *DownloadInstaller) resolveTargetDir(spec *SkillInstallSpec) string {
	// Use spec's targetDir if provided
	if spec.TargetDir != "" {
		return ResolveUserPath(spec.TargetDir)
	}

	// Use configured ConfigDir
	if d.ConfigDir != "" {
		return filepath.Join(d.ConfigDir, "tools")
	}

	// Default to ~/.goclaw/tools
	home, err := config.ResolveUserHomeDir()
	if err != nil {
		return "/tmp/goclaw-tools"
	}
	return filepath.Join(home, ".goclaw", "tools")
}

// resolveArchiveType determines the archive type from the filename
func (d *DownloadInstaller) resolveArchiveType(spec *SkillInstallSpec, filename string) string {
	// Use explicit archive type if specified
	if spec.Archive != "" {
		return strings.ToLower(strings.TrimSpace(spec.Archive))
	}

	// Auto-detect from filename extension
	lower := strings.ToLower(filename)

	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return "tar.gz"
	}
	if strings.HasSuffix(lower, ".tar.bz2") || strings.HasSuffix(lower, ".tbz2") {
		return "tar.bz2"
	}
	if strings.HasSuffix(lower, ".tar.xz") || strings.HasSuffix(lower, ".txz") {
		return "tar.xz"
	}
	if strings.HasSuffix(lower, ".zip") {
		return "zip"
	}

	return ""
}

// downloadFile downloads a file from a URL
func (d *DownloadInstaller) downloadFile(ctx context.Context, url, destPath string) (int64, error) {
	client := d.Client
	if client == nil {
		// Create client with timeout
		client = &http.Client{
			Timeout: 10 * time.Minute,
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download failed with status %d %s", resp.StatusCode, resp.Status)
	}

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return 0, fmt.Errorf("failed to create directory: %w", err)
	}

	// Create destination file
	file, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Download with progress tracking
	copied, err := io.Copy(file, resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to write file: %w", err)
	}

	return copied, nil
}

// extractArchive extracts an archive file
func (d *DownloadInstaller) extractArchive(
	ctx context.Context,
	spec *SkillInstallSpec,
	archivePath,
	targetDir,
	archiveType string,
) (stdout, stderr string, exitCode *int) {
	var argv []string

	switch archiveType {
	case "zip":
		if !HasBinary("unzip") {
			return "", "", nil
		}
		argv = []string{"unzip", "-qo", archivePath, "-d", targetDir}

	case "tar.gz", "tar.bz2", "tar.xz":
		if !HasBinary("tar") {
			return "", "", nil
		}
		argv = []string{"tar", "xf", archivePath, "-C", targetDir}

		// Add strip-components if specified
		if spec.StripComponents > 0 {
			argv = append(argv, "--strip-components", fmt.Sprintf("%d", spec.StripComponents))
		}
	}

	if argv == nil {
		return "", "", nil
	}

	s, e, ec, _ := RunCommandWithTimeout(ctx, argv, nil)
	return s, e, ec
}
