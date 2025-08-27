package analysis

import (
	"fmt"
	"strings"

	"github.com/flanksource/arch-unit/models"
)

// CommentAnalysisPrompt is the main template for comment quality analysis
const CommentAnalysisPrompt = `Analyze this code comment for quality issues:

Comment Text: "%s"
Context: %s (line %d)
Type: %s
Word Count: %d

Please assess the comment for:

1. **Verbosity**: Is it overly verbose with unnecessary words, repetition, or redundant information?
   - Verbose comments repeat obvious information or use excessive words
   - Good comments are concise and to the point

2. **Descriptiveness**: Is it descriptive and helpful? Does it explain WHY something is done, not just WHAT?
   - Good comments explain intent, business logic, edge cases, or non-obvious behavior
   - Poor comments state the obvious (e.g., "increment counter" for counter++)
   - Documentation comments should explain purpose, parameters, and return values

3. **Quality Issues**: Are there specific problems like being too vague, obvious, or misleading?
   - Vague comments provide no useful information
   - Obvious comments repeat what the code clearly shows
   - Misleading comments don't match the actual implementation

Guidelines for good comments:
- Explain WHY, not WHAT (the code shows what)
- Describe business logic, constraints, or edge cases
- Document non-obvious behavior or complex algorithms
- Provide context that isn't clear from the code itself
- Keep documentation comments comprehensive but focused

Respond in valid JSON format only (no additional text):
{
  "is_verbose": boolean,
  "is_descriptive": boolean,
  "score": float (0.0-1.0, where 1.0 is excellent quality),
  "issues": ["specific issue 1", "specific issue 2"],
  "suggestions": ["specific improvement 1", "specific improvement 2"]
}`

// BuildCommentAnalysisPrompt builds a comment analysis prompt with specific comment details
func BuildCommentAnalysisPrompt(comment models.Comment) string {
	contextInfo := "Unknown context"
	if comment.Context != "" {
		contextInfo = comment.Context
	}

	return fmt.Sprintf(CommentAnalysisPrompt,
		comment.Text,
		contextInfo,
		comment.StartLine,
		comment.Type,
		comment.WordCount,
	)
}

// NameAnalysisPrompt is for analyzing identifier names
const NameAnalysisPrompt = `Analyze this code identifier name for quality:

Name: "%s"
Context: %s
Type: %s
Length: %d characters

Assess the name for:
1. **Clarity**: Is it clear what this represents?
2. **Appropriateness**: Does it follow naming conventions?
3. **Length**: Is it appropriately sized (not too short or verbose)?

Common naming issues:
- Too generic (temp, data, info, manager)
- Too abbreviated (usr, ctx without clear context)
- Too verbose (unnecessarily long)
- Misleading or unclear purpose

Respond in JSON:
{
  "is_clear": boolean,
  "is_appropriate": boolean,
  "is_good_length": boolean,
  "score": float (0.0-1.0),
  "issues": ["issue1"],
  "suggestions": ["suggestion1"]
}`

// FileStructurePrompt analyzes overall file structure
const FileStructurePrompt = `Analyze this file's structure for quality issues:

File: %s
Language: %s
Total Lines: %d
Functions: %d
Types: %d
Comments: %d

Guidelines:
- Files should be focused on a single responsibility
- Excessive length may indicate need for refactoring
- Good balance of code and documentation
- Clear organization and structure

Assess for:
1. **Length**: Is the file too long?
2. **Organization**: Is it well-structured?
3. **Balance**: Good ratio of code to comments?

Respond in JSON:
{
  "is_appropriate_length": boolean,
  "is_well_organized": boolean,
  "has_good_balance": boolean,
  "score": float (0.0-1.0),
  "issues": ["issue1"],
  "suggestions": ["suggestion1"]
}`

// BuildNameAnalysisPrompt builds a name analysis prompt
func BuildNameAnalysisPrompt(name, context, nameType string) string {
	return fmt.Sprintf(NameAnalysisPrompt, name, context, nameType, len(name))
}

