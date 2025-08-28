package dependencies

func init() {
	// Initialize all scanners
	// They will auto-register in their init() functions
	NewGoDependencyScanner()
	NewPythonDependencyScanner()
	NewNodeDependencyScanner()
	NewDockerDependencyScanner()
	NewHelmDependencyScanner()
}
