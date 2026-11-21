import { useState } from 'react';
import { Button } from '../Button';

interface RoomJoinProps {
  onCreateRoom: (nickname: string) => void;
  onJoinRoom: (nickname: string, code: string) => void;
  busy: boolean;
}

export function RoomJoin({ onCreateRoom, onJoinRoom, busy }: RoomJoinProps) {
  const [mode, setMode] = useState<'choose' | 'create' | 'join'>('choose');
  const [nickname, setNickname] = useState('');
  const [code, setCode] = useState('');

  return (
    <div className="flex-1 flex items-center justify-center">
      <div className="w-full max-w-sm space-y-6">
        <div className="text-center space-y-1">
        </div>

        <input
          value={nickname}
          onChange={(e) => setNickname(e.target.value)}
          placeholder="Nickname (this chat only)"
          className="w-full bg-zinc-900 border border-zinc-700 rounded-md px-3 py-2 text-sm outline-none focus:border-blue-500"
        />

        {mode === 'choose' && (
          <div className="flex flex-col gap-2">
            <Button disabled={!nickname || busy} onClick={() => setMode('create')}>Create a room</Button>
            <Button variant="ghost" disabled={!nickname || busy} onClick={() => setMode('join')}>Join with a code</Button>
          </div>
        )}

        {mode === 'create' && (
          <div className="flex flex-col gap-2">
            <p className="text-xs text-zinc-500">A one-time code will be generated for you to share.</p>
            <Button disabled={busy} onClick={() => onCreateRoom(nickname)}>Generate code</Button>
            <Button variant="ghost" onClick={() => setMode('choose')}>Back</Button>
          </div>
        )}

        {mode === 'join' && (
          <div className="flex flex-col gap-2">
            <input
              value={code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
              placeholder="Enter room code"
              className="w-full bg-zinc-900 border border-zinc-700 rounded-md px-3 py-2 text-sm tracking-widest text-center outline-none focus:border-blue-500"
              maxLength={8}
            />
            <Button disabled={!code || busy} onClick={() => onJoinRoom(nickname, code)}>Join room</Button>
            <Button variant="ghost" onClick={() => setMode('choose')}>Back</Button>
          </div>
        )}
      </div>
    </div>
  );
}