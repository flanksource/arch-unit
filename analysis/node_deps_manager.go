package analysis

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	flanksourceContext "github.com/flanksource/commons/context"
)

var (
	depsOnce    sync.Once
	depsInstalled bool
	depsError   error
)

// NodeDependenciesManager manages Node.js dependencies for parsers
type NodeDependenciesManager struct {
	baseDir string
}

// NewNodeDependenciesManager creates a new Node dependencies manager
func NewNodeDependenciesManager() *NodeDependenciesManager {
	homeDir, _ := os.UserHomeDir()
	baseDir := filepath.Join(homeDir, ".arch-unit", "node_modules")
	
	return &NodeDependenciesManager{
		baseDir: baseDir,
	}
}

// GetNodeModulesPath returns the path to the node_modules directory
func (m *NodeDependenciesManager) GetNodeModulesPath() string {
	return m.baseDir
}

// EnsureDependencies ensures all required Node.js dependencies are installed
func (m *NodeDependenciesManager) EnsureDependencies(ctx flanksourceContext.Context) error {
	depsOnce.Do(func() {
		depsError = m.installDependencies(ctx)
		if depsError == nil {
			depsInstalled = true
		}
	})
	
	return depsError
}

// installDependencies installs required Node.js dependencies
func (m *NodeDependenciesManager) installDependencies(ctx flanksourceContext.Context) error {
	// Create .arch-unit directory if it doesn't exist
	archUnitDir := filepath.Dir(m.baseDir)
	ctx.Infof("Creating .arch-unit directory...")
	
	if err := os.MkdirAll(archUnitDir, 0755); err != nil {
		ctx.Errorf("Failed to create directory: %v", err)
		return fmt.Errorf("failed to create .arch-unit directory: %w", err)
	}

	// Check if package.json exists
	packageJSONPath := filepath.Join(archUnitDir, "package.json")
	ctx.Infof("Checking package.json...")
	
	if _, err := os.Stat(packageJSONPath); os.IsNotExist(err) {
		// Create package.json
		ctx.Infof("Creating package.json with dependencies...")
		
		packageJSON := map[string]interface{}{
			"name":        "arch-unit-parsers",
			"version":     "1.0.0",
			"description": "Node.js dependencies for arch-unit AST parsers",
			"private":     true,
			"dependencies": map[string]string{
				"typescript": "^5.0.0",
				"acorn":      "^8.0.0",
				"acorn-walk": "^8.0.0",
				"@babel/parser": "^7.0.0",
				"@babel/traverse": "^7.0.0",
				"@babel/types": "^7.0.0",
			},
		}
		
		data, err := json.MarshalIndent(packageJSON, "", "  ")
		if err != nil {
			ctx.Errorf("Failed to create package.json: %v", err)
			return fmt.Errorf("failed to marshal package.json: %w", err)
		}
		
		if err := os.WriteFile(packageJSONPath, data, 0644); err != nil {
			ctx.Errorf("Failed to write package.json: %v", err)
			return fmt.Errorf("failed to write package.json: %w", err)
		}
	}

	// Check if node_modules exists and has the required packages
	ctx.Infof("Checking existing packages...")
	
	if m.checkDependenciesInstalled() {
		ctx.Infof("Dependencies already installed")
		ctx.Debugf("Node.js dependencies already installed in %s", m.baseDir)
		return nil
	}

	// Install dependencies
	ctx.Infof("Installing packages with npm...")
	
	cmd := exec.Command("npm", "install", "--no-save", "--no-audit", "--no-fund")
	cmd.Dir = archUnitDir
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try with yarn if npm fails
		ctx.Warnf("npm failed, trying yarn...")
		ctx.Debugf("npm install failed, trying yarn...")
		
		cmd = exec.Command("yarn", "install", "--silent")
		cmd.Dir = archUnitDir
		output, err = cmd.CombinedOutput()
		
		if err != nil {
			ctx.Errorf("Failed to install dependencies: %v", err)
			return fmt.Errorf("failed to install Node.js dependencies: %w\nOutput: %s", err, string(output))
		}
	}

	// Verify installation
	ctx.Infof("Verifying installation...")
	
	if !m.checkDependenciesInstalled() {
		ctx.Errorf("Dependencies not properly installed")
		return fmt.Errorf("dependencies were not properly installed")
	}

	ctx.Infof("✓ TypeScript, Acorn, and Babel parsers installed")
	ctx.Infof("Node.js dependencies installed successfully")
	return nil
}

// checkDependenciesInstalled checks if all required dependencies are installed
func (m *NodeDependenciesManager) checkDependenciesInstalled() bool {
	requiredPackages := []string{
		"typescript",
		"acorn",
		"acorn-walk",
		"@babel/parser",
		"@babel/traverse",
		"@babel/types",
	}

	for _, pkg := range requiredPackages {
		pkgPath := filepath.Join(m.baseDir, pkg)
		if _, err := os.Stat(pkgPath); os.IsNotExist(err) {
			return false
		}
	}

	return true
}

// CreateParserScript creates a temporary script file with proper module resolution
func (m *NodeDependenciesManager) CreateParserScript(ctx flanksourceContext.Context, scriptContent string, scriptType string) (string, error) {
	// Ensure dependencies are installed
	if err := m.EnsureDependencies(ctx); err != nil {
		return "", fmt.Errorf("failed to ensure dependencies: %w", err)
	}

	// Create temp file for the script
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("%s_parser_*.js", scriptType))
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	// Prepend module resolution setup to the script
	moduleSetup := fmt.Sprintf(`
// Set up module resolution
const Module = require('module');
const originalResolveFilename = Module._resolveFilename;
Module._resolveFilename = function(request, parent, isMain) {
	// For our parser dependencies, resolve from .arch-unit/node_modules
	if (request === 'typescript' || request === 'acorn' || request === 'acorn-walk' || 
	    request.startsWith('@babel/')) {
		try {
			return originalResolveFilename.call(this, request, {
				paths: ['%s']
			});
		} catch (e) {
			// Fallback to original resolution
		}
	}
	return originalResolveFilename.call(this, request, parent, isMain);
};

`, m.baseDir)

	fullScript := moduleSetup + scriptContent
	
	if _, err := tmpFile.WriteString(fullScript); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write script: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	return tmpFile.Name(), nil
}