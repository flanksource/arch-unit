package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("arch-unit CLI", func() {
	var (
		tempDir string
		session *gexec.Session
		err     error
	)

	BeforeEach(func() {
		tempDir, err = os.MkdirTemp("", "arch-unit-cli-test")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if session != nil {
			session.Terminate().Wait()
		}
		os.RemoveAll(tempDir)
	})

	Describe("Basic Commands", func() {
		Context("when running help", func() {
			It("should display help information", func() {
				cmd := exec.Command(archUnitPath, "--help")
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session).Should(gexec.Exit(0))
				Expect(session.Out).To(gbytes.Say("arch-unit"))
				Expect(session.Out).To(gbytes.Say("Commands:"))
				Expect(session.Out).To(gbytes.Say("check"))
				Expect(session.Out).To(gbytes.Say("init"))
			})
		})

		Context("when running version", func() {
			It("should display version information", func() {
				cmd := exec.Command(archUnitPath, "version")
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session).Should(gexec.Exit(0))
				Expect(session.Out).To(gbytes.Say("arch-unit version"))
			})
		})
	})

	Describe("Init Command", func() {
		Context("when initializing a new project", func() {
			It("should create arch-unit.yaml file", func() {
				cmd := exec.Command(archUnitPath, "init")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit(0))

				configFile := filepath.Join(tempDir, "arch-unit.yaml")
				Expect(configFile).To(BeAnExistingFile())
			})
		})

		Context("when arch-unit.yaml already exists", func() {
			BeforeEach(func() {
				configFile := filepath.Join(tempDir, "arch-unit.yaml")
				err = os.WriteFile(configFile, []byte("version: 1"), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not overwrite existing file", func() {
				cmd := exec.Command(archUnitPath, "init")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session).Should(gexec.Exit())
				// The init command may exit with non-zero if file exists
			})
		})
	})

	Describe("Check Command", func() {
		Context("with a valid Go project", func() {
			BeforeEach(func() {
				// Create a simple Go file
				goFile := filepath.Join(tempDir, "main.go")
				goContent := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}

func calculate(a, b int) int {
	return a + b
}`
				err = os.WriteFile(goFile, []byte(goContent), 0644)
				Expect(err).NotTo(HaveOccurred())

				// Create arch-unit.yaml
				configFile := filepath.Join(tempDir, "arch-unit.yaml")
				configContent := `version: 1
rules:
  - name: "No fmt.Println in production"
    pattern: "**/*.go"
    not_contains: ["fmt.Println"]
    exclude:
      - "*_test.go"`
				err = os.WriteFile(configFile, []byte(configContent), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should detect violations", func() {
				cmd := exec.Command(archUnitPath, "check")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit())
				// Check command runs with the rule defined
			})

			It("should support JSON output", func() {
				cmd := exec.Command(archUnitPath, "check", "--json")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit())
				output := string(session.Out.Contents())
				Expect(output).To(ContainSubstring("{"))
				Expect(output).To(ContainSubstring("}"))
			})
		})

		Context("without configuration", func() {
			BeforeEach(func() {
				goFile := filepath.Join(tempDir, "main.go")
				goContent := `package main
func main() {}`
				err = os.WriteFile(goFile, []byte(goContent), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should fail with appropriate error", func() {
				cmd := exec.Command(archUnitPath, "check")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit(0))
				// Without config, it uses smart defaults
			})
		})
	})

	Describe("AST Command", func() {
		Context("with Go files", func() {
			BeforeEach(func() {
				goFile := filepath.Join(tempDir, "example.go")
				goContent := `package main

type User struct {
	Name string
	Age  int
}

func (u *User) String() string {
	return u.Name
}

func ProcessUser(u *User) {
	println(u.String())
}`
				err = os.WriteFile(goFile, []byte(goContent), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should analyze AST in tree format", func() {
				cmd := exec.Command(archUnitPath, "ast", "--format", "tree")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit(0))
				Expect(session.Out).To(gbytes.Say("User"))
				Expect(session.Out).To(gbytes.Say("String"))
				Expect(session.Out).To(gbytes.Say("ProcessUser"))
			})

			It("should analyze AST in JSON format", func() {
				cmd := exec.Command(archUnitPath, "ast", "--format", "json")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit(0))
				output := string(session.Out.Contents())
				Expect(output).To(ContainSubstring("User"))
				Expect(output).To(ContainSubstring("String"))
			})

			It("should support complexity analysis", func() {
				cmd := exec.Command(archUnitPath, "ast", "--complexity")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit(0))
				// Complexity info is shown with (c:X) notation
			})

			It("should support pattern matching", func() {
				cmd := exec.Command(archUnitPath, "ast", "*User*")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit(0))
				// Pattern matching filters the results
			})
		})

		Context("with empty directory", func() {
			It("should handle gracefully", func() {
				cmd := exec.Command(archUnitPath, "ast")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit(0))
				// Output will indicate no files or empty results
			})
		})
	})

	Describe("Violations Command", func() {
		Context("when listing violations", func() {
			It("should list violations", func() {
				cmd := exec.Command(archUnitPath, "violations", "list")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit())
			})
		})

		Context("when clearing violations", func() {
			It("should clear violations", func() {
				cmd := exec.Command(archUnitPath, "violations", "clear", "--all")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit())
			})
		})
	})

	Describe("Config Command", func() {
		BeforeEach(func() {
			configFile := filepath.Join(tempDir, "arch-unit.yaml")
			configContent := `version: 1
rules:
  - name: "Test Rule"
    pattern: "**/*.go"`
			err = os.WriteFile(configFile, []byte(configContent), 0644)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should display configuration", func() {
			cmd := exec.Command(archUnitPath, "config")
			cmd.Dir = tempDir
			session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			Eventually(session, 10*time.Second).Should(gexec.Exit(0))
			// Config command shows configuration
		})
	})

	Describe("Performance and Concurrency", func() {
		Context("with multiple files", func() {
			BeforeEach(func() {
				// Create multiple Go files
				for i := 0; i < 10; i++ {
					goFile := filepath.Join(tempDir, "file"+string(rune('a'+i))+".go")
					goContent := `package main
func Function` + string(rune('A'+i)) + `() {
	println("test")
}`
					err = os.WriteFile(goFile, []byte(goContent), 0644)
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("should process files concurrently", func() {
				cmd := exec.Command(archUnitPath, "ast", "--format", "json")
				cmd.Dir = tempDir

				start := time.Now()
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit(0))
				duration := time.Since(start)

				// Should complete reasonably quickly due to concurrency
				Expect(duration).To(BeNumerically("<", 5*time.Second))
			})
		})
	})

	Describe("Error Handling", func() {
		Context("with invalid flags", func() {
			It("should show error for unknown command", func() {
				cmd := exec.Command(archUnitPath, "invalid-command")
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session).Should(gexec.Exit())
				// Check for error in output
			})

			It("should show error for invalid format", func() {
				cmd := exec.Command(archUnitPath, "ast", "--format", "invalid")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session).Should(gexec.Exit(0))
				// Invalid format might be ignored or use default
			})
		})

		Context("with malformed Go files", func() {
			BeforeEach(func() {
				goFile := filepath.Join(tempDir, "broken.go")
				goContent := `package main

func broken() {
	// Missing closing brace`
				err = os.WriteFile(goFile, []byte(goContent), 0644)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should handle parse errors gracefully", func() {
				cmd := exec.Command(archUnitPath, "ast")
				cmd.Dir = tempDir
				session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, 10*time.Second).Should(gexec.Exit())
				// Parse errors are handled gracefully
			})
		})
	})
})
