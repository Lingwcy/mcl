#![cfg_attr(
    all(not(debug_assertions), target_os = "windows"),
    windows_subsystem = "windows"
)]
use std::path::Path;
use std::sync::Mutex;
use std::sync::Arc;
use std::sync::Once;

use download::{get, Download, LibaryAllowed};
use model::version::Version;
use parse::Parse;
use tauri::command;
use serde::{Serialize, Deserialize};
use path::MinecraftPath;  

#[derive(Serialize, Deserialize)]
struct MinecraftVersion {
    id: String,
    type_: String,
    url: String,
    time: String,
    release_time: String,
}

impl From<model::version_manifest::Version> for MinecraftVersion {
    fn from(value: model::version_manifest::Version) -> Self {
        Self {
            id: value.id,
            type_: value.type_,
            url: value.url,
            time: value.time,
            release_time: value.release_time,
        }
    }
}

#[command]
fn get_default_game_directory() -> String {
    let game_dir = std::env::current_dir().unwrap().join(".minecraft");
    game_dir.to_string_lossy().to_string()
}

#[command]
fn a_test() -> String{
    "hello".to_string()
}

#[command]
fn search_versions(version_filter: Option<String>, version_type: String) -> Vec<MinecraftVersion> {
    let versions = get_version_manifest().versions;

    versions.into_iter()
        .filter(|v| {
            (if let Some(filter) = &version_filter {
                v.id.contains(filter)
            } else {
                true
            }) && v.type_.eq(&version_type)
        })
        .map(MinecraftVersion::from)
        .collect()
}

#[command]
fn download_version(version_id: String, game_dir: String) -> Result<String, String> {
    let game_dir = Path::new(&game_dir);
    let versions = get_version_manifest().versions;

    if let Some(version) = versions.iter().find(|v| v.id.eq(&version_id)) {
        version.download(game_dir).map_err(|err| format!("Download Error: {}", err))?;
        Ok(format!("Successfully downloaded version {}", version_id))
    } else {
        Err(format!("Version: {} not found", version_id))
    }
}

#[command]
fn launch_game(username: String, version_id: String, game_dir: String) -> Result<String, String> {
    let game_dir = Path::new(&game_dir);
    let libraries_dir = game_dir.join("libraries");
    let assets_dir = game_dir.join("assets");
    let version_dir = game_dir.join("versions").join(&version_id);
    let natives_dir = version_dir.join("natives");
    let config_path = version_dir.join(format!("{}.json", version_id));
    let version_path = version_dir.join(format!("{}.jar", version_id));

    if !version_path.exists() || !config_path.exists() {
        return Err(format!("Version: {} not found", version_id));
    }

    let version = &Version::parse(&std::fs::read_to_string(&config_path).map_err(|e| e.to_string())?)
        .map_err(|e| e.to_string())?;

    if !natives_dir.exists() {
        std::fs::create_dir_all(&natives_dir).map_err(|e| e.to_string())?;
    }

    for library in &version.libraries {
        if library.allowed() && library.name.contains("natives") {
            extract_jar(
                &libraries_dir.join(&library.downloads.artifact.path),
                &natives_dir,
            );
        }
    }

    let classpath = format!(
        "{}{}",
        &version
            .libraries
            .iter()
            .map(|library| {
                format!(
                    "{}{}",
                    libraries_dir
                        .join(&library.downloads.artifact.path)
                        .display(),
                    if cfg!(windows) { ";" } else { ":" }
                )
            })
            .collect::<String>(),
        version_path.display()
    );

    let status = std::process::Command::new("java")
        .current_dir(&game_dir)
        .arg(format!("-Djava.library.path={}", natives_dir.display()))
        .arg("-cp")
        .arg(classpath)
        .arg(&version.main_class)
        .arg("--username")
        .arg(username)
        .arg("--version")
        .arg(&version.id)
        .arg("--gameDir")
        .arg(&game_dir)
        .arg("--assetsDir")
        .arg(assets_dir)
        .arg("--assetIndex")
        .arg(&version.asset_index.id)
        .arg("--accessToken")
        .arg("0")
        .arg("--versionType")
        .arg("RMCL 0.1.0")
        .status()
        .map_err(|e| e.to_string())?;

    if status.success() {
        Ok(format!("Game {} launched and exited successfully", version_id))
    } else {
        Err(format!("Game exited with code: {:?}", status.code()))
    }
}

