package changes

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name    string
		message string
		author  string
		want    Kind
	}{
		{"release plugin", "[maven-release-plugin] prepare for next development iteration", "someone", Bookkeeping},
		{"dependabot author", "Something unusual", "dependabot[bot]", Dependency},
		{"conventional deps", "chore(deps): bump foo from 1 to 2", "someone", Dependency},
		{"deps colon", "deps: Bump retrying-again version to 2.1.15 (#1445)", "someone", Dependency},
		{"bump", "Bump foo to 2", "someone", Dependency},
		{"ci", "ci: add a CodeQL workflow", "someone", Maintenance},
		{"build scope", "build(maven): bind a plugin", "someone", Maintenance},
		{"chore", "chore: exclude org.kiwiproject from cooldown", "someone", Maintenance},
		{"docs", "docs: explain a flag", "someone", Maintenance},
		{"breaking marker", "refactor!: rename a type", "someone", Maintenance},
		{"plain sentence", "Add a new thing", "someone", Other},
		{"feature", "feat: add a retry option", "someone", Other},
		{"prefix without separator", "building a thing", "someone", Other},
		{"case insensitive", "CI: add a workflow", "someone", Maintenance},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classify(tt.message, tt.author); got != tt.want {
				t.Errorf("classify(%q, %q) = %q, want %q", tt.message, tt.author, got, tt.want)
			}
		})
	}
}
