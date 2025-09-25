package analysis

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
	"golang.org/x/time/rate"
)

// ResolutionService resolves Git URLs for packages and caches results
type ResolutionService struct {
	cache       *cache.ASTCache
	httpClient  *http.Client
	rateLimiter *rate.Limiter
	cacheTTL    time.Duration
}

var (
	resolutionServiceInstance *ResolutionService
	resolutionServiceOnce     sync.Once
	resolutionServiceMutex    sync.RWMutex
	resolutionServiceTTL      time.Duration = 24 * time.Hour
)

// NewResolutionService creates a new resolution service with default 24-hour cache TTL
func NewResolutionService() *ResolutionService {
	return NewResolutionServiceWithTTL(24 * time.Hour)
}

// NewResolutionServiceWithTTL creates a new resolution service with configurable cache TTL
func NewResolutionServiceWithTTL(cacheTTL time.Duration) *ResolutionService {
	return &ResolutionService{
		cache: cache.MustGetASTCache(),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		rateLimiter: rate.NewLimiter(rate.Every(time.Second), 10), // 10 requests per second max
		cacheTTL:    cacheTTL,
	}
}

// GetResolutionService returns the global resolution service singleton
func GetResolutionService() (*ResolutionService, error) {
	var err error
	resolutionServiceOnce.Do(func() {
		resolutionServiceInstance = NewResolutionServiceWithTTL(resolutionServiceTTL)
	})
	return resolutionServiceInstance, err
}

// SetResolutionServiceTTL configures the resolution service TTL (must be called before first use)
func SetResolutionServiceTTL(ttl time.Duration) {
	resolutionServiceMutex.Lock()
	defer resolutionServiceMutex.Unlock()
	resolutionServiceTTL = ttl
}

// ResetResolutionService resets the singleton (for testing)
func ResetResolutionService() {
	resolutionServiceMutex.Lock()
	defer resolutionServiceMutex.Unlock()
	resolutionServiceInstance = nil
	resolutionServiceOnce = sync.Once{}
}

// ResolveGitURL attempts to resolve a Git URL for the given package
// Returns the resolved URL or empty string if none found
func (r *ResolutionService) ResolveGitURL(ctx *ScanContext, packageName, packageType string) (string, error) {
	// Check cache first (if cache is available and TTL > 0)
	if r.cache != nil && r.cacheTTL > 0 {
		if cached, err := r.getCachedAlias(packageName, packageType); err == nil && cached != nil && !cached.IsExpiredWithTTL(r.cacheTTL) {
			return cached.GitURL, nil
		}
	}

	// Try to resolve Git URL using heuristics
	gitURL, err := r.extractGitURL(ctx, packageName, packageType)
	if err != nil {
		return "", fmt.Errorf("failed to extract Git URL: %w", err)
	}

	// Validate the URL if one was found
	if gitURL != "" {
		if valid, finalURL, err := r.ValidateGitURL(ctx, gitURL); err != nil || !valid {
			gitURL = "" // Clear invalid URLs
		} else {
			// Use the redirected URL if validation succeeded
			gitURL = finalURL
		}
	}

	// Cache the result (including empty results to avoid repeated attempts)
	if r.cache != nil {
		if err := r.cacheAlias(packageName, packageType, gitURL); err != nil {
			// Log warning but don't fail the resolution
			if ctx != nil {
				return "", fmt.Errorf("failed to cache alias for %s/%s: %w", packageType, packageName, err)
			}
		}
	}

	return gitURL, nil
}

// extractGitURL uses heuristics to extract Git URLs based on package type
func (r *ResolutionService) extractGitURL(ctx *ScanContext, packageName, packageType string) (string, error) {
	switch strings.ToLower(packageType) {
	case "go":
		return r.extractGoGitURL(ctx, packageName)
	case "npm":
		return r.extractNpmGitURL(ctx, packageName)
	case "pip", "python":
		return r.extractPythonGitURL(ctx, packageName)
	case "docker":
		return r.extractDockerGitURL(ctx, packageName)
	case "helm":
		return r.extractHelmGitURL(ctx, packageName)
	default:
		return "", nil // Unknown package type
	}
}

// extractGoGitURL extracts Git URLs for Go modules
func (r *ResolutionService) extractGoGitURL(ctx *ScanContext, packageName string) (string, error) {
	// Direct GitHub/GitLab patterns
	if strings.HasPrefix(packageName, "github.com/") {
		return "https://" + packageName, nil
	}
	if strings.HasPrefix(packageName, "gitlab.com/") {
		return "https://" + packageName, nil
	}
	if strings.HasPrefix(packageName, "bitbucket.org/") {
		return "https://" + packageName, nil
	}

	// golang.org/x/* -> GitHub
	if strings.HasPrefix(packageName, "golang.org/x/") {
		repo := strings.TrimPrefix(packageName, "golang.org/x/")
		return "https://github.com/golang/" + repo, nil
	}

	// gopkg.in redirects - extract GitHub URL
	if strings.HasPrefix(packageName, "gopkg.in/") {
		return r.extractGopkgGitURL(ctx, packageName)
	}

	return "", nil // Cannot determine Git URL for this package
}