// BuildFileStructurePrompt builds a file structure analysis prompt
func BuildFileStructurePrompt(ast *models.GenericAST) string {
	return fmt.Sprintf(FileStructurePrompt,
		ast.FilePath,
		ast.Language,
		ast.LineCount,
		len(ast.Functions),
		len(ast.Types),
		len(ast.Comments),
	)
}

// PromptTemplate represents a reusable prompt template
type PromptTemplate struct {
	Name        string
	Template    string
	Description string
}

// PredefinedPrompts contains all predefined analysis prompts
var PredefinedPrompts = map[string]PromptTemplate{
	"comment-quality": {
		Name:        "comment-quality",
		Template:    CommentAnalysisPrompt,
		Description: "Analyzes code comments for quality, verbosity, and descriptiveness",
	},
	"name-analysis": {
		Name:        "name-analysis", 
		Template:    NameAnalysisPrompt,
		Description: "Analyzes identifier names for clarity and appropriateness",
	},
	"file-structure": {
		Name:        "file-structure",
		Template:    FileStructurePrompt,
		Description: "Analyzes overall file structure and organization",
	},
}

// GetPromptTemplate retrieves a prompt template by name
func GetPromptTemplate(name string) (PromptTemplate, bool) {
	template, exists := PredefinedPrompts[name]
	return template, exists
}

// CustomPromptBuilder helps build custom prompts for specific analysis needs
type CustomPromptBuilder struct {
	basePrompt  string
	context     map[string]string
	guidelines  []string
	assessments []string
}

// NewCustomPromptBuilder creates a new custom prompt builder
func NewCustomPromptBuilder(basePrompt string) *CustomPromptBuilder {
	return &CustomPromptBuilder{
		basePrompt: basePrompt,
		context:    make(map[string]string),
	}
}

// AddContext adds context information to the prompt
func (cpb *CustomPromptBuilder) AddContext(key, value string) *CustomPromptBuilder {
	cpb.context[key] = value
	return cpb
}

// AddGuideline adds a guideline to the prompt
func (cpb *CustomPromptBuilder) AddGuideline(guideline string) *CustomPromptBuilder {
	cpb.guidelines = append(cpb.guidelines, guideline)
	return cpb
}

// AddAssessment adds an assessment point to the prompt
func (cpb *CustomPromptBuilder) AddAssessment(assessment string) *CustomPromptBuilder {
	cpb.assessments = append(cpb.assessments, assessment)
	return cpb
}

// Build constructs the final prompt
func (cpb *CustomPromptBuilder) Build() string {
	var prompt strings.Builder
	
	prompt.WriteString(cpb.basePrompt)
	prompt.WriteString("\n\n")
	
	if len(cpb.context) > 0 {
		prompt.WriteString("Context:\n")
		for key, value := range cpb.context {
			prompt.WriteString(fmt.Sprintf("- %s: %s\n", key, value))
		}
		prompt.WriteString("\n")
	}
	
	if len(cpb.guidelines) > 0 {
		prompt.WriteString("Guidelines:\n")
		for _, guideline := range cpb.guidelines {
			prompt.WriteString(fmt.Sprintf("- %s\n", guideline))
		}
		prompt.WriteString("\n")
	}
	
	if len(cpb.assessments) > 0 {
		prompt.WriteString("Please assess for:\n")
		for i, assessment := range cpb.assessments {
			prompt.WriteString(fmt.Sprintf("%d. %s\n", i+1, assessment))
		}
		prompt.WriteString("\n")
	}
	
	prompt.WriteString("Respond in valid JSON format with your analysis.")
	
	return prompt.String()
}

// PromptVariants contains different versions of prompts for A/B testing or different contexts
var PromptVariants = map[string][]string{
	"comment-quality-short": {
		`Analyze this comment: "%s"
Rate 0-1 for: verbose, descriptive, helpful
JSON response only: {"is_verbose": bool, "is_descriptive": bool, "score": float, "issues": [], "suggestions": []}`,
	},
	"comment-quality-detailed": {
		CommentAnalysisPrompt,
	},
}

// GetPromptVariant gets a specific variant of a prompt
func GetPromptVariant(promptType string, variant int) string {
	variants, exists := PromptVariants[promptType]
	if !exists || variant >= len(variants) {
		return ""
	}
	return variants[variant]
}