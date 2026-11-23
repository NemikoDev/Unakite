import { useLayoutEffect, useRef, useState } from 'react';
import { ChatMessage, FileTransferItem, Peer } from '../../types';
import { FileTransferBar } from '../file/FileTransferBar';
import { Button } from '../Button';

interface ChatViewProps {
  roomCode: string;
  self: Peer;
  peer: Peer | null;
  messages: ChatMessage[];
  transfers: FileTransferItem[];
  closesAt: number | null;
  onSendMessage: (content: string) => void;
  onSendFile: () => void;
  onLeave: () => void;
}

export function ChatView({ roomCode, peer, messages, transfers, closesAt, onSendMessage, onSendFile, onLeave }: ChatViewProps) {
  const [draft, setDraft] = useState('');
  const scrollRef = useRef<HTMLDivElement>(null);
  const stickToBottom = useRef(true);

  const handleScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  };


  useLayoutEffect(() => {
    const el = scrollRef.current;
    if (el && stickToBottom.current) el.scrollTop = el.scrollHeight;
  }, [messages, transfers]);

  const canSend = peer !== null;
  const status = peer ? `connected to ${peer.nickname}` : closesAt ? `closing at ${new Date(closesAt).toLocaleTimeString()}` : 'waiting for peer…';
  const send = () => {
    if (!canSend || !draft.trim()) return;
    stickToBottom.current = true;
    onSendMessage(draft.trim());
    setDraft('');
  };

  return (
    <div className="flex-1 min-h-0 flex flex-col">
      <header className="shrink-0 border-b border-zinc-800 px-4 py-3 flex items-center justify-between">
        <div className="text-sm">
          <span className="text-zinc-500">Room </span>
          <span className="font-mono tracking-widest text-zinc-200">{roomCode}</span>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-zinc-500">
            {peer ? `Connected to ${peer.nickname}` : 'Waiting for peer…'}
          </span>
          <Button variant="ghost" onClick={onLeave}>Leave</Button>
        </div>
      </header>

      <div ref={scrollRef} onScroll={handleScroll} className="flex-1 min-h-0 overflow-y-auto px-4 py-4 space-y-3">
        {messages.map((m) =>
          m.kind === 'system' ? (
            <div key={m.id} className="text-center text-xs text-zinc-500 py-1">{m.content}</div>
          ) : (
            <div key={m.id} className={`flex flex-col ${m.isOwn ? 'items-end' : 'items-start'}`}>
              <span className="text-[10px] text-zinc-500 mb-0.5">{m.sender.nickname}</span>
              <div className={`max-w-xs px-3 py-2 rounded-md text-sm break-words whitespace-pre-wrap ${m.isOwn ? 'bg-blue-600 text-white' : 'bg-zinc-800 text-zinc-100'}`}>
                {m.content}
              </div>
            </div>
          ),
        )}
        {transfers.map((t) => (
          <div key={t.id} className={`flex ${t.direction === 'sending' ? 'justify-end' : 'justify-start'}`}>
            <FileTransferBar item={t} />
          </div>
        ))}
      </div>

      <div className="shrink-0 border-t border-zinc-800 p-3 flex gap-2">
        <Button variant="ghost" onClick={onSendFile} disabled={!canSend}>+ File</Button>
        <input
          value={draft}
          disabled={!canSend}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && send()}
          placeholder={canSend ? 'Message' : 'Nobody to message right now'}
          className="flex-1 bg-zinc-900 border border-zinc-700 rounded-md px-3 py-2 text-sm outline-none focus:border-blue-500 disabled:opacity-40"
        />
        <Button onClick={send} disabled={!canSend}>Send</Button>
      </div>
    </div>
  );
}