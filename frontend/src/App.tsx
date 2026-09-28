import { useState } from 'react';
import { Layout } from './components/Layout';
import { RoomJoin } from './components/room/RoomJoin';
import { ChatView } from './components/chat/ChatView';
import { RoomState, ChatMessage, Peer } from './types';
import { mockMessages, mockPeer, mockTransfer } from './mocks/mockData';

function generateMockCode() {
  return Math.random().toString(36).slice(2, 8).toUpperCase();
}

export default function App() {
  const [room, setRoom] = useState<RoomState>({ status: 'idle', roomCode: null, self: null, peer: null });
  const [messages, setMessages] = useState<ChatMessage[]>(mockMessages);

  const handleCreateRoom = (nickname: string) => {
    const self: Peer = { id: 'self-' + Date.now(), nickname };
    setRoom({ status: 'connected', roomCode: generateMockCode(), self, peer: mockPeer });
  };

  const handleJoinRoom = (nickname: string, code: string) => {
    const self: Peer = { id: 'self-' + Date.now(), nickname };
    setRoom({ status: 'connected', roomCode: code, self, peer: mockPeer });
  };

  const handleSendMessage = (content: string) => {
    if (!room.self) return;
    setMessages((prev) => [...prev, { id: String(prev.length + 1), sender: room.self!, content, timestamp: Date.now(), isOwn: true }]);
  };

  return (
    <Layout>
      {room.status !== 'connected' || !room.self ? (
        <RoomJoin onCreateRoom={handleCreateRoom} onJoinRoom={handleJoinRoom} />
      ) : (
        <ChatView
          roomCode={room.roomCode!}
          self={room.self}
          peer={room.peer}
          messages={messages}
          transfers={[mockTransfer]}
          onSendMessage={handleSendMessage}
          onSendFile={() => {}}
        />
      )}
    </Layout>
  );
}