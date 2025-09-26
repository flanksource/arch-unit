package markdown

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMarkdownAnalysis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Markdown Analysis Suite")
}
