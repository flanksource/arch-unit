package analysis_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/flanksource/arch-unit/analysis"
)

var _ = Describe("Reanalysis Detector", func() {
	var (
		detector *analysis.ReanalysisDetector
		tempDir  string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "reanalysis-test")
		Expect(err).NotTo(HaveOccurred())
		detector = analysis.NewReanalysisDetector()
	})

	AfterEach(func() {
		err := os.RemoveAll(tempDir)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("when checking file changes", func() {
		It("should detect when file needs initial analysis", func() {
			testFile := filepath.Join(tempDir, "test.sql")
			err := os.WriteFile(testFile, []byte("CREATE TABLE test (id INTEGER);"), 0644)
			Expect(err).NotTo(HaveOccurred())

			needsReanalysis, err := detector.NeedsReanalysis(testFile, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis).To(BeTrue())
		})

		It("should detect when file has not changed", func() {
			testFile := filepath.Join(tempDir, "test.sql")
			content := []byte("CREATE TABLE test (id INTEGER);")
			err := os.WriteFile(testFile, content, 0644)
			Expect(err).NotTo(HaveOccurred())

			// First analysis - should need reanalysis
			needsReanalysis, err := detector.NeedsReanalysis(testFile, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis).To(BeTrue())

			// Record the analysis
			err = detector.RecordAnalysis(testFile, "")
			Expect(err).NotTo(HaveOccurred())

			// Second check - should not need reanalysis
			needsReanalysis, err = detector.NeedsReanalysis(testFile, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis).To(BeFalse())
		})

		It("should detect when file content has changed", func() {
			testFile := filepath.Join(tempDir, "test.sql")

			// Write initial content
			initialContent := []byte("CREATE TABLE test (id INTEGER);")
			err := os.WriteFile(testFile, initialContent, 0644)
			Expect(err).NotTo(HaveOccurred())

			// Record initial analysis
			err = detector.RecordAnalysis(testFile, "")
			Expect(err).NotTo(HaveOccurred())

			// Sleep to ensure different modification time
			time.Sleep(10 * time.Millisecond)

			// Change content
			modifiedContent := []byte("CREATE TABLE test (id INTEGER, name VARCHAR(100));")
			err = os.WriteFile(testFile, modifiedContent, 0644)
			Expect(err).NotTo(HaveOccurred())

			// Should need reanalysis
			needsReanalysis, err := detector.NeedsReanalysis(testFile, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis).To(BeTrue())
		})

		It("should handle non-existent files", func() {
			nonExistentFile := filepath.Join(tempDir, "nonexistent.sql")

			needsReanalysis, err := detector.NeedsReanalysis(nonExistentFile, "")
			Expect(err).To(HaveOccurred())
			Expect(needsReanalysis).To(BeFalse())
		})
	})

	Context("when checking URL changes", func() {
		var server *httptest.Server

		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Last-Modified", time.Now().Format(http.TimeFormat))
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"openapi": "3.0.0", "info": {"title": "Test API", "version": "1.0.0"}}`))
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("should detect when URL needs initial analysis", func() {
			needsReanalysis, err := detector.NeedsReanalysis("", server.URL)
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis).To(BeTrue())
		})

		It("should detect when URL content has not changed", func() {
			// First check - should need reanalysis
			needsReanalysis, err := detector.NeedsReanalysis("", server.URL)
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis).To(BeTrue())

			// Record analysis
			err = detector.RecordAnalysis("", server.URL)
			Expect(err).NotTo(HaveOccurred())

			// Second check - should not need reanalysis (same content)
			needsReanalysis, err = detector.NeedsReanalysis("", server.URL)
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis).To(BeFalse())
		})

		It("should handle URL fetch errors", func() {
			invalidURL := "http://nonexistent.example.com/api"

			needsReanalysis, err := detector.NeedsReanalysis("", invalidURL)
			Expect(err).To(HaveOccurred())
			Expect(needsReanalysis).To(BeTrue()) // When URL fetch fails, assume it needs reanalysis
		})
	})

	Context("when checking SQL connection changes", func() {
		It("should detect when connection needs initial analysis", func() {
			connectionString := "postgres://user@localhost/testdb"

			needsReanalysis, err := detector.NeedsReanalysisForConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis).To(BeTrue())
		})

		It("should detect when connection has not changed", func() {
			connectionString := "postgres://user@localhost/testdb"

			// First check
			needsReanalysis, err := detector.NeedsReanalysisForConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis).To(BeTrue())

			// Record analysis with mock schema hash
			err = detector.RecordConnectionAnalysis(connectionString, "mock_schema_hash")
			Expect(err).NotTo(HaveOccurred())

			// For testing purposes, since we just recorded the analysis,
			// it should not need reanalysis (within the 1-hour window)
			needsReanalysis, err = detector.NeedsReanalysisForConnection(connectionString)
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis).To(BeFalse()) // Should be false since less than 1 hour has passed
		})
	})

	Context("when handling hash calculations", func() {
		It("should generate consistent hashes for same content", func() {
			content1 := []byte("CREATE TABLE test (id INTEGER);")
			content2 := []byte("CREATE TABLE test (id INTEGER);")

			hash1 := detector.CalculateContentHash(content1)
			hash2 := detector.CalculateContentHash(content2)

			Expect(hash1).To(Equal(hash2))
			Expect(hash1).NotTo(BeEmpty())
		})

		It("should generate different hashes for different content", func() {
			content1 := []byte("CREATE TABLE test1 (id INTEGER);")
			content2 := []byte("CREATE TABLE test2 (id INTEGER);")

			hash1 := detector.CalculateContentHash(content1)
			hash2 := detector.CalculateContentHash(content2)

			Expect(hash1).NotTo(Equal(hash2))
		})

		It("should handle empty content", func() {
			emptyContent := []byte("")
			hash := detector.CalculateContentHash(emptyContent)
			Expect(hash).NotTo(BeEmpty())
		})
	})

	Context("when managing analysis records", func() {
		It("should clear records for specific path", func() {
			testFile := filepath.Join(tempDir, "test.sql")
			err := os.WriteFile(testFile, []byte("CREATE TABLE test (id INTEGER);"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Record analysis
			err = detector.RecordAnalysis(testFile, "")
			Expect(err).NotTo(HaveOccurred())

			// Verify it's recorded
			needsReanalysis, err := detector.NeedsReanalysis(testFile, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis).To(BeFalse())

			// Clear record
			err = detector.ClearAnalysisRecord(testFile, "")
			Expect(err).NotTo(HaveOccurred())

			// Should need reanalysis again
			needsReanalysis, err = detector.NeedsReanalysis(testFile, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis).To(BeTrue())
		})

		It("should clear all records", func() {
			// Create multiple test files
			testFile1 := filepath.Join(tempDir, "test1.sql")
			testFile2 := filepath.Join(tempDir, "test2.sql")

			err := os.WriteFile(testFile1, []byte("CREATE TABLE test1 (id INTEGER);"), 0644)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(testFile2, []byte("CREATE TABLE test2 (id INTEGER);"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Record analyses
			err = detector.RecordAnalysis(testFile1, "")
			Expect(err).NotTo(HaveOccurred())
			err = detector.RecordAnalysis(testFile2, "")
			Expect(err).NotTo(HaveOccurred())

			// Clear all records
			err = detector.ClearAllRecords()
			Expect(err).NotTo(HaveOccurred())

			// Both should need reanalysis
			needsReanalysis1, err := detector.NeedsReanalysis(testFile1, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis1).To(BeTrue())

			needsReanalysis2, err := detector.NeedsReanalysis(testFile2, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(needsReanalysis2).To(BeTrue())
		})
	})
})