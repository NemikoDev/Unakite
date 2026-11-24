import { FileTransferItem } from '../../types';
import { formatSize } from '../utils/format';
import { Button } from '../Button';

interface Props {
  item: FileTransferItem;
  onAccept: (id: string) => void;
  onDecline: (id: string) => void;
}

export function FileTransferBar({ item, onAccept, onDecline }: Props) {
  const incoming = item.direction === 'receiving';
  const arrow = incoming ? '↓' : '↑';

  return (
    <div className="bg-zinc-900 border border-zinc-700 rounded-md px-3 py-2 w-72 max-w-full">
      <div className="flex justify-between gap-2 text-xs text-zinc-300 mb-1">
        <span className="truncate" title={item.fileName}>{arrow} {item.fileName}</span>
        <span className="shrink-0 text-zinc-500">{formatSize(item.fileSize)}</span>
      </div>

      {item.status === 'awaiting' && incoming && (
        <div className="space-y-2">
          <p className="text-xs text-zinc-400">Wants to send you this file.</p>
          <div className="flex gap-2">
            <Button onClick={() => onAccept(item.id)}>Accept</Button>
            <Button variant="ghost" onClick={() => onDecline(item.id)}>Decline</Button>
          </div>
        </div>
      )}

      {item.status === 'awaiting' && !incoming && (
        <p className="text-xs text-zinc-500">Waiting for them to accept…</p>
      )}

      {item.status === 'in-progress' && (
        <>
          <div className="h-1.5 bg-zinc-800 rounded-full overflow-hidden">
            <div className="h-full bg-blue-500 transition-all" style={{ width: `${item.progress}%` }} />
          </div>
          <p className="text-[10px] text-zinc-500 mt-1">{item.progress}%</p>
        </>
      )}

      {item.status === 'complete' && (
        <p className="text-xs text-emerald-400 break-all">
          {incoming ? `Saved to ${item.path}` : 'Sent and verified'}
        </p>
      )}
      {item.status === 'rejected' && (
        <p className="text-xs text-zinc-500">{incoming ? 'Declined' : 'They did not accept this file'}</p>
      )}
      {item.status === 'failed' && (
        <p className="text-xs text-red-400 break-words">Failed: {item.error || 'unknown error'}</p>
      )}
    </div>
  );
}