// extractGopkgGitURL handles gopkg.in redirects
func (r *ResolutionService) extractGopkgGitURL(ctx *ScanContext, packageName string) (string, error) {
	// gopkg.in/yaml.v3 -> github.com/go-yaml/yaml
	// gopkg.in/user/repo.v1 -> github.com/user/repo

	trimmed := strings.TrimPrefix(packageName, "gopkg.in/")

	// Handle version suffixes
	versionPattern := regexp.MustCompile(`\.v\d+$`)
	trimmed = versionPattern.ReplaceAllString(trimmed, "")

	if strings.Contains(trimmed, "/") {
		// gopkg.in/user/repo format
		return "https://github.com/" + trimmed, nil
	}

	// Common gopkg.in mappings
	mappings := map[string]string{
		"yaml": "go-yaml/yaml",
	}

	if mapped, exists := mappings[trimmed]; exists {
		return "https://github.com/" + mapped, nil
	}

	return "", nil
}

// extractNpmGitURL extracts Git URLs for NPM packages (placeholder)
func (r *ResolutionService) extractNpmGitURL(ctx *ScanContext, packageName string) (string, error) {
	// This would typically involve calling the NPM registry API
	// For now, return empty as we focus on Go packages first
	return "", nil
}

// extractPythonGitURL extracts Git URLs for Python packages (placeholder)
func (r *ResolutionService) extractPythonGitURL(ctx *ScanContext, packageName string) (string, error) {
	// This would typically involve calling PyPI API
	// For now, return empty as we focus on Go packages first
	return "", nil
}

// extractDockerGitURL extracts Git URLs for Docker images
func (r *ResolutionService) extractDockerGitURL(ctx *ScanContext, packageName string) (string, error) {
	// Strip common registry prefixes
	packageName = strings.TrimPrefix(packageName, "docker.io/")
	packageName = strings.TrimPrefix(packageName, "index.docker.io/")
	packageName = strings.TrimPrefix(packageName, "registry.hub.docker.com/")

	// Handle organization/image patterns for well-known organizations
	if strings.Contains(packageName, "/") {
		parts := strings.Split(packageName, "/")
		if len(parts) >= 2 {
			org := parts[0]
			image := parts[1]

			// Well-known organizations that host their Docker images on GitHub
			knownOrgs := map[string]string{
				"flanksource": "https://github.com/flanksource/%s",
				"bitnami":     "https://github.com/bitnami/containers", // Special case - bitnami uses containers repo
				"nginx":       "https://github.com/nginxinc/docker-%s",
				"postgres":    "https://github.com/docker-library/%s",
				"redis":       "https://github.com/docker-library/%s",
				"mysql":       "https://github.com/docker-library/%s",
			}

			if template, exists := knownOrgs[org]; exists {
				if org == "bitnami" {
					// Bitnami uses a single containers repository
					return "https://github.com/bitnami/containers", nil
				}
				return fmt.Sprintf(template, image), nil
			}

			// For other organizations, try the standard GitHub pattern
			// This will be validated by HTTP check
			return fmt.Sprintf("https://github.com/%s/%s", org, image), nil
		}
	}

	// Handle official Docker images (no organization prefix)
	// These typically don't have direct GitHub repositories
	officialImages := map[string]string{
		"nginx":    "https://github.com/nginxinc/docker-nginx",
		"postgres": "https://github.com/docker-library/postgres",
		"redis":    "https://github.com/docker-library/redis",
		"mysql":    "https://github.com/docker-library/mysql",
		"node":     "https://github.com/nodejs/docker-node",
		"python":   "https://github.com/docker-library/python",
		"golang":   "https://github.com/docker-library/golang",
		"alpine":   "https://github.com/alpinelinux/docker-alpine",
		"ubuntu":   "https://github.com/tianon/docker-brew-ubuntu-core",
		"debian":   "https://github.com/debuerreotype/docker-debian-artifacts",
	}

	if gitURL, exists := officialImages[packageName]; exists {
		return gitURL, nil
	}

	// For unknown images, we cannot reliably determine the Git URL
	return "", nil
}

