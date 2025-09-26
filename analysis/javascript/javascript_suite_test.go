package javascript

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestJavaScript(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "JavaScript AST Extractor Suite")
}
