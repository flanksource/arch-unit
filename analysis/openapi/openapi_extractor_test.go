package openapi_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/analysis/openapi"
	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/models"
)

func TestOpenAPI(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OpenAPI Suite")
}

var _ = Describe("OpenAPI Extractor", func() {
	var (
		extractor *openapi.OpenAPIExtractor
		astCache  *cache.ASTCache
	)

	BeforeEach(func() {
		extractor = openapi.NewOpenAPIExtractor()
		astCache = cache.MustGetASTCache()
	})

	Context("when extracting from valid OpenAPI JSON", func() {
		It("should extract schemas and endpoints", func() {
			openAPIJSON := `{
  "openapi": "3.0.0",
  "info": {
    "title": "Pet Store API",
    "version": "1.0.0"
  },
  "paths": {
    "/pets": {
      "get": {
        "operationId": "listPets",
        "summary": "List all pets",
        "parameters": [
          {
            "name": "limit",
            "in": "query",
            "description": "How many items to return",
            "required": false,
            "schema": {
              "type": "integer"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "A list of pets",
            "content": {
              "application/json": {
                "schema": {
                  "type": "array",
                  "items": {
                    "$ref": "#/components/schemas/Pet"
                  }
                }
              }
            }
          }
        }
      },
      "post": {
        "operationId": "createPet",
        "summary": "Create a pet",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "$ref": "#/components/schemas/Pet"
              }
            }
          }
        },
        "responses": {
          "201": {
            "description": "Pet created"
          }
        }
      }
    },
    "/pets/{petId}": {
      "get": {
        "operationId": "showPetById",
        "summary": "Info for a specific pet",
        "parameters": [
          {
            "name": "petId",
            "in": "path",
            "required": true,
            "description": "The id of the pet to retrieve",
            "schema": {
              "type": "string"
            }
          }
        ],
        "responses": {
          "200": {
            "description": "Expected response to a valid request"
          }
        }
      }
    }
  },
  "components": {
    "schemas": {
      "Pet": {
        "type": "object",
        "required": ["id", "name"],
        "properties": {
          "id": {
            "type": "integer",
            "description": "Unique identifier"
          },
          "name": {
            "type": "string",
            "description": "Pet name"
          },
          "tag": {
            "type": "string",
            "description": "Pet tag"
          }
        }
      }
    }
  }
}`

			result, err := extractor.ExtractFile(astCache, "api/openapi.json", []byte(openAPIJSON))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			// Check that we have the expected nodes
			var schemaNodes []*models.ASTNode
			var methodNodes []*models.ASTNode
			var fieldNodes []*models.ASTNode

			for _, node := range result.Nodes {
				switch node.NodeType {
				case models.NodeTypeTypeHTTPSchema:
					schemaNodes = append(schemaNodes, node)
				case models.NodeTypeMethodHTTPGet, models.NodeTypeMethodHTTPPost:
					methodNodes = append(methodNodes, node)
				case models.NodeTypeField:
					fieldNodes = append(fieldNodes, node)
				}
			}

			// Should have Pet schema
			Expect(schemaNodes).To(HaveLen(1))
			petSchema := schemaNodes[0]
			Expect(petSchema.TypeName).To(Equal("Pet"))
			Expect(petSchema.PackageName).To(Equal("v1_0_0"))

			// Should have 3 HTTP methods (GET /pets, POST /pets, GET /pets/{petId})
			Expect(methodNodes).To(HaveLen(3))

			// Check specific methods
			var listPets, createPet, showPet *models.ASTNode
			for _, method := range methodNodes {
				switch method.MethodName {
				case "listPets":
					listPets = method
				case "createPet":
					createPet = method
				case "showPetById":
					showPet = method
				}
			}

			Expect(listPets).NotTo(BeNil())
			Expect(listPets.NodeType).To(Equal(models.NodeTypeMethodHTTPGet))
			Expect(listPets.Parameters).To(HaveLen(1))
			Expect(listPets.Parameters[0].Name).To(Equal("limit"))
			Expect(listPets.Parameters[0].Type).To(Equal("integer"))

			Expect(createPet).NotTo(BeNil())
			Expect(createPet.NodeType).To(Equal(models.NodeTypeMethodHTTPPost))
			Expect(createPet.Parameters).To(HaveLen(1))
			Expect(createPet.Parameters[0].Name).To(Equal("body"))

			Expect(showPet).NotTo(BeNil())
			Expect(showPet.NodeType).To(Equal(models.NodeTypeMethodHTTPGet))
			Expect(showPet.Parameters).To(HaveLen(1))
			Expect(showPet.Parameters[0].Name).To(Equal("petId"))
			Expect(showPet.Parameters[0].Type).To(Equal("string"))

			// Should have Pet schema fields
			Expect(fieldNodes).To(HaveLen(3))
			fieldNames := make([]string, len(fieldNodes))
			for i, field := range fieldNodes {
				fieldNames[i] = field.FieldName
				Expect(field.TypeName).To(Equal("Pet"))
			}
			Expect(fieldNames).To(ConsistOf("id", "name", "tag"))
		})
	})

	Context("when extracting from valid OpenAPI YAML", func() {
		It("should extract basic structure", func() {
			openAPIYAML := `openapi: 3.0.0
info:
  title: Simple API
  version: 2.1.0
paths:
  /health:
    get:
      operationId: healthCheck
      summary: Health check endpoint
      responses:
        '200':
          description: Service is healthy
components:
  schemas:
    HealthStatus:
      type: object
      properties:
        status:
          type: string
          description: Current health status
        timestamp:
          type: integer
          description: Unix timestamp
`

			result, err := extractor.ExtractFile(astCache, "api/health.yaml", []byte(openAPIYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Nodes).NotTo(BeEmpty())

			// Check namespace from version
			hasV2_1_0Namespace := false
			for _, node := range result.Nodes {
				if node.PackageName == "v2_1_0" {
					hasV2_1_0Namespace = true
					break
				}
			}
			Expect(hasV2_1_0Namespace).To(BeTrue())

			// Check that health endpoint exists
			hasHealthEndpoint := false
			for _, node := range result.Nodes {
				if node.MethodName == "healthCheck" && node.NodeType == models.NodeTypeMethodHTTPGet {
					hasHealthEndpoint = true
					break
				}
			}
			Expect(hasHealthEndpoint).To(BeTrue())

			// Check that HealthStatus schema exists
			hasHealthSchema := false
			for _, node := range result.Nodes {
				if node.TypeName == "HealthStatus" && node.NodeType == models.NodeTypeTypeHTTPSchema {
					hasHealthSchema = true
					break
				}
			}
			Expect(hasHealthSchema).To(BeTrue())
		})
	})

	Context("when handling invalid inputs", func() {
		It("should return error for invalid JSON", func() {
			invalidJSON := `{
  "openapi": "3.0.0",
  "info": {
    "title": "Broken API"
    // missing comma
    "version": "1.0.0"
  }
}`

			result, err := extractor.ExtractFile(astCache, "api/broken.json", []byte(invalidJSON))
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should return error for invalid YAML", func() {
			invalidYAML := `openapi: 3.0.0
info:
  title: Broken API
  version: 1.0.0
paths:
  /test:
    get:
      responses:
        200:
          description: [invalid yaml structure
`

			result, err := extractor.ExtractFile(astCache, "api/broken.yaml", []byte(invalidYAML))
			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should handle empty content", func() {
			result, err := extractor.ExtractFile(astCache, "api/empty.json", []byte(""))
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			// Empty content results in empty spec with no nodes
			Expect(result.Nodes).To(BeEmpty())
		})
	})

	Context("when validating OpenAPI specs", func() {
		It("should validate required fields", func() {
			spec := &openapi.OpenAPISpec{
				OpenAPI: "3.0.0",
				Info: openapi.Info{
					Title:   "Test API",
					Version: "1.0.0",
				},
				Paths: map[string]openapi.PathItem{
					"/test": {},
				},
			}

			err := extractor.ValidateSpec(spec)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject spec without openapi version", func() {
			spec := &openapi.OpenAPISpec{
				Info: openapi.Info{
					Title:   "Test API",
					Version: "1.0.0",
				},
				Paths: map[string]openapi.PathItem{
					"/test": {},
				},
			}

			err := extractor.ValidateSpec(spec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("openapi version"))
		})

		It("should reject spec without title", func() {
			spec := &openapi.OpenAPISpec{
				OpenAPI: "3.0.0",
				Info: openapi.Info{
					Version: "1.0.0",
				},
				Paths: map[string]openapi.PathItem{
					"/test": {},
				},
			}

			err := extractor.ValidateSpec(spec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("title"))
		})

		It("should reject spec without paths", func() {
			spec := &openapi.OpenAPISpec{
				OpenAPI: "3.0.0",
				Info: openapi.Info{
					Title:   "Test API",
					Version: "1.0.0",
				},
				Paths: map[string]openapi.PathItem{},
			}

			err := extractor.ValidateSpec(spec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("paths"))
		})
	})

	Context("when checking version support", func() {
		It("should support standard OpenAPI versions", func() {
			Expect(extractor.IsVersionSupported("3.0.0")).To(BeTrue())
			Expect(extractor.IsVersionSupported("3.0.1")).To(BeTrue())
			Expect(extractor.IsVersionSupported("3.0.2")).To(BeTrue())
			Expect(extractor.IsVersionSupported("3.0.3")).To(BeTrue())
			Expect(extractor.IsVersionSupported("3.1.0")).To(BeTrue())
		})

		It("should reject unsupported versions", func() {
			Expect(extractor.IsVersionSupported("2.0")).To(BeFalse())
			Expect(extractor.IsVersionSupported("4.0.0")).To(BeFalse())
			Expect(extractor.IsVersionSupported("invalid")).To(BeFalse())
		})

		It("should return list of supported versions", func() {
			versions := extractor.GetSupportedVersions()
			Expect(versions).To(ContainElement("3.0.0"))
			Expect(versions).To(ContainElement("3.1.0"))
			Expect(len(versions)).To(BeNumerically(">", 0))
		})
	})
})