package core

import (
	"fmt"

	"github.com/gobwas/glob"
)

// Filterer applies filters.include/filters.exclude glob patterns to
// repository names, equivalent to plan.ts's use of minimatch. A name passes
// the filter if it matches at least one include pattern and no exclude
// pattern. Patterns are compiled once by NewFilterer and reused across
// Match calls.
type Filterer struct {
	include []glob.Glob
	exclude []glob.Glob
}

// NewFilterer compiles the include and exclude glob patterns.
func NewFilterer(include, exclude []string) (*Filterer, error) {
	includeGlobs, err := compileGlobs(include)
	if err != nil {
		return nil, err
	}
	excludeGlobs, err := compileGlobs(exclude)
	if err != nil {
		return nil, err
	}
	return &Filterer{include: includeGlobs, exclude: excludeGlobs}, nil
}

func compileGlobs(patterns []string) ([]glob.Glob, error) {
	globs := make([]glob.Glob, len(patterns))
	for i, pattern := range patterns {
		g, err := glob.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
		globs[i] = g
	}
	return globs, nil
}

// Match reports whether name should be included: it must match at least one
// include pattern and must not match any exclude pattern.
func (f *Filterer) Match(name string) bool {
	included := false
	for _, g := range f.include {
		if g.Match(name) {
			included = true
			break
		}
	}
	if !included {
		return false
	}

	for _, g := range f.exclude {
		if g.Match(name) {
			return false
		}
	}
	return true
}
