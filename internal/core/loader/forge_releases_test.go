package loader

import (
	"net/http"
	"testing"
)

func TestForgeCatalogKeepsExplicitGameBranchIdentity(t *testing.T) {
	catalog := NewClient()
	catalog.HTTP = &http.Client{Transport: legacyTestTransport{map[string][]byte{
		catalog.ForgeURL + "/maven-metadata.json":  []byte(`{"1.7.10":["1.7.10-10.13.4.1614-1.7.10","1.7.10-10.13.4.1558","1.7.10-10.13.4.1600-beta","1.7.10-10.13.4.1600-1.6.4"]}`),
		catalog.ForgeURL + "/promotions_slim.json": []byte(`{"promos":{"1.7.10-recommended":"10.13.4.1614"}}`),
	}}}
	rows, err := catalog.Releases(t.Context(), "forge", "1.7.10")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || rows[0].Version != "10.13.4.1614-1.7.10" || !rows[0].Stable || !rows[0].Recommended {
		t.Fatal("explicit catalog branch or recommendation was lost", rows)
	}
	for _, row := range rows {
		if row.Version == "10.13.4.1558" && !row.Stable {
			t.Fatal("numeric release lost stable status")
		}
		if (row.Version == "10.13.4.1600-beta" || row.Version == "10.13.4.1600-1.6.4") && row.Stable {
			t.Fatal("unknown branch was guessed stable")
		}
	}
}
