use std::fs;
use std::path::PathBuf;

use crate::models::Settings;

const SETTINGS_DIR: &str = "TorrSyncPlayer";
const SETTINGS_FILE: &str = "settings.json";

impl Default for Settings {
    fn default() -> Self {
        let fallback_cache = PathBuf::from("./cache");
        let cache_path = dirs::cache_dir()
            .unwrap_or(fallback_cache)
            .join(SETTINGS_DIR)
            .to_string_lossy()
            .into_owned();

        Self {
            server_url: "ws://localhost:8080".to_string(),
            cache_path,
            buffer_size: 8 * 1024 * 1024,
        }
    }
}

fn settings_path() -> Result<PathBuf, String> {
    let mut base = dirs::config_dir().ok_or_else(|| "config dir not found".to_string())?;
    base.push(SETTINGS_DIR);
    fs::create_dir_all(&base).map_err(|err| err.to_string())?;
    base.push(SETTINGS_FILE);
    Ok(base)
}

pub fn get_settings() -> Result<Settings, String> {
    let path = settings_path()?;
    if !path.exists() {
        let default_settings = Settings::default();
        save_settings(&default_settings)?;
        return Ok(default_settings);
    }

    let raw = fs::read_to_string(&path).map_err(|err| err.to_string())?;
    serde_json::from_str::<Settings>(&raw).map_err(|err| err.to_string())
}

pub fn save_settings(settings: &Settings) -> Result<(), String> {
    let path = settings_path()?;
    let payload = serde_json::to_string_pretty(settings).map_err(|err| err.to_string())?;
    fs::write(path, payload).map_err(|err| err.to_string())
}
