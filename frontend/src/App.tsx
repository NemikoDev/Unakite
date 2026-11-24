import { useEffect, useRef, useState } from 'react';
import { Layout } from './components/Layout';
import { RoomJoin } from './components/room/RoomJoin';
import { ChatView } from './components/chat/ChatView';
import { ChatMessage, FileTransferItem, Peer, PeerLeft } from './types';
import { EVENT_MESSAGE, EVENT_PEER_JOINED, EVENT_PEER_LEFT, EVENT_ROOM_CLOSED, EVENT_FILE_TRANSFER } from './types/events';
import { useWailsEvent } from './hooks/useWailsEvent';
import { CreateRoom, JoinRoom, SendMessage, LeaveRoom, GetState, SendFile, AcceptFile, DeclineFile } from '../wailsjs/go/main/App';

interface ActiveRoom {
  code: string;
  self: Peer;
}

export default function App() {
  const [room, setRoom] = useState<ActiveRoom | null>(null);
  const [peer, setPeer] = useState<Peer | null>(null);
  const [closesAt, setClosesAt] = useState<number | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const systemSeq = useRef(0);
  const [transfers, setTransfers] = useState<FileTransferItem[]>([]);

  const resetChat = () => {
    setMessages([]);
    setPeer(null);
    setClosesAt(null);
    setTransfers([]);
  };

  const addMessage = (m: ChatMessage) =>
    setMessages((prev) => (prev.some((x) => x.id === m.id) ? prev : [...prev, m]));

  const isFinished = (t: FileTransferItem) =>
    t.status === 'complete' || t.status === 'failed' || t.status === 'rejected';

  const upsertTransfer = (t: FileTransferItem) =>
    setTransfers((prev) => {
      const i = prev.findIndex((x) => x.id === t.id);
      if (i < 0) return [...prev, t];
      if (isFinished(prev[i]) && !isFinished(t)) return prev;
      const next = [...prev];
      next[i] = t;
      return next;
    });

  const addSystemLine = (text: string) =>
    addMessage({
      id: `sys-${++systemSeq.current}`,
      sender: { id: '', nickname: '' },
      content: text,
      timestamp: Date.now(),
      isOwn: false,
      kind: 'system',
    });

  useEffect(() => {
    GetState()
      .then((s) => {
        if (s.inRoom) {
          setRoom({ code: s.code, self: s.self });
          setPeer(s.peer ?? null);
          setClosesAt(s.closesAt || null);
          setTransfers((s.transfers ?? []) as FileTransferItem[]);
        }
      })
      .catch((e) => setError(String(e)));
  }, []);

  useWailsEvent<Peer>(EVENT_PEER_JOINED, (p) => {
    setPeer(p);
    setClosesAt(null);
    addSystemLine(`${p.nickname} joined the chat`);
  });
  useWailsEvent<PeerLeft>(EVENT_PEER_LEFT, (e) => {
    setPeer(null);
    setClosesAt(e.closesAt);
    addSystemLine(`${e.nickname || 'The other person'} has left the chat`);
  });
  useWailsEvent<ChatMessage>(EVENT_MESSAGE, addMessage);
  useWailsEvent<{ reason: string }>(EVENT_ROOM_CLOSED, (e) => {
    setRoom(null);
    resetChat();
    setError(
      e.reason === 'expired'
        ? 'Nobody joined in time, so the room was closed. Create a new one.'
        : e.reason === 'peer-left'
          ? 'The other person left, so the room was closed and its history discarded.'
          : 'The room was closed.',
    );
  });
  useWailsEvent<FileTransferItem>(EVENT_FILE_TRANSFER, upsertTransfer);

  const handleCreateRoom = async (nickname: string) => {
    setBusy(true);
    setError(null);
    resetChat();
    try {
      const code = await CreateRoom(nickname);
      setRoom({ code, self: { id: 'self', nickname } });
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const handleJoinRoom = async (nickname: string, code: string) => {
    setBusy(true);
    setError(null);
    resetChat();
    try {
      await JoinRoom(nickname, code);
      setRoom({ code: code.trim().toUpperCase(), self: { id: 'self', nickname } });
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  const handleSendMessage = async (content: string) => {
    try {
      addMessage(await SendMessage(content));
    } catch (e) {
      setError(String(e));
    }
  };

  const handleLeave = async () => {
    await LeaveRoom();
    setRoom(null);
    resetChat();
  };

  const handleSendFile = async () => {
    try {
      await SendFile();
    } catch (e) {
      setError(String(e));
    }
  };
  const handleAcceptFile = (id: string) => AcceptFile(id).catch((e) => setError(String(e)));
  const handleDeclineFile = (id: string) => DeclineFile(id).catch((e) => setError(String(e)));

  return (
    <Layout>
      {error && (
        <div className="bg-red-950 text-red-300 text-xs px-4 py-2 flex justify-between">
          <span>{error}</span>
          <button onClick={() => setError(null)}>✕</button>
        </div>
      )}
      {!room ? (
        <RoomJoin onCreateRoom={handleCreateRoom} onJoinRoom={handleJoinRoom} busy={busy} />
      ) : (
        <ChatView
          roomCode={room.code}
          self={room.self}
          peer={peer}
          closesAt={closesAt}
          messages={messages}
          transfers={transfers}
          onSendMessage={handleSendMessage}
          onSendFile={handleSendFile}
          onAcceptFile={handleAcceptFile}
          onDeclineFile={handleDeclineFile}
          onLeave={handleLeave}
        />
      )}
    </Layout>
  );
}