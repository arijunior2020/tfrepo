package cli

import "fmt"

// countLabel formats count together with the Portuguese singular or plural
// form of a noun, e.g. countLabel(1, "repositório", "repositórios") returns
// "1 repositório" and countLabel(2, "repositório", "repositórios") returns
// "2 repositórios".
func countLabel(count int, singular, plural string) string {
	word := plural
	if count == 1 {
		word = singular
	}
	return fmt.Sprintf("%d %s", count, word)
}
