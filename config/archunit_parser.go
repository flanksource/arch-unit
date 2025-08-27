package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/models"
	"github.com/flanksource/commons/logger"
)

const ArchUnitFileName = ".ARCHUNIT"

// ArchUnitParser handles parsing of .ARCHUNIT files
type ArchUnitParser struct {
	rootDir string
}

// NewArchUnitParser creates a new .ARCHUNIT parser
func NewArchUnitParser(rootDir string) *ArchUnitParser {
	return &ArchUnitParser{
		rootDir: rootDir,
	}
}

// LoadArchUnitRules loads all .ARCHUNIT files in the directory tree
func (p *ArchUnitParser) LoadArchUnitRules() ([]models.RuleSet, error) {
	var ruleSets []models.RuleSet
	
	logger.Debugf("Searching for .ARCHUNIT files in %s", p.rootDir)
	
	err := filepath.Walk(p.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files with errors
		}
		
		if info.Name() == ArchUnitFileName {
			logger.Debugf("Found .ARCHUNIT file at %s", path)
			ruleSet, err := p.parseArchUnitFile(path)
			if err != nil {
				logger.Warnf("Failed to parse %s: %v", path, err)
				return nil // Continue with other files
			}
			ruleSets = append(ruleSets, *ruleSet)
		}
		
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	logger.Infof("Loaded %d .ARCHUNIT rule sets", len(ruleSets))
	return ruleSets, nil
}

// parseArchUnitFile parses a single .ARCHUNIT file
func (p *ArchUnitParser) parseArchUnitFile(path string) (*models.RuleSet, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	// Use relative path for rule source
	relPath, err := filepath.Rel(".", path)
	if err != nil || strings.HasPrefix(relPath, "..") {
		relPath = path
	}
	
	ruleSet := &models.RuleSet{
		Path:  filepath.Dir(path),
		Rules: []models.Rule{},
	}
	
	scanner := bufio.NewScanner(file)
	lineNum := 0
	
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		rule, err := p.parseArchUnitLine(line, relPath, lineNum, ruleSet.Path)
		if err != nil {
			logger.Warnf("Line %d in %s: %v", lineNum, path, err)
			continue
		}
		
		if rule != nil {
			ruleSet.Rules = append(ruleSet.Rules, *rule)
		}
	}
	
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	
	logger.Debugf("Parsed %d rules from %s", len(ruleSet.Rules), path)
	return ruleSet, nil
}

// parseArchUnitLine parses a single line of .ARCHUNIT syntax
func (p *ArchUnitParser) parseArchUnitLine(line, sourceFile string, lineNum int, scope string) (*models.Rule, error) {
	originalLine := line
	rule := &models.Rule{
		SourceFile:   sourceFile,
		LineNumber:   lineNum,
		Scope:        scope,
		OriginalLine: originalLine,
		Type:         models.RuleTypeAllow,
	}
	
	// Check for file-specific pattern [file-pattern] at the beginning
	if strings.HasPrefix(line, "[") {
		endIdx := strings.Index(line, "]")
		if endIdx == -1 {
			return nil, fmt.Errorf("invalid file-specific rule format (missing closing bracket): %s", originalLine)
		}
		rule.FilePattern = strings.TrimSpace(line[1:endIdx])
		if rule.FilePattern == "" {
			return nil, fmt.Errorf("invalid file-specific rule format (empty pattern): %s", originalLine)
		}
		line = strings.TrimSpace(line[endIdx+1:])
		if line == "" {
			return nil, fmt.Errorf("invalid file-specific rule format (missing rule after pattern): %s", originalLine)
		}
	}
	
	// Determine rule type based on prefix
	if strings.HasPrefix(line, "+") {
		rule.Type = models.RuleTypeOverride
		line = line[1:]
	} else if strings.HasPrefix(line, "!") {
		rule.Type = models.RuleTypeDeny
		line = line[1:]
	}
	
	// Check if it's a method-specific rule (contains :)
	if strings.Contains(line, ":") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid method rule format: %s", originalLine)
		}
		
		rule.Package = strings.TrimSpace(parts[0])
		methodPart := strings.TrimSpace(parts[1])
		
		// Handle method negation
		if strings.HasPrefix(methodPart, "!") {
			rule.Method = methodPart[1:]
			if rule.Type == models.RuleTypeAllow {
				rule.Type = models.RuleTypeDeny
			}
		} else {
			rule.Method = methodPart
		}
	} else {
		// It's a package/folder rule
		rule.Pattern = line
	}
	
	return rule, nil
}

// ConvertArchUnitToYAML converts .ARCHUNIT rules to YAML config format
func ConvertArchUnitToYAML(ruleSets []models.RuleSet) *models.Config {
	config := &models.Config{
		Version: "1.0",
		Rules:   make(map[string]models.RuleConfig),
	}
	
	// Group rules by file pattern
	rulesByPattern := make(map[string][]string)
	
	for _, ruleSet := range ruleSets {
		for _, rule := range ruleSet.Rules {
			pattern := "**"
			if rule.FilePattern != "" {
				pattern = rule.FilePattern
			}
			
			// Convert rule to import string
			importStr := ConvertRuleToImportString(&rule)
			if importStr != "" {
				rulesByPattern[pattern] = append(rulesByPattern[pattern], importStr)
			}
		}
	}
	
	// Convert to RuleConfig
	for pattern, imports := range rulesByPattern {
		config.Rules[pattern] = models.RuleConfig{
			Imports: imports,
		}
		logger.Debugf("Created rule pattern: %s with %d imports", pattern, len(imports))
	}
	
	return config
}

// ConvertRuleToImportString converts a Rule to an import string for YAML config
func ConvertRuleToImportString(rule *models.Rule) string {
	prefix := ""
	switch rule.Type {
	case models.RuleTypeDeny:
		prefix = "!"
	case models.RuleTypeOverride:
		prefix = "+"
	}
	
	if rule.Method != "" {
		// Method-specific rule
		pkg := rule.Package
		if pkg == "" {
			pkg = "*"
		}
		return fmt.Sprintf("%s%s:%s", prefix, pkg, rule.Method)
	} else if rule.Pattern != "" {
		// Package/pattern rule
		return prefix + rule.Pattern
	}
	
	return ""
}

// ParseArchUnitSyntax parses a string containing .ARCHUNIT syntax rules
// This can be used in YAML files to support mixed syntax
func ParseArchUnitSyntax(syntax string) []string {
	var imports []string
	
	// Split by whitespace or newlines
	lines := strings.Fields(syntax)
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Parse each line as a simple .ARCHUNIT rule
		// (without file-specific patterns in this context)
		imports = append(imports, line)
	}
	
	return imports
}

// MergeArchUnitWithYAML merges .ARCHUNIT rules with existing YAML config
func MergeArchUnitWithYAML(yamlConfig *models.Config, archunitRules []models.RuleSet) {
	if yamlConfig.Rules == nil {
		yamlConfig.Rules = make(map[string]models.RuleConfig)
	}
	
	// Convert and merge .ARCHUNIT rules
	archunitConfig := ConvertArchUnitToYAML(archunitRules)
	
	for pattern, ruleConfig := range archunitConfig.Rules {
		if existing, exists := yamlConfig.Rules[pattern]; exists {
			// Merge imports
			existing.Imports = append(existing.Imports, ruleConfig.Imports...)
			yamlConfig.Rules[pattern] = existing
		} else {
			yamlConfig.Rules[pattern] = ruleConfig
		}
	}
	
	logger.Debugf("Merged %d .ARCHUNIT rule patterns into config", len(archunitConfig.Rules))
}