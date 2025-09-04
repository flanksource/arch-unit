package database_test_suite

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var testDB *TestDB

func TestDatabase(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Database Suite")
}

var _ = BeforeSuite(func() {
	var err error
	testDB, err = NewTestDB()
	Expect(err).ToNot(HaveOccurred())

	DeferCleanup(func() {
		if testDB != nil {
			testDB.Close()
		}
	})
})

var _ = BeforeEach(func() {
	// Clear all data between tests for isolation
	err := testDB.ClearAllData()
	Expect(err).ToNot(HaveOccurred())
})
