import { ChatMessage, Peer, FileTransferItem } from '../types';

export const mockPeer: Peer = { id: 'peer-ephemeral-1', nickname: 'Anon' };
export const mockSelf: Peer = { id: 'self-ephemeral-1', nickname: 'You' };

export const mockMessages: ChatMessage[] = [
  { id: '1', sender: mockPeer, content: 'mock text 1', timestamp: Date.now() - 60000, isOwn: false },
  { id: '2', sender: mockSelf, content: 'mock text 2', timestamp: Date.now() - 45000, isOwn: true },
  { id: '3', sender: mockPeer, content: 'mock text 3', timestamp: Date.now() - 20000, isOwn: false },
];

export const mockTransfer: FileTransferItem = {
  id: 't1',
  fileName: 'notes.pdf',
  fileSize: 2_400_000,
  direction: 'receiving',
  progress: 62,
  status: 'in-progress',
};