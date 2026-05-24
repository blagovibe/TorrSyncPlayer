import { useContext } from "react";
import { RoomStateContext, type RoomStateContextValue } from "../contexts/RoomStateContext";

export function useRoomStateContext(): RoomStateContextValue {
  const ctx = useContext(RoomStateContext);
  if (!ctx) throw new Error("useRoomStateContext must be used within RoomStateProvider");
  return ctx;
}
