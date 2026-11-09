package app

import (
	"net/url"
	"path/filepath"
	"testing"
)

func TestImportLinkOnlyAllowsAnExplicitLocalModpack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "中文 文件.zip")
	got, err := ImportLinkPath("mcl://import-modpack?path=" + url.QueryEscape(path))
	if err != nil || got != path {
		t.Fatal("local import path did not survive URL decoding", err)
	}
	for _, address := range []string{"sjmcl://import-modpack?path=x.zip", "mcl://import-modpack?path=relative.zip", "mcl://import-modpack?url=https%3A%2F%2Fexample.com%2Fpack.zip", "mcl://launch?path=x.zip", "mcl://import-modpack?path=a.zip&path=b.zip", "mcl://import-modpack?path=x.zip&execute=1", "mcl://user@import-modpack?path=x.zip", "mcl://import-modpack?path=%00.zip", "mcl://import-modpack?path=%zz"} {
		if _, err := ImportLinkPath(address); err == nil {
			t.Fatalf("accepted foreign/remote/ambiguous intent %q", address)
		}
	}
}
