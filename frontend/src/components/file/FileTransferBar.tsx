import { FileTransferItem } from '../../types';

export function FileTransferBar({ item }: { item: FileTransferItem }) {
  const sizeMB = (item.fileSize / 1_000_000).toFixed(1);
  return (
    <div className="bg-zinc-900 border border-zinc-700 rounded-md px-3 py-2 max-w-xs">
      <div className="flex justify-between text-xs text-zinc-400 mb-1">
        <span>{item.direction === 'receiving' ? '↓' : '↑'} {item.fileName}</span>
        <span>{sizeMB} MB</span>
      </div>
      <div className="h-1.5 bg-zinc-800 rounded-full overflow-hidden">
        <div className="h-full bg-blue-500 transition-all" style={{ width: `${item.progress}%` }} />
      </div>
    </div>
  );
}