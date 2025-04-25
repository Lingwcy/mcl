use std::fs;
use std::path::Path;
use std::path::PathBuf;

pub struct MinecraftPath {
    root: PathBuf,
    data_path: Option<DataPath>,
}

pub struct DataPath {
    pub screenshots: Vec<String>,
    pub version: Vec<VersionPath>,
    pub mods: Vec<ModPath>,
}

pub struct VersionPath {
    pub root: PathBuf,
    pub name: String,
    pub mods: Vec<ModPath>,
}

pub struct ModPath {
    pub path: String,
    pub name: String,
    pub location: String, 
}

impl MinecraftPath {
    pub fn new(root_path: &str) -> Self {
        let path = PathBuf::from(root_path);
        let mut minecraft_path = MinecraftPath {
            root: path.clone(),
            data_path: Some(DataPath {
                screenshots: Vec::new(),
                version: Vec::new(),
                mods: Vec::new(), 
            }),
        };

        if let Some(data_path) = &mut minecraft_path.data_path {
            data_path.screenshots = Self::init_screenshots(&path);
            data_path.mods = Self::init_root_mods(&path);
            data_path.version = Self::init_versions(&path);
        }

        minecraft_path
    }

    // 初始化截图
    fn init_screenshots(path: &PathBuf) -> Vec<String> {
        let mut screen_shots_path = path.clone();
        screen_shots_path.push("screenshots");

        if !screen_shots_path.exists() {
            return Vec::new();
        }

        if let Ok(entries) = fs::read_dir(&screen_shots_path) {
            entries
                .filter_map(|entry| entry.ok())
                .filter_map(|entry| {
                    let path = entry.path();
                    if path.is_file() {
                        if let Some(ext) = path.extension() {
                            let ext_str = ext.to_string_lossy().to_lowercase();
                            if ["png", "jpg", "jpeg", "gif", "bmp"].contains(&ext_str.as_str()) {
                                return path.to_str().map(|s| s.to_string());
                            }
                        }
                    }
                    None
                })
                .collect()
        } else {
            Vec::new()
        }
    }

    // 初始化根目录下的mods
    fn init_root_mods(path: &PathBuf) -> Vec<ModPath> {
        let mut mods_path = path.clone();
        mods_path.push("mods");

        if !mods_path.exists() {
            return Vec::new();
        }

        Self::scan_mods_directory(&mods_path, "global".to_string())
    }

    // 扫描指定目录下的mod文件，指定location
    fn scan_mods_directory(mods_path: &PathBuf, location: String) -> Vec<ModPath> {
        if let Ok(entries) = fs::read_dir(mods_path) {
            entries
                .filter_map(|entry| entry.ok())
                .filter_map(|entry| {
                    let path = entry.path();
                    if path.is_file() {
                        if let Some(ext) = path.extension() {
                            let ext_str = ext.to_string_lossy().to_lowercase();
                            if ext_str == "jar" {
                                if let Some(name) = path.file_stem() {
                                    return Some(ModPath {
                                        path: path.to_string_lossy().to_string(),
                                        name: name.to_string_lossy().to_string(),
                                        location: location.clone(),
                                    });
                                }
                            }
                        }
                    }
                    None
                })
                .collect()
        } else {
            Vec::new()
        }
    }

    fn init_versions(path: &PathBuf) -> Vec<VersionPath> {
        let mut versions_path = path.clone();
        versions_path.push("versions");

        if !versions_path.exists() {
            return Vec::new();
        }

        if let Ok(entries) = fs::read_dir(&versions_path) {
            entries
                .filter_map(|entry| entry.ok())
                .filter_map(|entry| {
                    let path = entry.path();
                    if path.is_dir() {
                        if let Some(name) = path.file_name() {
                            let version_name = name.to_string_lossy().to_string();

                            // 检查版本特定的mods目录
                            let mut version_mods_path = path.clone();
                            version_mods_path.push("mods");
                            let version_mods = if version_mods_path.exists() {
                                Self::scan_mods_directory(&version_mods_path, version_name.clone())
                            } else {
                                Vec::new()
                            };

                            return Some(VersionPath {
                                root: path.clone(),
                                name: version_name,
                                mods: version_mods,
                            });
                        }
                    }
                    None
                })
                .collect()
        } else {
            Vec::new()
        }
    }

    pub fn get_path(&self) -> &Path {
        self.root.as_path()
    }

    pub fn get_sceenshots(&mut self) -> Result<(PathBuf, Vec<String>), &'static str> {
        let mut screen_shots_path = self.root.clone();
        screen_shots_path.push("screenshots");

        if !screen_shots_path.exists() {
            return Ok((screen_shots_path, Vec::new()));
        }

        if self.data_path.is_none() {
            return Err("数据路径模块还没有被初始化");
        }

        let screenshots = self.data_path.as_ref().unwrap().screenshots.clone();

        Ok((screen_shots_path, screenshots))
    }

    pub fn get_versions(&self) -> Result<Vec<&VersionPath>, &'static str> {
        if let Some(data_path) = &self.data_path {
            Ok(data_path.version.iter().collect())
        } else {
            Err("数据路径模块还没有被初始化")
        }
    }

    // 获取所有mods（包括根目录和版本特定的）
    pub fn get_all_mods(&self) -> Result<Vec<&ModPath>, &'static str> {
        if let Some(data_path) = &self.data_path {
            let mut all_mods = Vec::new();

            // 添加根目录mods
            for mod_path in &data_path.mods {
                all_mods.push(mod_path);
            }

            // 添加版本特定mods
            for version in &data_path.version {
                for mod_path in &version.mods {
                    all_mods.push(mod_path);
                }
            }

            Ok(all_mods)
        } else {
            Err("数据路径模块还没有被初始化")
        }
    }

    // 获取根目录mods
    pub fn get_root_mods(&self) -> Result<&Vec<ModPath>, &'static str> {
        if let Some(data_path) = &self.data_path {
            Ok(&data_path.mods)
        } else {
            Err("数据路径模块还没有被初始化")
        }
    }

    // 获取特定版本的mods
    pub fn get_version_mods(&self, version_name: &str) -> Result<&Vec<ModPath>, &'static str> {
        if let Some(data_path) = &self.data_path {
            for version in &data_path.version {
                if version.name == version_name {
                    return Ok(&version.mods);
                }
            }
            Err("未找到指定版本")
        } else {
            Err("数据路径模块还没有被初始化")
        }
    }
}
