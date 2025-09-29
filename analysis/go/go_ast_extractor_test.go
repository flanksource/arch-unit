package _go

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/internal/cache"
)

var _ = Describe("Go AST Extractor", func() {
	var (
		extractor *GoASTExtractor
		astCache  *cache.ASTCache
	)

	BeforeEach(func() {
		extractor = NewGoASTExtractor()
		astCache = cache.MustGetASTCache()
	})

	Context("when extracting from a Go file", func() {
		var testFile string

		BeforeEach(func() {
			testFile = filepath.Join("testdata", "calculator.go")
		})

		It("should successfully extract AST nodes", func() {
			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())
		})

		It("should find expected structs, methods, and functions", func() {
			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			nodes := result.Nodes

			var foundStruct, foundAdd, foundMultiply, foundMain bool
			for _, node := range nodes {
				switch {
				case node.TypeName == "Calculator" && node.NodeType == "type":
					foundStruct = true
				case node.MethodName == "Add" && node.TypeName == "Calculator":
					foundAdd = true
					Expect(node.CyclomaticComplexity).To(BeNumerically(">", 1), "Add method should have complexity > 1")
				case node.MethodName == "Multiply" && node.TypeName == "Calculator":
					foundMultiply = true
					Expect(node.CyclomaticComplexity).To(BeNumerically(">", 2), "Multiply method should have complexity > 2")
				case node.MethodName == "main":
					foundMain = true
				}
			}

			Expect(foundStruct).To(BeTrue(), "Should find Calculator struct")
			Expect(foundAdd).To(BeTrue(), "Should find Add method")
			Expect(foundMultiply).To(BeTrue(), "Should find Multiply method")
			Expect(foundMain).To(BeTrue(), "Should find main function")
		})
	})

	Context("when testing IsPrivate functionality", func() {
		It("should correctly identify private and public Go identifiers", func() {
			goCode := `package test

// PublicType is exported
type PublicType struct {
	PublicField   string
	privateField  int
}

// privateType is not exported
type privateType struct {
	PublicField   string
	privateField  int
}

// PublicFunction is exported
func PublicFunction() {
}

// privateFunction is not exported
func privateFunction() {
}

var PublicVar = "exported"
var privateVar = "not exported"
`

			result, err := extractor.ExtractFile(astCache, "/test/test.go", []byte(goCode))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			// Find nodes by name and check their privacy
			nodeMap := make(map[string]bool) // name -> IsPrivate
			for _, node := range result.Nodes {
				switch node.NodeType {
				case "type":
					nodeMap[node.TypeName] = node.IsPrivate
				case "method":
					nodeMap[node.MethodName] = node.IsPrivate
				case "field":
					nodeMap[node.FieldName] = node.IsPrivate
				case "variable":
					nodeMap[node.FieldName] = node.IsPrivate
				}
			}

			// Verify type visibility
			Expect(nodeMap["PublicType"]).To(BeFalse(), "PublicType should not be private")
			Expect(nodeMap["privateType"]).To(BeTrue(), "privateType should be private")

			// Verify function visibility
			Expect(nodeMap["PublicFunction"]).To(BeFalse(), "PublicFunction should not be private")
			Expect(nodeMap["privateFunction"]).To(BeTrue(), "privateFunction should be private")

			// Verify field visibility
			Expect(nodeMap["PublicField"]).To(BeFalse(), "PublicField should not be private")
			Expect(nodeMap["privateField"]).To(BeTrue(), "privateField should be private")

			// Verify variable visibility
			Expect(nodeMap["PublicVar"]).To(BeFalse(), "PublicVar should not be private")
			Expect(nodeMap["privateVar"]).To(BeTrue(), "privateVar should be private")
		})

		It("should correctly determine privacy with isPrivate helper method", func() {
			// Test public (exported) names
			Expect(extractor.isPrivate("PublicType")).To(BeFalse())
			Expect(extractor.isPrivate("PublicFunction")).To(BeFalse())
			Expect(extractor.isPrivate("PublicField")).To(BeFalse())
			Expect(extractor.isPrivate("HTTP")).To(BeFalse())

			// Test private (unexported) names
			Expect(extractor.isPrivate("privateType")).To(BeTrue())
			Expect(extractor.isPrivate("privateFunction")).To(BeTrue())
			Expect(extractor.isPrivate("privateField")).To(BeTrue())
			Expect(extractor.isPrivate("http")).To(BeTrue())

			// Test edge cases
			Expect(extractor.isPrivate("")).To(BeFalse())
			Expect(extractor.isPrivate("_private")).To(BeTrue())
			Expect(extractor.isPrivate("_Export")).To(BeFalse())
		})

		It("should correctly handle complex Go visibility scenarios", func() {
			testFile := filepath.Join("testdata", "visibility.go")
			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			// Create a map for easy lookup of elements by name and type
			elementMap := make(map[string]map[string]bool) // [nodeType][elementName] = IsPrivate

			for _, node := range result.Nodes {
				if elementMap[node.NodeType] == nil {
					elementMap[node.NodeType] = make(map[string]bool)
				}

				var elementName string
				switch node.NodeType {
				case "type":
					elementName = node.TypeName
				case "method":
					elementName = node.MethodName
				case "field":
					elementName = node.FieldName
				case "variable":
					elementName = node.FieldName
				}

				if elementName != "" {
					elementMap[node.NodeType][elementName] = node.IsPrivate
				}
			}

			// Test type visibility
			Expect(elementMap["type"]["PublicStruct"]).To(BeFalse(), "PublicStruct should not be private")
			Expect(elementMap["type"]["privateStruct"]).To(BeTrue(), "privateStruct should be private")
			Expect(elementMap["type"]["PublicInterface"]).To(BeFalse(), "PublicInterface should not be private")
			Expect(elementMap["type"]["privateInterface"]).To(BeTrue(), "privateInterface should be private")
			Expect(elementMap["type"]["PublicAlias"]).To(BeFalse(), "PublicAlias should not be private")
			Expect(elementMap["type"]["privateAlias"]).To(BeTrue(), "privateAlias should be private")

			// Test field visibility
			Expect(elementMap["field"]["PublicField"]).To(BeFalse(), "PublicField should not be private")
			Expect(elementMap["field"]["privateField"]).To(BeTrue(), "privateField should be private")
			Expect(elementMap["field"]["XMLTag"]).To(BeFalse(), "XMLTag should not be private (acronym)")
			Expect(elementMap["field"]["httpClient"]).To(BeTrue(), "httpClient should be private")
			Expect(elementMap["field"]["URLPath"]).To(BeFalse(), "URLPath should not be private (acronym)")
			Expect(elementMap["field"]["_hiddenField"]).To(BeTrue(), "_hiddenField should be private")
			Expect(elementMap["field"]["_PublicExport"]).To(BeFalse(), "_PublicExport should not be private (capitalized after underscore)")

			// Test method visibility
			Expect(elementMap["method"]["PublicMethod"]).To(BeFalse(), "PublicMethod should not be private")
			Expect(elementMap["method"]["privateMethod"]).To(BeTrue(), "privateMethod should be private")
			Expect(elementMap["method"]["HTTPHandler"]).To(BeFalse(), "HTTPHandler should not be private")
			Expect(elementMap["method"]["jsonParser"]).To(BeTrue(), "jsonParser should be private")
			Expect(elementMap["method"]["XMLProcessor"]).To(BeFalse(), "XMLProcessor should not be private")
			Expect(elementMap["method"]["urlBuilder"]).To(BeTrue(), "urlBuilder should be private")

			// Test variable visibility
			Expect(elementMap["variable"]["PublicVariable"]).To(BeFalse(), "PublicVariable should not be private")
			Expect(elementMap["variable"]["privateVariable"]).To(BeTrue(), "privateVariable should be private")
			Expect(elementMap["variable"]["HTTPSEnabled"]).To(BeFalse(), "HTTPSEnabled should not be private")
			Expect(elementMap["variable"]["xmlParser"]).To(BeTrue(), "xmlParser should be private")
			Expect(elementMap["variable"]["_global"]).To(BeTrue(), "_global should be private")
			Expect(elementMap["variable"]["_ExportGlobal"]).To(BeFalse(), "_ExportGlobal should not be private")

			// Test constant visibility
			Expect(elementMap["variable"]["PublicConstant"]).To(BeFalse(), "PublicConstant should not be private")
			Expect(elementMap["variable"]["privateConstant"]).To(BeTrue(), "privateConstant should be private")
			Expect(elementMap["variable"]["MaxLimit"]).To(BeFalse(), "MaxLimit should not be private")
			Expect(elementMap["variable"]["defaultTimeout"]).To(BeTrue(), "defaultTimeout should be private")

			// Test edge case functions
			Expect(elementMap["method"]["_privateUnderscore"]).To(BeTrue(), "_privateUnderscore should be private")
			Expect(elementMap["method"]["_PublicUnderscore"]).To(BeFalse(), "_PublicUnderscore should not be private")
		})

		It("should correctly handle special Go functions", func() {
			testFile := filepath.Join("testdata", "visibility.go")
			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, content)
			Expect(err).NotTo(HaveOccurred())

			// Create a map for method names to privacy
			methodPrivacy := make(map[string]bool)
			for _, node := range result.Nodes {
				if node.NodeType == "method" && node.MethodName != "" {
					methodPrivacy[node.MethodName] = node.IsPrivate
				}
			}

			// Special functions that are lowercase but should be treated as public in Go
			// Note: init and main are special cases in Go - they're lowercase but exported behavior
			if privacy, exists := methodPrivacy["init"]; exists {
				Expect(privacy).To(BeTrue(), "init function should be considered private by naming convention")
			}
			if privacy, exists := methodPrivacy["main"]; exists {
				Expect(privacy).To(BeTrue(), "main function should be considered private by naming convention")
			}
		})

		It("should handle methods on both public and private types correctly", func() {
			testFile := filepath.Join("testdata", "visibility.go")
			content, err := os.ReadFile(testFile)
			Expect(err).NotTo(HaveOccurred())

			result, err := extractor.ExtractFile(astCache, testFile, content)
			Expect(err).NotTo(HaveOccurred())

			// Find methods and their receiver types
			methodsOnPublic := make(map[string]bool)
			methodsOnPrivate := make(map[string]bool)

			for _, node := range result.Nodes {
				if node.NodeType == "method" && node.TypeName != "" {
					if node.TypeName == "PublicStruct" || node.TypeName == "HTTPClient" {
						methodsOnPublic[node.MethodName] = node.IsPrivate
					} else if node.TypeName == "privateStruct" || node.TypeName == "xmlDocument" {
						methodsOnPrivate[node.MethodName] = node.IsPrivate
					}
				}
			}

			// Methods on public types
			Expect(methodsOnPublic["PublicMethod"]).To(BeFalse(), "PublicMethod on public type should not be private")
			Expect(methodsOnPublic["privateMethod"]).To(BeTrue(), "privateMethod on public type should be private")
			Expect(methodsOnPublic["HTTPGet"]).To(BeFalse(), "HTTPGet on public type should not be private")

			// Methods on private types - the method visibility is independent of type visibility
			Expect(methodsOnPrivate["PublicMethod"]).To(BeFalse(), "PublicMethod on private type should not be private (method name determines visibility)")
			Expect(methodsOnPrivate["privateMethod"]).To(BeTrue(), "privateMethod on private type should be private")
		})
	})
})