// extractHelmGitURL extracts Git URLs for Helm charts
func (r *ResolutionService) extractHelmGitURL(ctx *ScanContext, packageName string) (string, error) {
	// Handle common Helm chart repositories

	// Flanksource charts - try individual repositories first, then fall back to monorepo
	if strings.Contains(packageName, "flanksource") ||
		packageName == "config-db" || packageName == "canary-checker" ||
		packageName == "flanksource-ui" || packageName == "apm-hub" {

		// First try individual chart repository
		individualRepo := fmt.Sprintf("https://github.com/flanksource/%s", packageName)
		if valid, finalURL, err := r.ValidateGitURL(ctx, individualRepo); err == nil && valid {
			return finalURL, nil // Return the redirected URL if any
		}

		// Fall back to the charts monorepo
		return "https://github.com/flanksource/charts", nil
	}

	// Ory charts (like kratos)
	if packageName == "kratos" || strings.Contains(packageName, "ory") {
		return "https://github.com/ory/k8s", nil
	}

	// Bitnami charts
	if strings.Contains(packageName, "bitnami") {
		return "https://github.com/bitnami/charts", nil
	}

	// Prometheus community charts
	if strings.Contains(packageName, "prometheus") || strings.Contains(packageName, "grafana") {
		return "https://github.com/prometheus-community/helm-charts", nil
	}

	// Ingress-nginx
	if strings.Contains(packageName, "ingress-nginx") {
		return "https://github.com/kubernetes/ingress-nginx", nil
	}

	// Cert-manager
	if strings.Contains(packageName, "cert-manager") {
		return "https://github.com/cert-manager/cert-manager", nil
	}

	// For unknown charts, we cannot reliably determine the Git URL
	// without accessing the chart repository index
	return "", nil
}

func (r *ResolutionService) ValidateGitURL(ctx *ScanContext, gitURL string) (bool, string, error) {
	valid, finalURL, err := r.validateGitURL(gitURL)
	if ctx != nil {
		if gitURL != finalURL {
			ctx.Debugf("%s -> %s (valid: %t, err: %v)", gitURL, finalURL, valid, err)
		} else {
			ctx.Debugf("%s (valid: %t, err: %v)", gitURL, valid, err)
		}
	}
	return valid, finalURL, err
}

// ValidateGitURL checks if a Git URL is accessible and returns the final URL after redirects
func (r *ResolutionService) validateGitURL(gitURL string) (bool, string, error) {
	// Rate limit validation requests
	ctx := context.Background()
	if err := r.rateLimiter.Wait(ctx); err != nil {
		return false, gitURL, err
	}

	// Normalize URL for validation
	validationURL := r.normalizeGitURL(gitURL)
	finalURL := validationURL

	// Create a custom HTTP client that tracks redirects
	client := &http.Client{
		Timeout: r.httpClient.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Limit to 10 redirects to prevent infinite loops
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// Track the final URL
			finalURL = req.URL.String()
			return nil
		},
	}

	req, err := http.NewRequest("HEAD", validationURL, nil)
	if err != nil {
		return false, gitURL, err
	}

	resp, err := client.Do(req)
	if err != nil {
		// Check if it's a redirect error (too many redirects)
		if strings.Contains(err.Error(), "redirect") {
			return false, gitURL, nil
		}
		return false, gitURL, nil // Network error = invalid
	}
	defer func() { _ = resp.Body.Close() }()

	// If we got a successful response, use the final URL
	if resp.StatusCode < 400 {
		// For successful responses, convert back to Git format if needed
		if finalURL != validationURL {
			// Remove any trailing slashes and ensure it's a proper Git URL
			finalURL = strings.TrimSuffix(finalURL, "/")
			// If the original had .git suffix and the redirect doesn't, preserve it
			if strings.HasSuffix(gitURL, ".git") && !strings.HasSuffix(finalURL, ".git") {
				finalURL = finalURL + ".git"
			}
		} else {
			// No redirect occurred, return the original Git URL
			finalURL = gitURL
		}
		return true, finalURL, nil
	}

	return false, gitURL, nil
}

// normalizeGitURL converts Git URLs to HTTP URLs suitable for validation
func (r *ResolutionService) normalizeGitURL(gitURL string) string {
	// Remove .git suffix for HTTP validation
	url := strings.TrimSuffix(gitURL, ".git")

	// Ensure https prefix
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}

	return url
}

// getCachedAlias retrieves a cached dependency alias
func (r *ResolutionService) getCachedAlias(packageName, packageType string) (*models.DependencyAlias, error) {
	return r.cache.GetDependencyAlias(packageName, packageType)
}

// cacheAlias stores a dependency alias in the cache
func (r *ResolutionService) cacheAlias(packageName, packageType, gitURL string) error {
	alias := models.NewDependencyAlias(packageName, packageType, gitURL)
	return r.cache.StoreDependencyAlias(alias)
}
