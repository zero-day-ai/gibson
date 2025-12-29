package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// GitOperations defines the interface for git operations
type GitOperations interface {
	// Clone clones a repository to the specified destination
	Clone(url, dest string, opts CloneOptions) error

	// Pull performs a git pull in the specified directory
	Pull(dir string) error

	// GetVersion returns the current commit hash of the repository
	GetVersion(dir string) (string, error)

	// ParseRepoURL extracts component information from a repository URL
	ParseRepoURL(url string) (*RepoInfo, error)
}

// CloneOptions contains options for cloning a repository
type CloneOptions struct {
	// Depth specifies the depth for shallow clones (0 for full clone)
	Depth int

	// Branch specifies the branch to clone
	Branch string

	// Tag specifies the tag to clone
	Tag string
}

// RepoInfo contains parsed information from a repository URL
type RepoInfo struct {
	// Host is the git hosting service (e.g., github.com)
	Host string

	// Owner is the repository owner or organization
	Owner string

	// Repo is the full repository name (e.g., gibson-agent-scanner)
	Repo string

	// Kind is the component type extracted from repo name (agent, tool, plugin)
	Kind string

	// Name is the component name extracted from repo name (scanner, nmap, vuln-db)
	Name string
}

// DefaultGitOperations implements GitOperations using os/exec
type DefaultGitOperations struct{}

// NewDefaultGitOperations creates a new DefaultGitOperations instance
func NewDefaultGitOperations() GitOperations {
	return &DefaultGitOperations{}
}

// Clone clones a repository to the specified destination
func (g *DefaultGitOperations) Clone(url, dest string, opts CloneOptions) error {
	args := []string{"clone"}

	// Add depth option for shallow clone
	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}

	// Add branch option
	if opts.Branch != "" {
		args = append(args, "--branch", opts.Branch)
	}

	// Add tag option (tags can be checked out like branches)
	if opts.Tag != "" {
		args = append(args, "--branch", opts.Tag)
	}

	args = append(args, url, dest)

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// Pull performs a git pull in the specified directory
func (g *DefaultGitOperations) Pull(dir string) error {
	cmd := exec.Command("git", "pull")
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// GetVersion returns the current commit hash of the repository
func (g *DefaultGitOperations) GetVersion(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w (output: %s)", err, string(output))
	}

	// Trim whitespace from output
	hash := strings.TrimSpace(string(output))
	if hash == "" {
		return "", fmt.Errorf("git rev-parse returned empty hash")
	}

	return hash, nil
}

// ParseRepoURL extracts component information from a repository URL
func (g *DefaultGitOperations) ParseRepoURL(url string) (*RepoInfo, error) {
	if url == "" {
		return nil, fmt.Errorf("repository URL cannot be empty")
	}

	// Patterns to match:
	// https://github.com/org/gibson-agent-scanner.git
	// git@github.com:org/gibson-tool-nmap.git
	// https://github.com/org/gibson-plugin-vuln-db

	var host, owner, repo string

	// Try HTTPS pattern first
	httpsPattern := regexp.MustCompile(`^https?://([^/]+)/([^/]+)/([^/]+?)(?:\.git)?$`)
	if matches := httpsPattern.FindStringSubmatch(url); matches != nil {
		host = matches[1]
		owner = matches[2]
		repo = matches[3]
	} else {
		// Try SSH pattern
		sshPattern := regexp.MustCompile(`^git@([^:]+):([^/]+)/([^/]+?)(?:\.git)?$`)
		if matches := sshPattern.FindStringSubmatch(url); matches != nil {
			host = matches[1]
			owner = matches[2]
			repo = matches[3]
		} else {
			return nil, fmt.Errorf("unable to parse repository URL: %s", url)
		}
	}

	// Extract component kind and name from repo name
	// Expected format: gibson-{kind}-{name} (e.g., gibson-agent-scanner)
	kind, name, err := parseRepoName(repo)
	if err != nil {
		return nil, fmt.Errorf("failed to parse repository name: %w", err)
	}

	return &RepoInfo{
		Host:  host,
		Owner: owner,
		Repo:  repo,
		Kind:  kind,
		Name:  name,
	}, nil
}

// parseRepoName extracts component kind and name from repository name
func parseRepoName(repoName string) (kind, name string, err error) {
	// Expected format: gibson-{kind}-{name}
	// Examples:
	//   gibson-agent-scanner -> kind: agent, name: scanner
	//   gibson-tool-nmap -> kind: tool, name: nmap
	//   gibson-plugin-vuln-db -> kind: plugin, name: vuln-db

	if !strings.HasPrefix(repoName, "gibson-") {
		return "", "", fmt.Errorf("repository name must start with 'gibson-': %s", repoName)
	}

	// Remove "gibson-" prefix
	remainder := strings.TrimPrefix(repoName, "gibson-")

	// Split on first hyphen to get kind and name
	parts := strings.SplitN(remainder, "-", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("repository name must be in format 'gibson-{kind}-{name}': %s", repoName)
	}

	kind = parts[0]
	name = parts[1]

	// Validate kind is one of the expected types
	validKinds := map[string]bool{
		"agent":  true,
		"tool":   true,
		"plugin": true,
	}

	if !validKinds[kind] {
		return "", "", fmt.Errorf("invalid component kind '%s', must be one of: agent, tool, plugin", kind)
	}

	if name == "" {
		return "", "", fmt.Errorf("component name cannot be empty")
	}

	return kind, name, nil
}

// String returns a string representation of RepoInfo
func (r *RepoInfo) String() string {
	return fmt.Sprintf("%s/%s/%s (kind=%s, name=%s)", r.Host, r.Owner, r.Repo, r.Kind, r.Name)
}

// ToURL converts RepoInfo to an HTTPS URL
func (r *RepoInfo) ToURL() string {
	return fmt.Sprintf("https://%s/%s/%s.git", r.Host, r.Owner, r.Repo)
}

// ToSSHURL converts RepoInfo to an SSH URL
func (r *RepoInfo) ToSSHURL() string {
	return fmt.Sprintf("git@%s:%s/%s.git", r.Host, r.Owner, r.Repo)
}

// ComponentPath returns the expected filesystem path for this component
// Typically: {kind}s/{name} (e.g., agents/scanner, tools/nmap)
func (r *RepoInfo) ComponentPath() string {
	return filepath.Join(r.Kind+"s", r.Name)
}