#[command]
fn get_installed_versions(game_dir: String) -> Vec<String> {
    let game_dir = Path::new(&game_dir);
    let versions_dir = game_dir.join("versions");
    
    if !versions_dir.exists() {
        return Vec::new();
    }
    
    match std::fs::read_dir(versions_dir) {
        Ok(entries) => {
            entries
                .filter_map(|entry| {
                    if let Ok(entry) = entry {
                        let path = entry.path();
                        if path.is_dir() {
                            path.file_name().map(|name| name.to_string_lossy().to_string())
                        } else {
                            None
                        }
                    } else {
                        None
                    }
                })
                .collect()
        },
        Err(_) => Vec::new(),
    }
}

// Maintain multiple Minecraft paths
static INIT: Once = Once::new();
static mut MINECRAFT_PATHS: Option<Arc<Mutex<Vec<MinecraftPath>>>> = None;

// Function to initialize and add a new MinecraftPath instance to the collection
fn initialize_minecraft_path(root_path: &str) -> Result<String, String> {
    unsafe {
        // First time initialization
        if MINECRAFT_PATHS.is_none() {
            INIT.call_once(|| {
                let initial_paths = vec![MinecraftPath::new(root_path)];
                MINECRAFT_PATHS = Some(Arc::new(Mutex::new(initial_paths)));
            });
        } 
        // Add to existing collection
        else if let Some(instance) = &MINECRAFT_PATHS {
            if let Ok(mut guard) = instance.lock() {
                // Check if path already exists to avoid duplicates
                if !guard.iter().any(|path| path.get_path().to_str() == Some(root_path)) {
                    guard.push(MinecraftPath::new(root_path));
                }
            } else {
                return Err("Failed to acquire lock on MinecraftPaths".to_string());
            }
        }
    }
    
    Ok("添加新的游戏路径成功".to_string())
}

// Function to initialize a MinecraftPath with a custom name
fn initialize_minecraft_path_with_name(root_path: &str, name: &str) -> Result<String, String> {
    unsafe {
        // First time initialization
        if MINECRAFT_PATHS.is_none() {
            INIT.call_once(|| {
                let initial_paths = vec![MinecraftPath::new_with_name(root_path, name)];
                MINECRAFT_PATHS = Some(Arc::new(Mutex::new(initial_paths)));
            });
        } 
        // Add to existing collection
        else if let Some(instance) = &MINECRAFT_PATHS {
            if let Ok(mut guard) = instance.lock() {
                // Check if path already exists to avoid duplicates
                if !guard.iter().any(|path| path.get_path().to_str() == Some(root_path)) {
                    guard.push(MinecraftPath::new_with_name(root_path, name));
                }
            } else {
                return Err("Failed to acquire lock on MinecraftPaths".to_string());
            }
        }
    }
    
    Ok("添加新的游戏路径成功".to_string())
}

// Function to get a reference to the initialized MinecraftPaths collection
fn get_minecraft_paths() -> Result<Arc<Mutex<Vec<MinecraftPath>>>, String> {
    unsafe {
        if let Some(instance) = &MINECRAFT_PATHS {
            Ok(instance.clone())
        } else {
            Err("MinecraftPaths has not been initialized yet".to_string())
        }
    }
}

#[command]
fn initialize_game_path(root_path: String) -> Result<String, String> {
    initialize_minecraft_path(&root_path)
}

#[command]
fn initialize_game_path_with_name(root_path: String, name: String) -> Result<String, String> {
    initialize_minecraft_path_with_name(&root_path, &name)
}

#[command]
fn get_minecraft_path_info() -> Result<Vec<String>, String> {
    let instance = get_minecraft_paths()?;
    let paths = instance.lock().map_err(|_| "Failed to acquire lock on MinecraftPaths".to_string())?;
    
    if paths.is_empty() {
        return Err("No Minecraft paths have been initialized".to_string());
    }
    
    Ok(paths.iter().map(|p| p.get_path().to_string_lossy().to_string()).collect())
}

#[derive(Serialize)]
struct RootPathInfo {
    path: String,
    display_name: String,
}

#[derive(Serialize)]
struct VersionInfo {
    name: String,
    path: String,
    mod_count: usize,
}

#[derive(Serialize)]
struct ModInfo {
    name: String,
    path: String,
    location: String,
}

