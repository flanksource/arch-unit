package database_test_suite

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/internal/cache"
)

var _ = Describe("Singleton Cache", func() {
	Context("AST Cache Singleton", func() {
		BeforeEach(func() {
			cache.ResetASTCache()
		})

		AfterEach(func() {
			cache.ResetASTCache()
		})

		It("should create instance on first call", func() {
			cache1, err := cache.GetASTCache()
			Expect(err).ToNot(HaveOccurred())
			Expect(cache1).ToNot(BeNil())
		})

		It("should return same instance on subsequent calls", func() {
			cache1, err := cache.GetASTCache()
			Expect(err).ToNot(HaveOccurred())
			Expect(cache1).ToNot(BeNil())

			cache2, err := cache.GetASTCache()
			Expect(err).ToNot(HaveOccurred())
			Expect(cache1).To(BeIdenticalTo(cache2), "Expected same instance from singleton")
		})
	})

	Context("Violation Cache Singleton", func() {
		BeforeEach(func() {
			cache.ResetViolationCache()
		})

		AfterEach(func() {
			cache.ResetViolationCache()
		})

		It("should create instance on first call", func() {
			cache1, err := cache.GetViolationCache()
			Expect(err).ToNot(HaveOccurred())
			Expect(cache1).ToNot(BeNil())
		})

		It("should return same instance on subsequent calls", func() {
			cache1, err := cache.GetViolationCache()
			Expect(err).ToNot(HaveOccurred())
			Expect(cache1).ToNot(BeNil())

			cache2, err := cache.GetViolationCache()
			Expect(err).ToNot(HaveOccurred())
			Expect(cache1).To(BeIdenticalTo(cache2), "Expected same instance from singleton")
		})
	})
})
