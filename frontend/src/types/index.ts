export interface Peer {
  id: string;
  nickname: string;
}

export interface ChatMessage {
  id: string;
  sender: Peer;
  content: string;
  timestamp: number;
  isOwn: boolean;
  kind?: 'system';
}

export interface FileTransferItem {
  id: string;
  fileName: string;
  fileSize: number;
  direction: 'sending' | 'receiving';
  progress: number; 
  status: 'awaiting' | 'in-progress' | 'complete' | 'failed' | 'rejected';
  error?: string;
  path?: string;
  startedAt: number;
}

export type RoomStatus = 'idle' | 'creating' | 'joining' | 'connected';

export interface RoomState {
  status: RoomStatus;
  roomCode: string | null;
  self: Peer | null;
  peer: Peer | null;
}

export interface PeerLeft extends Peer {
  closesAt: number;
}