package mcp

import "testing"

func TestAuditedSubscriptionURIScopeBoundaries(t *testing.T) {
	// Arrange. Routing boundaries are stricter than general URI resolution.
	cases := []struct {
		name, base, update string
		want               bool
	}{
		{"encoded separator sibling", "https://e.test/root", "https://e.test/root%2Fchild", false},
		{"encoded separator data remains equal", "https://e.test/root%2fpart", "https://e.test/root%2Fpart", true},
		{"literal separator not equal data", "https://e.test/root%2Fpart", "https://e.test/root/part", false},
		{"encoded traversal", "https://e.test/root", "https://e.test/root/%2e%2e/outside", false},
		{"encoded traversal slash", "https://e.test/root", "https://e.test/root/..%2Foutside", false},
		{"userinfo change", "https://u@e.test/root", "https://v@e.test/root/child", false},
		{"password change", "https://u:p@e.test/root", "https://u:q@e.test/root/child", false},
		{"origin change", "https://e.test/root", "https://else.test/root/child", false},
		{"scheme change", "https://e.test/root", "http://e.test/root/child", false},
		{"nondefault port change", "https://e.test:444/root", "https://e.test/root/child", false},
		{"default port", "HTTPS://E.test:443/root", "https://e.test/root/child", true},
		{"query child", "https://e.test/root?q=1", "https://e.test/root/child?q=1", false},
		{"empty query child", "https://e.test/root?", "https://e.test/root/child", false},
		{"fragment child", "https://e.test/root#part", "https://e.test/root/child", false},
		{"empty fragment child", "https://e.test/root#", "https://e.test/root/child", false},
		{"candidate empty fragment", "https://e.test/root", "https://e.test/root/child#", false},
		{"empty fragment exact", "https://e.test/root#", "https://e.test/root#", true},
		{"empty fragment not absent", "https://e.test/root#", "https://e.test/root", false},
		{"absent fragment not empty", "https://e.test/root", "https://e.test/root#", false},
		{"normalized empty fragment identity", "HTTPS://E.test:443/%72oot#", "https://e.test/root#", true},
		{"encoded hash path is not fragment", "https://e.test/root%23part", "https://e.test/root%23part/child", true},
		{"fragment same exact", "https://e.test/root#part", "https://e.test/root#part", true},
		{"fragment identity differs", "https://e.test/root#part", "https://e.test/root#other", false},
		{"repeated slash distinct", "https://e.test/root/private", "https://e.test/root//private", false},
		{"invalid escape", "https://e.test/root", "https://e.test/root/%zz", false},
		{"malformed relative", ":root", ":root/child", false},
		{"opaque exact", "urn:tool:one", "urn:tool:one", true},
		{"opaque prefix", "urn:tool:one", "urn:tool:one/child", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Act.
			got := resourceSubscriptionAllows([]string{tc.base}, tc.update)
			// Assert.
			if got != tc.want {
				t.Fatalf("subscription %q update %q: got %v, want %v", tc.base, tc.update, got, tc.want)
			}
		})
	}
}
