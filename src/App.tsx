import { useState } from "react";

function App() {
  const [magnetLink, setMagnetLink] = useState("");
  const [status, setStatus] = useState("idle");

  const handleLoadTorrent = () => {
    if (!magnetLink) return;
    setStatus("loading");
    // TODO: Integrate WebTorrent
  };

  return (
    <div className="app">
      <header className="header">
        <h1>TorrSyncPlayer</h1>
      </header>
      
      <main className="main">
        <div className="input-section">
          <input
            type="text"
            value={magnetLink}
            onChange={(e) => setMagnetLink(e.target.value)}
            placeholder="Enter magnet link..."
            className="magnet-input"
          />
          <button onClick={handleLoadTorrent} className="load-btn">
            Load
          </button>
        </div>
        
        <div className="player-section">
          <div className="video-placeholder">
            {status === "loading" ? "Loading..." : "Enter a magnet link to start"}
          </div>
        </div>
        
        <div className="status-bar">
          <span>Status: {status}</span>
        </div>
      </main>
    </div>
  );
}

export default App;