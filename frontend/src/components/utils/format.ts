// Formats a file size in bytes into a human-readable string with appropriate units (B, KB, MB, GB)
export function formatSize(bytes: number): string {
  if (bytes < 1000) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB'];
  let value = bytes;
  let i = -1;
  do {
    value /= 1000;
    i++;
  } while (value >= 1000 && i < units.length - 1);
  return `${value.toFixed(value < 10 ? 1 : 0)} ${units[i]}`;
}