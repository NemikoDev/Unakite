import { useState } from 'react';
import { ChatMessage, FileTransferItem, Peer } from '../../types';
import { FileTransferBar } from '../file/FileTransferBar';
import { Button } from '../Button';

interface ChatViewProps {
  roomCode: string;
  self: Peer;
  peer: Peer | null;
  messages: ChatMessage[];
  transfers: FileTransferItem[];
  onSendMessage: (content: string) => void;
  onSendFile: () => void;
}

export function ChatView({ roomCode, self, peer, messages, transfers, onSendMessage, onSendFile }: ChatViewProps) {
  const [draft, setDraft] = useState('');

  const send = () => {
    if (!draft.trim()) return;
    onSendMessage(draft.trim());
    setDraft('');
  };

  return (
    <div className="flex-1 flex flex-col">
      <header className="border-b border-zinc-800 px-4 py-3 flex items-center justify-between">
        <div className="text-sm">
          <span className="text-zinc-500">Room </span>
          <span className="font-mono tracking-widest text-zinc-200">{roomCode}</span>
        </div>
        <div className="text-xs text-zinc-500">
          {peer ? `Connected to ${peer.nickname}` : 'Waiting for peer…'}
        </div>
      </header>

      <div className="flex-1 overflow-y-auto px-4 py-4 space-y-3">
        {messages.map((m) => (
          <div key={m.id} className={`flex flex-col ${m.isOwn ? 'items-end' : 'items-start'}`}>
            <span className="text-[10px] text-zinc-500 mb-0.5">{m.sender.nickname}</span>
            <div className={`max-w-xs px-3 py-2 rounded-md text-sm ${m.isOwn ? 'bg-blue-600 text-white' : 'bg-zinc-800 text-zinc-100'}`}>
              {m.content}
            </div>
          </div>
        ))}
        {transfers.map((t) => (
          <div key={t.id} className={`flex ${t.direction === 'sending' ? 'justify-end' : 'justify-start'}`}>
            <FileTransferBar item={t} />
          </div>
        ))}
      </div>

      <div className="border-t border-zinc-800 p-3 flex gap-2">
        <Button variant="ghost" onClick={onSendFile}>+ File</Button>
        <input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && send()}
          placeholder="Message"
          className="flex-1 bg-zinc-900 border border-zinc-700 rounded-md px-3 py-2 text-sm outline-none focus:border-blue-500"
        />
        <Button onClick={send}>Send</Button>
      </div>
    </div>
  );
}