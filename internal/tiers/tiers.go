// Package tiers resolves consumer-owned business classifications, not business risk.
package tiers

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Disble/dharness/internal/project"
)

// IntegrationPath is owned coverage configuration, not an installation ledger.
const IntegrationPath = ".dharness/coverage-tiers.mjs"

// File is a canonical repository-relative classification and native target.
type File struct {
	Path     string `json:"path"`
	Tier     string `json:"tier"`
	Coverage int    `json:"coverage"`
}

// Resolution is the shared machine contract for coverage and mutation.
type Resolution struct {
	Version int    `json:"version"`
	Active  bool   `json:"active"`
	Files   []File `json:"files"`
}

// Tier returns the declaration for a resolved file; false preserves legacy behavior.
func (r Resolution) Tier(path string) (string, bool) {
	for _, file := range r.Files {
		if file.Path == path {
			return file.Tier, true
		}
	}
	return "", false
}

func target(tier string) int {
	switch tier {
	case "core":
		return 100
	case "important":
		return 80
	default:
		return 0
	}
}

// Resolve reads the complete adopted source universe and never writes to root.
func Resolve(root string) (Resolution, error) {
	result := Resolution{Version: 1, Files: []File{}}
	raw, err := os.ReadFile(filepath.Join(root, ManifestName))
	if os.IsNotExist(err) {
		_, integrationErr := os.Lstat(filepath.Join(root, filepath.FromSlash(IntegrationPath)))
		if integrationErr == nil {
			return Resolution{}, declarationError("required by the managed coverage integration; restore the manifest")
		}
		if !os.IsNotExist(integrationErr) {
			return Resolution{}, fmt.Errorf("read coverage integration: %w", integrationErr)
		}
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("read %s: %w", ManifestName, err)
	}
	manifest, err := Parse(raw)
	if err != nil {
		return Resolution{}, err
	}
	result.Active = true
	for _, feature := range manifest.Features {
		// For validated canonical paths, a matching child sorts after its parent.
		// Reverse path order lets the first match win without an artificial length baseline.
		slices.SortFunc(feature.Overrides, func(a, b Rule) int { return strings.Compare(b.Path, a.Path) })
	}
	seen := make(map[string]bool)
	for _, prefix := range manifest.EnforcedPrefixes {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(prefix)), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				switch entry.Name() {
				case "node_modules", ".git", ".dharness", "__tests__":
					return filepath.SkipDir
				}
				return nil
			}
			name := strings.ToLower(entry.Name())
			if !project.IsSourceFile(path) || strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if seen[rel] {
				return nil
			}
			for _, feature := range manifest.Features {
				if !matches(feature.Path, rel) {
					continue
				}
				tier := feature.DefaultTier
				for _, override := range feature.Overrides {
					if matches(override.Path, rel) {
						tier = override.Tier
						break
					}
				}
				result.Files = append(result.Files, File{Path: rel, Tier: tier, Coverage: target(tier)})
				seen[rel] = true
				return nil
			}
			return fmt.Errorf("%s: source %s has no declared feature; declare it or narrow the adopted prefixes", ManifestName, rel)
		})
		if err != nil {
			return Resolution{}, err
		}
	}
	slices.SortFunc(result.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return result, nil
}