#[command]
fn get_all_minecraft_paths() -> Result<Vec<RootPathInfo>, String> {
    let instance = get_minecraft_paths()?;
    let paths = instance.lock().map_err(|_| "Failed to acquire lock on MinecraftPaths".to_string())?;
    
    Ok(paths.iter().map(|p| RootPathInfo {
        path: p.get_path().to_string_lossy().to_string(),
        display_name: p.get_display_name().to_string(),
    }).collect())
}

#[command]
fn get_minecraft_versions_for_path(path: String) -> Result<Vec<VersionInfo>, String> {
    let instance = get_minecraft_paths()?;
    let paths = instance.lock().map_err(|_| "Failed to acquire lock on MinecraftPaths".to_string())?;
    
    // Find the specified path
    if let Some(mc_path) = paths.iter().find(|p| p.get_path().to_string_lossy() == path) {
        match mc_path.get_versions() {
            Ok(versions) => Ok(versions.iter().map(|v| VersionInfo {
                name: v.name.clone(),
                path: v.root.to_string_lossy().to_string(),
                mod_count: v.mods.len(),
            }).collect()),
            Err(e) => Err(e.to_string()),
        }
    } else {
        Err("Specified Minecraft path not found".to_string())
    }
}

#[command]
fn get_minecraft_mods_for_version(root_path: String, version_name: String) -> Result<Vec<ModInfo>, String> {
    let instance = get_minecraft_paths()?;
    let paths = instance.lock().map_err(|_| "Failed to acquire lock on MinecraftPaths".to_string())?;
    
    // Find the specified path
    if let Some(mc_path) = paths.iter().find(|p| p.get_path().to_string_lossy() == root_path) {
        match mc_path.get_version_mods(&version_name) {
            Ok(mods) => Ok(mods.iter().map(|m| ModInfo {
                name: m.name.clone(),
                path: m.path.clone(),
                location: m.location.clone(),
            }).collect()),
            Err(e) => Err(e.to_string()),
        }
    } else {
        Err("Specified Minecraft path not found".to_string())
    }
}

#[command]
fn get_minecraft_screenshots() -> Result<Vec<String>, String> {
    let instance = get_minecraft_paths()?;
    let mut paths = instance.lock().map_err(|_| "Failed to acquire lock on MinecraftPaths".to_string())?;
    
    if paths.is_empty() {
        return Err("No Minecraft paths have been initialized".to_string());
    }
    
    // For now, just return screenshots from the first path
    match paths[0].get_sceenshots() {
        Ok((_, screenshots)) => Ok(screenshots),
        Err(e) => Err(e.to_string()),
    }
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_global_shortcut::Builder::new().build())
        .plugin(tauri_plugin_http::init())
        .plugin(tauri_plugin_os::init())
        .plugin(tauri_plugin_notification::init())
        .plugin(tauri_plugin_clipboard_manager::init())
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_fs::init())
        .plugin(tauri_plugin_dialog::init())
        .invoke_handler(tauri::generate_handler![
            search_versions,
            download_version,
            launch_game,
            get_default_game_directory,
            get_installed_versions,
            a_test,
            // Updated MinecraftPath commands
            initialize_game_path,
            initialize_game_path_with_name,
            get_minecraft_path_info,
            get_minecraft_screenshots,
            // Remove the undefined functions
            // get_minecraft_versions,
            // get_minecraft_all_mods,
            // get_minecraft_root_mods,
            // get_minecraft_version_mods,
            get_all_minecraft_paths,
            get_minecraft_versions_for_path,
            get_minecraft_mods_for_version,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}

fn get_version_manifest() -> model::version_manifest::VersionManifest {
    return get("https://launchermeta.mojang.com/mc/game/version_manifest.json")
        .unwrap()
        .json::<model::version_manifest::VersionManifest>()
        .unwrap();
}

fn extract_jar(jar: &Path, dir: &Path) {
    let mut archive = zip::ZipArchive::new(std::fs::File::open(jar).unwrap()).unwrap();
    for i in 0..archive.len() {
        let mut entry = archive.by_index(i).unwrap();
        if entry.is_file() && !entry.name().contains("META-INF") {
            let mut name = entry.name();

            if name.contains("/") {
                name = &name[entry.name().rfind('/').unwrap() + 1..];
            }

            let path = dir.join(name);

            if path.exists() {
                std::fs::remove_file(&path).unwrap();
            }

            let mut file = std::fs::File::create(&path).unwrap();

            std::io::copy(&mut entry, &mut file).unwrap();
        }
    }
}

