use serde::Deserialize;
/*

  "libraries": [
    {
      "downloads": {
        "artifact": {
          "path": "ca/weblite/java-objc-bridge/1.1/java-objc-bridge-1.1.jar",
          "sha1": "1227f9e0666314f9de41477e3ec277e542ed7f7b",
          "size": 1330045,
          "url": "https://libraries.minecraft.net/ca/weblite/java-objc-bridge/1.1/java-objc-bridge-1.1.jar"
        }
      },
      "name": "ca.weblite:java-objc-bridge:1.1",
      "rules": [
        {
          "action": "allow",
          "os": {
            "name": "osx"
          }
        }
      ]


*/
#[derive(Deserialize)]
// 映射源 version_manifest/uri(Version)/libraries[Library_index]
pub struct Library {
    pub downloads: Download,
    pub name: String,
    pub rules: Option<Vec<Rule>>,
}

#[derive(Deserialize)]
pub struct Rule {
    pub action: String,
    pub os: Os,
}

#[derive(Deserialize)]
pub struct Os {
    pub name: String,
}

#[derive(Deserialize)]
pub struct Download {
    pub artifact: Artifact,
}

#[derive(Deserialize)]
pub struct Artifact {
    pub path: String,
    pub sha1: String,
    pub size: i32,
    pub url: String,
}
