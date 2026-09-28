import { ButtonHTMLAttributes } from 'react';

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'ghost';
}

export function Button({ variant = 'primary', className = '', ...props }: ButtonProps) {
  const base = 'px-4 py-2 rounded-md text-sm font-medium transition-colors disabled:opacity-40 disabled:cursor-not-allowed';
  const variants = {
    primary: 'bg-blue-600 hover:bg-blue-500 text-white',
    ghost: 'bg-transparent hover:bg-zinc-800 text-zinc-300 border border-zinc-700',
  };
  return <button className={`${base} ${variants[variant]} ${className}`} {...props} />;
}