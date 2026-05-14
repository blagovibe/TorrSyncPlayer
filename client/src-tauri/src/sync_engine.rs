use serde::{Deserialize, Serialize};
use std::time::{SystemTime, UNIX_EPOCH};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum PlaybackRole {
    Master,
    Slave,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
pub struct SyncState {
    pub role: PlaybackRole,
    pub position_secs: f64,
    pub is_playing: bool,
    pub server_ts_ms: u64,
}

#[derive(Debug, Clone)]
pub struct SyncEngine {
    role: PlaybackRole,
    position_secs: f64,
    is_playing: bool,
    latency_secs: f64,
}

impl Default for SyncEngine {
    fn default() -> Self {
        Self::new(PlaybackRole::Slave)
    }
}

impl SyncEngine {
    pub const fn new(role: PlaybackRole) -> Self {
        Self {
            role,
            position_secs: 0.0,
            is_playing: false,
            latency_secs: 0.0,
        }
    }

    pub fn apply_sync(&mut self, state: SyncState) {
        // Validate incoming state
        if !state.position_secs.is_finite() || state.position_secs < 0.0 {
            return; // Silently ignore invalid state
        }

        let now_ms = current_ts_ms();
        // Don't accept timestamps from the future (more than 5s tolerance)
        let latency_ms = if state.server_ts_ms > now_ms + 5000 {
            0
        } else {
            now_ms.saturating_sub(state.server_ts_ms)
        };
        self.latency_secs = latency_ms as f64 / 1000.0;
        self.role = state.role;
        self.is_playing = state.is_playing;
        self.position_secs = state.position_secs;
    }

    pub fn get_adjusted_position(&self) -> f64 {
        if self.is_playing {
            self.position_secs + self.latency_secs
        } else {
            self.position_secs
        }
    }

    pub fn role(&self) -> PlaybackRole {
        self.role
    }

    pub fn is_playing(&self) -> bool {
        self.is_playing
    }

    pub fn latency_secs(&self) -> f64 {
        self.latency_secs
    }
}

fn current_ts_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as u64)
        .unwrap_or(0)
}
