package tiers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"
)

// ManifestName is the repository-owned declaration, outside managed config.
const ManifestName = "coverage-tiers.json"

func matches(rule, path string) bool {
	if strings.HasSuffix(rule, "/") {
		return strings.HasPrefix(path, rule)
	}
	return rule == path
}

// Rule classifies an exact source path or a directory ending in a slash.
type Rule struct {
	Path   string `json:"path"`
	Tier   string `json:"tier"`
	Reason string `json:"reason"`
}

// Feature adopts its whole path, regardless of the age of its source files.
type Feature struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	DefaultTier string `json:"defaultTier"`
	Reason      string `json:"reason"`
	Overrides   []Rule `json:"overrides"`
}

// Manifest declares adoption rather than tracking installation progress.
type Manifest struct {
	Version          int       `json:"version"`
	EnforcedPrefixes []string  `json:"enforcedPrefixes"`
	Features         []Feature `json:"features"`
}

func declarationError(format string, args ...any) error {
	return fmt.Errorf(ManifestName+": "+format, args...)
}

func canonical(value string) error {
	trimmed := strings.TrimSuffix(value, "/")
	// path.Clean("") is ".", so the noncanonical-path check also rejects empty paths.
	if trimmed == "." || trimmed == ".." || strings.HasPrefix(trimmed, "../") || strings.HasPrefix(value, "/") || strings.ContainsAny(value, ":\\") || path.Clean(trimmed) != trimmed {
		return declarationError("path %q must be canonical and repository-relative", value)
	}
	return nil
}

func validTier(value string) error {
	switch value {
	case "core", "important", "infrastructure":
		return nil
	default:
		return declarationError("unknown tier %q; use core, important or infrastructure", value)
	}
}

// Parse validates all declarations before any adopted path is traversed.
func Parse(raw []byte) (Manifest, error) {
	var m Manifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return Manifest{}, declarationError("%v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Manifest{}, declarationError("declare exactly one JSON object")
	}
	if m.Version != 1 {
		return Manifest{}, declarationError("version must be 1")
	}
	if len(m.EnforcedPrefixes) == 0 || len(m.Features) == 0 {
		return Manifest{}, declarationError("declare at least one enforced prefix and feature")
	}
	for _, prefix := range m.EnforcedPrefixes {
		if err := canonical(prefix); err != nil {
			return Manifest{}, err
		}
	}
	// The stdlib comparator preserves equal identities for duplicate validation.
	slices.SortFunc(m.Features, func(a, b Feature) int { return strings.Compare(a.ID, b.ID) })
	for i, feature := range m.Features {
		if strings.TrimSpace(feature.ID) == "" || strings.TrimSpace(feature.Reason) == "" {
			return Manifest{}, declarationError("each feature must declare an id and reason")
		}
		if err := canonical(feature.Path); err != nil {
			return Manifest{}, err
		}
		if err := validTier(feature.DefaultTier); err != nil {
			return Manifest{}, err
		}
		inside := false
		for _, prefix := range m.EnforcedPrefixes {
			inside = inside || matches(prefix, feature.Path)
		}
		if !inside {
			return Manifest{}, declarationError("feature %q is outside the enforced prefixes", feature.ID)
		}
		for _, previous := range m.Features[:i] {
			if previous.ID == feature.ID {
				return Manifest{}, declarationError("duplicate feature id %q", feature.ID)
			}
			if matches(previous.Path, feature.Path) || matches(feature.Path, previous.Path) {
				return Manifest{}, declarationError("features %q and %q overlap", previous.ID, feature.ID)
			}
		}
		seen := make(map[string]bool)
		for _, override := range feature.Overrides {
			if err := canonical(override.Path); err != nil {
				return Manifest{}, err
			}
			if !matches(feature.Path, override.Path) {
				return Manifest{}, declarationError("override %q is outside feature %q", override.Path, feature.ID)
			}
			if seen[override.Path] {
				return Manifest{}, declarationError("feature %q repeats override %q", feature.ID, override.Path)
			}
			seen[override.Path] = true
			if err := validTier(override.Tier); err != nil {
				return Manifest{}, err
			}
			if strings.TrimSpace(override.Reason) == "" {
				return Manifest{}, declarationError("override %q must declare a reason", override.Path)
			}
		}
	}
	return m, nil
}
