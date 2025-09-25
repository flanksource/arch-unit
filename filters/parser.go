package filters

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flanksource/arch-unit/models"
)

const ArchUnitFileName = ".ARCHUNIT"

type Parser struct {
	rootDir string
}

func NewParser(rootDir string) *Parser {
	return &Parser{rootDir: rootDir}
}

func (p *Parser) LoadRules() ([]models.RuleSet, error) {
	var ruleSets []models.RuleSet

	err := filepath.Walk(p.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.Name() == ArchUnitFileName {
			ruleSet, err := p.parseRuleFile(path)
			if err != nil {
				return fmt.Errorf("failed to parse %s: %w", path, err)
			}
			ruleSets = append(ruleSets, *ruleSet)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return ruleSets, nil
}

func (p *Parser) parseRuleFile(path string) (*models.RuleSet, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

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

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		rule, err := p.parseLine(line, relPath, lineNum, ruleSet.Path)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}

		if rule != nil {
			ruleSet.Rules = append(ruleSet.Rules, *rule)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return ruleSet, nil
}

func (p *Parser) parseLine(line, sourceFile string, lineNum int, scope string) (*models.Rule, error) {
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

func (p *Parser) GetRulesForFile(filePath string, ruleSets []models.RuleSet) *models.RuleSet {
	// Find the most specific ruleset that applies to this file
	var bestMatch *models.RuleSet
	bestMatchDepth := -1

	absPath, _ := filepath.Abs(filePath)
	dir := filepath.Dir(absPath)

	for i := range ruleSets {
		ruleSet := &ruleSets[i]
		absRulePath, _ := filepath.Abs(ruleSet.Path)

		if strings.HasPrefix(dir, absRulePath) {
			depth := strings.Count(absRulePath, string(filepath.Separator))
			if depth > bestMatchDepth {
				bestMatch = ruleSet
				bestMatchDepth = depth
			}
		}
	}

	if bestMatch == nil && len(ruleSets) > 0 {
		// Use root rules if no specific match
		for i := range ruleSets {
			if ruleSets[i].Path == p.rootDir || ruleSets[i].Path == "." {
				return &ruleSets[i]
			}
		}
	}

	return bestMatch
}
