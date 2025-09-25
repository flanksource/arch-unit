package database_test_suite

import (
	"testing"

	"github.com/flanksource/arch-unit/internal/cache"
	"github.com/flanksource/arch-unit/languages"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var testDB *TestDB

func TestDatabase(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Database Suite")
}

var _ = BeforeSuite(func() {
	// Reset all global singletons before starting tests
	cache.ResetGormDB()
	cache.ResetASTCache()
	languages.ResetGenericAnalyzer()
	
	var err error
	testDB, err = NewTestDB()
	Expect(err).ToNot(HaveOccurred())

	DeferCleanup(func() {
		if testDB != nil {
			_ = testDB.Close()
		}
		// Reset singletons after tests
		cache.ResetGormDB()
		cache.ResetASTCache()
		languages.ResetGenericAnalyzer()
	})
})

var _ = BeforeEach(func() {
	// Clear all data between tests for isolation
	err := testDB.ClearAllData()
	Expect(err).ToNot(HaveOccurred())
})
