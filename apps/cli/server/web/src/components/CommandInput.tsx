import React, { useState, useRef, useEffect } from 'react';
import { ChevronRight } from 'lucide-react';

interface CommandInputProps {
  onSubmit: (command: string) => void;
}

export default function CommandInput({ onSubmit }: CommandInputProps) {
  const [value, setValue] = useState('');
  const [history, setHistory] = useState<string[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    // Focus input on mount
    inputRef.current?.focus();
  }, []);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!value.trim()) return;
    
    setHistory(prev => [...prev, value]);
    setHistoryIndex(-1);
    onSubmit(value);
    setValue('');
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (history.length > 0) {
        const newIndex = historyIndex === -1 ? history.length - 1 : Math.max(0, historyIndex - 1);
        setHistoryIndex(newIndex);
        setValue(history[newIndex] || '');
      }
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (historyIndex >= 0) {
        const newIndex = historyIndex + 1;
        if (newIndex >= history.length) {
          setHistoryIndex(-1);
          setValue('');
        } else {
          setHistoryIndex(newIndex);
          setValue(history[newIndex] || '');
        }
      }
    }
  };

  return (
    <form onSubmit={handleSubmit} className="flex items-center gap-2">
      <ChevronRight className="w-5 h-5 text-accent-500 flex-shrink-0" />
      <input
        ref={inputRef}
        type="text"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Enter command (type 'help' for available commands)"
        className="flex-1 bg-gray-700 rounded px-3 py-2 text-sm terminal-font outline-none border border-banyan-800 focus:border-accent-500"
        autoComplete="off"
        spellCheck={false}
      />
    </form>
  );
}

