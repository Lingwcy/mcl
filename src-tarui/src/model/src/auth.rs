use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
pub struct User {
    pub username: String,
    pub uuid: Option<String>,
    pub access_token: Option<String>,
}

impl Default for User {
    fn default() -> Self {
        Self {
            username: "Player".to_string(),
            uuid: None,
            access_token: None,
        }
    }
}
