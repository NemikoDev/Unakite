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
}

export interface FileTransferItem {
  id: string;
  fileName: string;
  fileSize: number;
  direction: 'sending' | 'receiving';
  progress: number; 
  status: 'in-progress' | 'complete' | 'failed';
}

export type RoomStatus = 'idle' | 'creating' | 'joining' | 'connected';

export interface RoomState {
  status: RoomStatus;
  roomCode: string | null;
  self: Peer | null;
  peer: Peer | null;
}