package market

// DependencyMatches compares identities within one catalog. It never guesses
// cross-provider identity from a name; a pinned file/version must also match.
func DependencyMatches(source Source, dependency Dependency, target InstallFile) bool {
	if source != target.Source {
		return false
	}
	if dependency.ProjectID == "" && dependency.VersionID == "" {
		return false
	}
	if dependency.ProjectID != "" && dependency.ProjectID != target.ProjectID {
		return false
	}
	return dependency.VersionID == "" || dependency.VersionID == target.VersionID
}

func Incompatible(left, right InstallFile) bool {
	if left.Metadata != nil {
		for _, dependency := range left.Metadata.Dependencies {
			if dependency.Incompatible && DependencyMatches(left.Source, dependency, right) {
				return true
			}
		}
	}
	if right.Metadata != nil {
		for _, dependency := range right.Metadata.Dependencies {
			if dependency.Incompatible && DependencyMatches(right.Source, dependency, left) {
				return true
			}
		}
	}
	return false
}
