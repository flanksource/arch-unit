package config_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/analysis/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var _ = Describe("Config Loader", func() {
	var (
		tempDir string
		loader  *config.ConfigLoader
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "arch-unit-config-test")
		Expect(err).NotTo(HaveOccurred())
		loader = config.NewConfigLoader(tempDir)
	})

	AfterEach(func() {
		err := os.RemoveAll(tempDir)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when loading valid configuration", func() {
		It("should load SQL analyzer configuration", func() {
			configContent := `version: "1.0"
analyzers:
  - path: "**/*.sql"
    analyzer: "sql"
    options:
      dialect: "postgresql"
      connection: "postgres://user:pass@localhost/testdb"
`
			configFile := filepath.Join(tempDir, ".arch-ast.yaml")
			err := os.WriteFile(configFile, []byte(configContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.LoadConfig("")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Version).To(Equal("1.0"))
			Expect(cfg.Analyzers).To(HaveLen(1))

			analyzer := cfg.Analyzers[0]
			Expect(analyzer.Path).To(Equal("**/*.sql"))
			Expect(analyzer.Analyzer).To(Equal("sql"))
			Expect(analyzer.Options).NotTo(BeNil())

			sqlOpts := analyzer.GetSQLOptions()
			Expect(sqlOpts.Dialect).To(Equal("postgresql"))
			Expect(sqlOpts.ConnectionString).To(Equal("postgres://user:pass@localhost/testdb"))
		})

		It("should load OpenAPI analyzer configuration", func() {
			configContent := `version: "1.0"
analyzers:
  - path: "api/openapi.yaml"
    analyzer: "openapi"
    options:
      version: "3.1"
      url: "https://api.example.com/openapi.json"
`
			configFile := filepath.Join(tempDir, ".arch-ast.yaml")
			err := os.WriteFile(configFile, []byte(configContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.LoadConfig("")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Analyzers).To(HaveLen(1))

			analyzer := cfg.Analyzers[0]
			Expect(analyzer.Path).To(Equal("api/openapi.yaml"))
			Expect(analyzer.Analyzer).To(Equal("openapi"))

			openAPIopts := analyzer.GetOpenAPIOptions()
			Expect(openAPIopts.Version).To(Equal("3.1"))
			Expect(openAPIopts.URL).To(Equal("https://api.example.com/openapi.json"))
		})

		It("should load multiple analyzers", func() {
			configContent := `version: "1.0"
analyzers:
  - path: "**/*.sql"
    analyzer: "sql"
    options:
      dialect: "mysql"
      connection: "mysql://user:pass@localhost/testdb"
  - path: "api/*.yaml"
    analyzer: "openapi"
    options:
      version: "3.0"
  - path: "src/**/*.go"
    analyzer: "custom"
    options:
      command: "custom-analyzer"
      field_mappings:
        custom_option: "value"
`
			configFile := filepath.Join(tempDir, ".arch-ast.yaml")
			err := os.WriteFile(configFile, []byte(configContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.LoadConfig("")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Analyzers).To(HaveLen(3))

			sqlAnalyzer := cfg.Analyzers[0]
			Expect(sqlAnalyzer.Analyzer).To(Equal("sql"))
			sqlOpts := sqlAnalyzer.GetSQLOptions()
			Expect(sqlOpts.Dialect).To(Equal("mysql"))

			openAPIAnalyzer := cfg.Analyzers[1]
			Expect(openAPIAnalyzer.Analyzer).To(Equal("openapi"))
			openAPIopts := openAPIAnalyzer.GetOpenAPIOptions()
			Expect(openAPIopts.Version).To(Equal("3.0"))

			customAnalyzer := cfg.Analyzers[2]
			Expect(customAnalyzer.Analyzer).To(Equal("custom"))
			customOpts := customAnalyzer.GetCustomOptions()
			Expect(customOpts.Command).To(Equal("custom-analyzer"))
			Expect(customOpts.FieldMappings).NotTo(BeNil())
		})

		It("should use custom config file path", func() {
			customConfigPath := filepath.Join(tempDir, "my-config.yaml")
			configContent := `version: "1.0"
analyzers:
  - path: "test.sql"
    analyzer: "sql"
    options:
      dialect: "sqlite"
`
			err := os.WriteFile(customConfigPath, []byte(configContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.LoadConfig(customConfigPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Version).To(Equal("1.0"))
		})
	})

	Context("when configuration file does not exist", func() {
		It("should return default config for missing default config", func() {
			cfg, err := loader.LoadConfig("")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Version).To(Equal("1.0"))
			Expect(cfg.Analyzers).To(BeEmpty())
		})

		It("should return error for missing custom config", func() {
			cfg, err := loader.LoadConfig(filepath.Join(tempDir, "nonexistent.yaml"))
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
		})
	})

	Context("when configuration is invalid", func() {
		It("should return error for invalid YAML", func() {
			configFile := filepath.Join(tempDir, ".arch-ast.yaml")
			invalidYAML := `version: "1.0"
analyzers:
  - path: "test.sql"
    analyzer: sql
    options:
      dialect: postgresql
      connection: [invalid yaml structure
`
			err := os.WriteFile(configFile, []byte(invalidYAML), 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.LoadConfig("")
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
		})

		It("should return error for missing version", func() {
			configFile := filepath.Join(tempDir, ".arch-ast.yaml")
			configContent := `analyzers:
  - path: "test.sql"
    analyzer: "sql"
`
			err := os.WriteFile(configFile, []byte(configContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.LoadConfig("")
			Expect(err).To(HaveOccurred())
			Expect(cfg).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("version is required"))
		})

		It("should accept empty analyzers list", func() {
			configFile := filepath.Join(tempDir, ".arch-ast.yaml")
			configContent := `version: "1.0"
analyzers: []
`
			err := os.WriteFile(configFile, []byte(configContent), 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg, err := loader.LoadConfig("")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
			Expect(cfg.Analyzers).To(BeEmpty())
		})
	})
})

var _ = Describe("Path Matcher", func() {
	var (
		matcher *config.PathMatcher
		tempDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "path-matcher-test")
		Expect(err).NotTo(HaveOccurred())
		matcher = config.NewPathMatcher(tempDir)
	})

	AfterEach(func() {
		err := os.RemoveAll(tempDir)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when matching analyzer configurations", func() {
		It("should match SQL files to SQL analyzer", func() {
			analyzers := []config.AnalyzerConfig{
				{
					Path:     "**/*.sql",
					Analyzer: "sql",
				},
			}

			result := matcher.MatchAnalyzer("database/schema.sql", analyzers)
			Expect(result).NotTo(BeNil())
			Expect(result.Analyzer).To(Equal("sql"))
		})

		It("should match OpenAPI files to OpenAPI analyzer", func() {
			analyzers := []config.AnalyzerConfig{
				{
					Path:     "api/*.yaml",
					Analyzer: "openapi",
				},
			}

			result := matcher.MatchAnalyzer("api/openapi.yaml", analyzers)
			Expect(result).NotTo(BeNil())
			Expect(result.Analyzer).To(Equal("openapi"))
		})

		It("should return nil for non-matching files", func() {
			analyzers := []config.AnalyzerConfig{
				{
					Path:     "**/*.sql",
					Analyzer: "sql",
				},
			}

			result := matcher.MatchAnalyzer("src/main.go", analyzers)
			Expect(result).To(BeNil())
		})
	})

	Context("when identifying virtual paths", func() {
		It("should identify virtual paths correctly", func() {
			Expect(matcher.IsVirtualPath("virtual://sql/localhost_db")).To(BeTrue())
			Expect(matcher.IsVirtualPath("virtual://openapi/api_example_com")).To(BeTrue())
			Expect(matcher.IsVirtualPath("regular/file.sql")).To(BeFalse())
			Expect(matcher.IsVirtualPath("**/*.sql")).To(BeFalse())
		})

		It("should extract virtual path types", func() {
			sqlType := matcher.ExtractVirtualPathType("virtual://sql/localhost_db")
			Expect(sqlType).To(Equal("sql"))

			openAPIType := matcher.ExtractVirtualPathType("virtual://openapi/api_example_com")
			Expect(openAPIType).To(Equal("openapi"))

			emptyType := matcher.ExtractVirtualPathType("regular/file.sql")
			Expect(emptyType).To(Equal(""))
		})
	})

	Context("when normalizing paths", func() {
		It("should normalize paths correctly", func() {
			// On macOS/Linux, backslashes are treated as literal characters in path names
			// The NormalizePath just converts to forward slashes using filepath.ToSlash
			normalized := matcher.NormalizePath("src/main.go")
			Expect(normalized).To(Equal("src/main.go"))

			// Test with absolute path relative to working dir
			absPath := filepath.Join(tempDir, "src", "main.go")
			normalized = matcher.NormalizePath(absPath)
			Expect(normalized).To(Equal("src/main.go"))
		})
	})
})