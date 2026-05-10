import { FormEvent, useState } from "react";

interface HomePageProps {
  roomCode: string;
  magnetLink: string;
  onMagnetLinkChange: (value: string) => void;
  onCreateRoom: () => void;
  onJoinRoom: (code: string) => void;
}

function HomePage({
  roomCode,
  magnetLink,
  onMagnetLinkChange,
  onCreateRoom,
  onJoinRoom,
}: HomePageProps) {
  const [joinCode, setJoinCode] = useState("");

  const handleJoin = (event: FormEvent) => {
    event.preventDefault();
    onJoinRoom(joinCode);
  };

  return (
    <section className="home-page">
      <h1 className="app-logo">TorrSyncPlayer</h1>
      <div className="panel">
        <button className="primary-btn" type="button" onClick={onCreateRoom}>
          Create Room
        </button>
        {roomCode && <p className="hint">Last room code: {roomCode}</p>}
      </div>

      <form className="panel" onSubmit={handleJoin}>
        <label htmlFor="join-code">Room code</label>
        <div className="row">
          <input
            id="join-code"
            value={joinCode}
            onChange={(event) => setJoinCode(event.target.value.toUpperCase())}
            placeholder="AB12CD"
          />
          <button type="submit">Join Room</button>
        </div>
      </form>

      <div className="panel">
        <label htmlFor="magnet-link">Magnet link</label>
        <textarea
          id="magnet-link"
          value={magnetLink}
          onChange={(event) => onMagnetLinkChange(event.target.value)}
          placeholder="magnet:?xt=urn:btih:..."
          rows={3}
        />
      </div>
    </section>
  );
}

export default HomePage;
