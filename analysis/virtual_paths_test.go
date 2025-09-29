package analysis_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/analysis"
)

var _ = Describe("Virtual Path Manager", func() {
	var manager *analysis.VirtualPathManager

	BeforeEach(func() {
		manager = analysis.NewVirtualPathManager()
	})

	Context("when creating virtual paths for SQL connections", func() {
		It("should create deterministic paths for PostgreSQL", func() {
			source := analysis.AnalysisSource{
				Type:             "sql_connection",
				ConnectionString: "postgres://user:pass@localhost:5432/testdb",
			}

			path1 := manager.CreateVirtualPath(source)
			path2 := manager.CreateVirtualPath(source)

			Expect(path1).To(Equal(path2))
			Expect(path1).To(HavePrefix("sql://"))
			Expect(path1).To(ContainSubstring("localhost"))
			Expect(path1).To(ContainSubstring("testdb"))
			Expect(path1).NotTo(ContainSubstring("pass"))
		})

		It("should create paths for different database types", func() {
			pgSource := analysis.AnalysisSource{
				Type:             "sql_connection",
				ConnectionString: "postgres://user@localhost/db1",
			}

			mysqlSource := analysis.AnalysisSource{
				Type:             "sql_connection",
				ConnectionString: "mysql://user@localhost/db2",
			}

			sqliteSource := analysis.AnalysisSource{
				Type:             "sql_connection",
				ConnectionString: "sqlite3://./test.db",
			}

			pgPath := manager.CreateVirtualPath(pgSource)
			mysqlPath := manager.CreateVirtualPath(mysqlSource)
			sqlitePath := manager.CreateVirtualPath(sqliteSource)

			Expect(pgPath).To(HavePrefix("sql://postgres_"))
			Expect(mysqlPath).To(HavePrefix("sql://mysql_"))
			Expect(sqlitePath).To(HavePrefix("sql://sqlite3_"))

			Expect(pgPath).NotTo(Equal(mysqlPath))
			Expect(mysqlPath).NotTo(Equal(sqlitePath))
		})

		It("should handle complex connection strings", func() {
			source := analysis.AnalysisSource{
				Type:             "sql_connection",
				ConnectionString: "postgres://user:complex-pass@db.example.com:5432/my_database?sslmode=require",
			}

			path := manager.CreateVirtualPath(source)

			Expect(path).To(HavePrefix("sql://postgres_"))
			Expect(path).To(ContainSubstring("db_example_com"))
			Expect(path).To(ContainSubstring("my_database"))
			Expect(path).NotTo(ContainSubstring("complex-pass"))
			Expect(path).NotTo(ContainSubstring("sslmode"))
		})
	})

	Context("when creating virtual paths for OpenAPI URLs", func() {
		It("should create deterministic paths for API URLs", func() {
			source := analysis.AnalysisSource{
				Type: "openapi_url",
				URL:  "https://api.example.com/openapi.json",
			}

			path1 := manager.CreateVirtualPath(source)
			path2 := manager.CreateVirtualPath(source)

			Expect(path1).To(Equal(path2))
			Expect(path1).To(HavePrefix("openapi://"))
			Expect(path1).To(ContainSubstring("api_example_com"))
			Expect(path1).To(ContainSubstring("openapi_json"))
		})

		It("should handle different URL formats", func() {
			httpSource := analysis.AnalysisSource{
				Type: "openapi_url",
				URL:  "http://localhost:3000/api-docs",
			}

			httpsSource := analysis.AnalysisSource{
				Type: "openapi_url",
				URL:  "https://api.service.com/v1/swagger.yaml",
			}

			httpPath := manager.CreateVirtualPath(httpSource)
			httpsPath := manager.CreateVirtualPath(httpsSource)

			Expect(httpPath).To(HavePrefix("openapi://"))
			Expect(httpPath).To(ContainSubstring("localhost"))
			Expect(httpPath).To(ContainSubstring("api_docs"))

			Expect(httpsPath).To(HavePrefix("openapi://"))
			Expect(httpsPath).To(ContainSubstring("api_service_com"))
			Expect(httpsPath).To(ContainSubstring("v1_swagger_yaml"))

			Expect(httpPath).NotTo(Equal(httpsPath))
		})

		It("should handle URLs with query parameters", func() {
			source := analysis.AnalysisSource{
				Type: "openapi_url",
				URL:  "https://api.example.com/openapi.json?version=v1&format=json",
			}

			path := manager.CreateVirtualPath(source)

			Expect(path).To(HavePrefix("openapi://"))
			Expect(path).To(ContainSubstring("api_example_com"))
			Expect(path).To(ContainSubstring("openapi_json"))
			Expect(path).NotTo(ContainSubstring("version"))
			Expect(path).NotTo(ContainSubstring("format"))
		})
	})

	Context("when handling invalid inputs", func() {
		It("should handle empty connection strings", func() {
			source := analysis.AnalysisSource{
				Type:             "sql_connection",
				ConnectionString: "",
			}

			path := manager.CreateVirtualPath(source)
			Expect(path).To(HavePrefix("sql://"))
			Expect(path).To(ContainSubstring("unknown"))
		})

		It("should handle invalid URLs", func() {
			source := analysis.AnalysisSource{
				Type: "openapi_url",
				URL:  "not-a-valid-url",
			}

			path := manager.CreateVirtualPath(source)
			Expect(path).To(HavePrefix("openapi://"))
			Expect(path).To(ContainSubstring("unknown"))
		})

		It("should handle unknown source types", func() {
			source := analysis.AnalysisSource{
				Type: "unknown_type",
				URL:  "https://example.com",
			}

			path := manager.CreateVirtualPath(source)
			Expect(path).To(HavePrefix("virtual://"))
			Expect(path).To(ContainSubstring("unknown_type"))
		})
	})

	Context("when testing identifier sanitization", func() {
		It("should sanitize special characters", func() {
			source := analysis.AnalysisSource{
				Type: "openapi_url",
				URL:  "https://api-server.example.com:8080/api/v1/openapi.json",
			}

			path := manager.CreateVirtualPath(source)

			// Extract the identifier part (after the scheme)
			identifier := strings.TrimPrefix(path, "openapi://")

			Expect(identifier).NotTo(ContainSubstring("-"))
			Expect(identifier).NotTo(ContainSubstring(":"))
			Expect(identifier).NotTo(ContainSubstring("/"))
			Expect(identifier).To(ContainSubstring("_"))
		})

		It("should handle unicode characters", func() {
			source := analysis.AnalysisSource{
				Type: "openapi_url",
				URL:  "https://api.测试.com/openapi.json",
			}

			path := manager.CreateVirtualPath(source)
			Expect(path).To(HavePrefix("openapi://"))
			// Should handle or escape unicode appropriately
		})
	})
})