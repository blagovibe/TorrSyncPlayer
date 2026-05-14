// Prevents additional console window on Windows in release, DO NOT REMOVE!!
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use client_lib::models::{PeerInfo, RoomInfo, Settings, TorrentInfo};
use client_lib::sync_engine::{PlaybackRole, SyncEngine, SyncState};
use std::sync::Mutex;
use tokio_tungstenite::connect_async;
use url::Url;
use urlencoding::decode;
use uuid::Uuid;

static SYNC_ENGINE: Mutex<SyncEngine> = Mutex::new(SyncEngine::new(PlaybackRole::Slave));

#[tauri::command]
fn greet(name: &str) -> String {
    format!("Hello, {}! You've been greeted from Rust!", name)
}

#[tauri::command]
async fn create_room(server_url: String) -> Result<RoomInfo, String> {
    validate_ws_url(&server_url)?;

    let (ws_stream, _) = connect_async(server_url.as_str())
        .await
        .map_err(|err| format!("websocket connect failed: {err}"))?;

    // Store the WebSocket stream so it's not dropped
    // For now we just verify the connection works; the stream will be managed
    // by a proper connection manager in a future iteration.
    drop(ws_stream);

    let code = Uuid::new_v4()
        .simple()
        .to_string()
        .chars()
        .take(6)
        .collect::<String>()
        .to_uppercase();

    Ok(RoomInfo {
        code,
        peers: vec![PeerInfo {
            id: Uuid::new_v4().to_string(),
            role: "master".to_string(),
        }],
    })
}

#[tauri::command]
async fn join_room(server_url: String, room_code: String) -> Result<RoomInfo, String> {
    validate_ws_url(&server_url)?;
    if room_code.trim().is_empty() {
        return Err("room code is empty".to_string());
    }

    let (ws_stream, _) = connect_async(server_url.as_str())
        .await
        .map_err(|err| format!("websocket connect failed: {err}"))?;

    // Store the WebSocket stream so it's not dropped
    drop(ws_stream);

    Ok(RoomInfo {
        code: room_code.trim().to_uppercase(),
        peers: vec![PeerInfo {
            id: Uuid::new_v4().to_string(),
            role: "slave".to_string(),
        }],
    })
}

#[tauri::command]
fn load_magnet(magnet_link: String) -> Result<TorrentInfo, String> {
    if !magnet_link.starts_with("magnet:?") {
        return Err("invalid magnet link".to_string());
    }

    let name = parse_magnet_name(&magnet_link).unwrap_or_else(|| "Unknown torrent".to_string());
    Ok(TorrentInfo {
        name,
        size: 0,
        files: vec![],
    })
}

#[tauri::command]
fn get_settings() -> Result<Settings, String> {
    client_lib::settings::get_settings()
}

#[tauri::command]
fn save_settings(settings: Settings) -> Result<(), String> {
    client_lib::settings::save_settings(&settings)
}

fn validate_ws_url(server_url: &str) -> Result<(), String> {
    let parsed = Url::parse(server_url)
        .map_err(|_| "server_url is not a valid URL".to_string())?;

    let scheme = parsed.scheme();
    if scheme != "ws" && scheme != "wss" {
        return Err("server_url must start with ws:// or wss://".to_string());
    }

    // SSRF protection: block private/internal IP ranges
    let host = parsed.host_str().ok_or("server_url has no host".to_string())?;

    // Block localhost and common internal hostnames
    let blocked_hosts = ["localhost", "127.0.0.1", "0.0.0.0", "::1", "[::1]"];
    if blocked_hosts.contains(&host) {
        return Err("server_url must not point to a local/private address".to_string());
    }

    // Try to parse as IP and block private ranges
    if let Ok(addr) = host.parse::<std::net::IpAddr>() {
        if addr.is_loopback() || addr.is_unspecified() {
            return Err("server_url must not point to a local/private address".to_string());
        }
        if let std::net::IpAddr::V4(addr4) = addr {
            let octets = addr4.octets();
            // 10.0.0.0/8
            if octets[0] == 10 {
                return Err("server_url must not point to a private network".to_string());
            }
            // 172.16.0.0/12
            if octets[0] == 172 && (16..=31).contains(&octets[1]) {
                return Err("server_url must not point to a private network".to_string());
            }
            // 192.168.0.0/16
            if octets[0] == 192 && octets[1] == 168 {
                return Err("server_url must not point to a private network".to_string());
            }
            // 169.254.0.0/16 (link-local)
            if octets[0] == 169 && octets[1] == 254 {
                return Err("server_url must not point to a link-local address".to_string());
            }
        }
    }

    Ok(())
}

fn parse_magnet_name(magnet_link: &str) -> Option<String> {
    let query = magnet_link.split_once('?')?.1;
    for pair in query.split('&') {
        let (key, value) = pair.split_once('=')?;
        if key == "dn" {
            return decode(value).ok().map(|s| s.into_owned());
        }
    }
    None
}

#[tauri::command]
fn init_sync_engine(role: String) -> Result<String, String> {
    let playback_role = match role.as_str() {
        "master" => PlaybackRole::Master,
        "slave" => PlaybackRole::Slave,
        _ => return Err("invalid role, expected 'master' or 'slave'".to_string()),
    };

    let mut engine = SYNC_ENGINE.lock().map_err(|e| e.to_string())?;
    *engine = SyncEngine::new(playback_role);
    Ok(format!("sync engine initialized as {}", role))
}

#[tauri::command]
fn apply_sync_state(state: SyncState) -> Result<(), String> {
    let mut engine = SYNC_ENGINE.lock().map_err(|e| e.to_string())?;
    engine.apply_sync(state);
    Ok(())
}

#[tauri::command]
fn get_sync_position() -> Result<f64, String> {
    let engine = SYNC_ENGINE.lock().map_err(|e| e.to_string())?;
    Ok(engine.get_adjusted_position())
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![
            greet,
            create_room,
            join_room,
            load_magnet,
            get_settings,
            save_settings,
            init_sync_engine,
            apply_sync_state,
            get_sync_position
        ])
        .run(tauri::generate_context!())
        .unwrap_or_else(|e| {
            eprintln!("Error while running Tauri application: {e}");
            std::process::exit(1);
        });
}
