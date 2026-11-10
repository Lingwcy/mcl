package meta

import "testing"

func TestMavenRepositoryBindsCoordinateAndClassifier(t *testing.T) {
	c, err := ParseCoordinate("net.minecraftforge:forge:1.7.10-10.13.4.1614-1.7.10:installer")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ repository, root string }{
		{"minecraft", "https://libraries.minecraft.net/"}, {"forge", "https://maven.minecraftforge.net/"},
		{"neoforge", "https://maven.neoforged.net/releases/"}, {"fabric", "https://maven.fabricmc.net/"},
		{"quilt-release", "https://maven.quiltmc.org/repository/release/"},
		{"central", "https://repo.maven.apache.org/maven2/"}, {"central-repo1", "https://repo1.maven.org/maven2/"},
	} {
		location, err := BindMavenURL(tc.root+c.Path(), c)
		if err != nil || location.Repository != tc.repository || location.Coordinate != c || location.URL() != tc.root+c.Path() {
			t.Fatal(tc, location, err)
		}
	}
	other := c
	other.Classifier = "universal"
	if _, err := BindMavenURL("https://maven.minecraftforge.net/"+c.Path(), other); err == nil {
		t.Fatal("installer accepted as universal")
	}
	parsed, err := ParseCoordinate(c.String())
	if err != nil || parsed != c {
		t.Fatal(parsed, err)
	}
}
func TestMavenRepositoryRejectsTrustedHostWithWrongRootOrFilename(t *testing.T) {
	for _, address := range []string{
		"https://repo.maven.apache.org/other/org/example/lib/1/lib-1.jar",
		"https://maven.neoforged.net/other/org/example/lib/1/lib-1.jar",
		"https://maven.quiltmc.org/org/example/lib/1/lib-1.jar",
		"https://maven.fabricmc.net/org/example/lib/1/lib-2.jar",
		"https://maven.minecraftforge.net/org/example/lib/1/lib-1.jar?",
		"https://maven.fabricmc.net/org%2fexample/lib/1/lib-1.jar",
		"https://maven.fabricmc.net/../org/example/lib/1/lib-1.jar",
		"https://maven.minecraftforge.net/net/minecraftforge/forge/maven-metadata.xml",
	} {
		if _, err := ParseMavenURL(address); err == nil {
			t.Fatal("unbound Maven location accepted", address)
		}
	}
	for _, root := range []string{"https://maven.fabricmc.net/unlisted/", "https://repo.maven.apache.org/", "https://user:secret@maven.fabricmc.net/"} {
		if _, err := MavenRepositoryRoot(root); err == nil {
			t.Fatal(root)
		}
	}
	if root, err := MavenRepositoryRoot(""); err != nil || root != "https://libraries.minecraft.net/" {
		t.Fatal(root, err)
	}
}
