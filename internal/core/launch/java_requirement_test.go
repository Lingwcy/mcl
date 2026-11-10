package launch

import (
	"mcl/internal/core/java"
	"testing"
)

func TestLaunchPreviewAndBuildKeepFixedJavaUpperBound(t *testing.T) {
	o := options(t, "1.21.1")
	maximum, err := java.ParseVersion("21.0.7")
	if err != nil {
		t.Fatal(err)
	}
	requirement := java.GameRequirement(21, o.Platform)
	requirement.Constraints = append(requirement.Constraints, java.Constraint{Source: "loader-game", Maximum: &java.Bound{Version: maximum, Inclusive: true}})
	o.JavaRequirement = requirement
	if _, err := PreviewOptions(o); err == nil {
		t.Fatal("preview ignored full Java requirement")
	}
	if _, err := Build(o, session(t)); err == nil {
		t.Fatal("final plan ignored full Java requirement")
	}
	maximum, err = java.ParseVersion("21.0.9")
	if err != nil {
		t.Fatal(err)
	}
	o.JavaRequirement.Constraints[len(requirement.Constraints)-1].Maximum.Version = maximum
	if _, err := PreviewOptions(o); err != nil {
		t.Fatal(err)
	}
}
