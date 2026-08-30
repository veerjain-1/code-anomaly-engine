package parser

import (
	"strings"
	"testing"
)

func TestExtractSnippets_SimpleDiff(t *testing.T) {
	diff := `diff --git a/main.c b/main.c
--- a/main.c
+++ b/main.c
@@ -1,5 +1,10 @@
 #include <stdio.h>
+#include <string.h>
 
-int main() {
-    printf("hello\n");
+int main() {
+    char buf[10];
+    gets(buf);
+    printf("%s\n", buf);
     return 0;
 }
`

	snippets := ExtractSnippets(diff, "test/repo", "abc123def")

	if len(snippets) == 0 {
		t.Fatal("Expected at least one snippet, got 0")
	}

	// Check that the snippet contains the vulnerable code
	found := false
	for _, s := range snippets {
		if strings.Contains(s.Code, "gets(buf)") {
			found = true
			if s.Repo != "test/repo" {
				t.Errorf("Expected repo 'test/repo', got '%s'", s.Repo)
			}
			if s.CommitSHA != "abc123def" {
				t.Errorf("Expected commit SHA 'abc123def', got '%s'", s.CommitSHA)
			}
			if s.FilePath != "main.c" {
				t.Errorf("Expected file path 'main.c', got '%s'", s.FilePath)
			}
		}
	}
	if !found {
		t.Error("Expected snippet containing 'gets(buf)' not found")
	}
}

func TestExtractSnippets_MultipleFunctions(t *testing.T) {
	diff := `diff --git a/lib.c b/lib.c
--- /dev/null
+++ b/lib.c
@@ -0,0 +1,15 @@
+int add(int a, int b) {
+    return a + b;
+}
+
+void unsafe_copy(char *dst, char *src) {
+    strcpy(dst, src);
+}
+
+int multiply(int a, int b) {
+    return a * b;
+}
`

	snippets := ExtractSnippets(diff, "test/repo", "def456abc")

	if len(snippets) < 2 {
		t.Fatalf("Expected at least 2 function snippets, got %d", len(snippets))
	}

	// Verify we got distinct functions
	hasAdd := false
	hasUnsafe := false
	for _, s := range snippets {
		if strings.Contains(s.Code, "add(int a, int b)") {
			hasAdd = true
		}
		if strings.Contains(s.Code, "unsafe_copy") {
			hasUnsafe = true
		}
	}

	if !hasAdd {
		t.Error("Expected to find 'add' function snippet")
	}
	if !hasUnsafe {
		t.Error("Expected to find 'unsafe_copy' function snippet")
	}
}

func TestExtractSnippets_EmptyDiff(t *testing.T) {
	snippets := ExtractSnippets("", "test/repo", "abc123")
	if snippets != nil {
		t.Errorf("Expected nil for empty diff, got %v", snippets)
	}
}

func TestIsCodeFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"main.c", true},
		{"lib.go", true},
		{"app.py", true},
		{"index.tsx", true},
		{"Main.java", true},
		{"engine.rs", true},
		{"README.md", false},
		{"config.yaml", false},
		{".gitignore", false},
		{"image.png", false},
	}

	for _, tt := range tests {
		// isCodeFile is in the webhook package; test the extension logic here
		got := hasCodeExtension(tt.path)
		if got != tt.want {
			t.Errorf("hasCodeExtension(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// hasCodeExtension mirrors the logic from the webhook package for testing.
func hasCodeExtension(path string) bool {
	codeExtensions := []string{
		".c", ".cpp", ".cc", ".h", ".hpp",
		".go", ".rs",
		".py",
		".js", ".ts", ".jsx", ".tsx",
		".java",
	}
	lower := strings.ToLower(path)
	for _, ext := range codeExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func TestFormatSnippetID(t *testing.T) {
	id := FormatSnippetID("user/repo", "abcdef1234567890", "main.c", 42)
	expected := "user/repo/abcdef12/main.c#L42"
	if id != expected {
		t.Errorf("FormatSnippetID() = %q, want %q", id, expected)
	}
}
