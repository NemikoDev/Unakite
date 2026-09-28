import { ReactNode } from 'react';

export function Layout({ children }: { children: ReactNode }) {
  return (
    <div className="h-screen w-screen bg-zinc-950 text-zinc-200 flex flex-col">
      {children}
    </div>
  );
}