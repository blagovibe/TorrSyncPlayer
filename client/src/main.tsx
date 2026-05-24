import "./browser-polyfills";
import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import ErrorBoundary from "./components/ErrorBoundary";
import { RoomStateProvider } from "./contexts/RoomStateContext";

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <ErrorBoundary>
      <RoomStateProvider>
        <App />
      </RoomStateProvider>
    </ErrorBoundary>
  </React.StrictMode>,
);
