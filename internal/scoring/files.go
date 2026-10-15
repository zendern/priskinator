package scoring

import (
	"path"
	"slices"
	"strings"
)

// sourceExts are the extensions the test signal counts as source code.
var sourceExts = []string{
	".go", ".py", ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".java", ".kt", ".kts",
	".scala", ".rb", ".rs", ".c", ".h", ".cc", ".cpp", ".hpp", ".cs", ".swift", ".php", ".sh",
}

// testDirs are directory names whose contents are tests.
var testDirs = []string{"test", "tests", "__tests__", "spec"}

// testSuffixes and testInfixes are test file name conventions per language.
var (
	testSuffixes = []string{
		"_test.go", "_test.py", "_spec.rb", "_test.rb",
		"Test.java", "Tests.java", "Test.kt", "Tests.kt", "Test.cs", "Tests.cs",
	}
	testInfixes = []string{".test.", ".spec."}
)

func isSource(p string) bool {
	return slices.Contains(sourceExts, strings.ToLower(path.Ext(p)))
}

func isTest(p string) bool {
	dir, name := path.Split(p)
	for _, d := range strings.Split(strings.TrimSuffix(dir, "/"), "/") {
		if slices.Contains(testDirs, d) {
			return true
		}
	}
	if name == "conftest.py" || (strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py")) {
		return true
	}
	for _, s := range testSuffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	for _, s := range testInfixes {
		if strings.Contains(name, s) {
			return true
		}
	}
	return false
}
