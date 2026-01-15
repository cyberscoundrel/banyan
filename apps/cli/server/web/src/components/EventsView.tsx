import { useEffect, useRef } from 'react';
import { Event } from '../types';
import { Terminal } from 'lucide-react';

interface EventsViewProps {
  events: Event[];
}

const eventColors: Record<string, string> = {
  peer_connected: 'text-green-400',
  peer_disconnected: 'text-red-400',
  peer_tracked: 'text-blue-400',
  http_test: 'text-yellow-400',
  error: 'text-red-500',
  info: 'text-gray-400',
  node_started: 'text-green-500',
  service_beacon_started: 'text-purple-400',
};

export default function EventsView({ events }: EventsViewProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [events]);

  if (events.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-64 text-gray-400">
        <Terminal className="w-16 h-16 mb-4 opacity-50" />
        <p>No events yet</p>
        <p className="text-sm mt-2">Events will appear here as they occur</p>
      </div>
    );
  }

  return (
    <div className="bg-gray-800 rounded-lg p-4 h-full border border-banyan-700">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <Terminal className="w-5 h-5 text-accent-500" />
          Event Log
        </h2>
        <span className="text-sm text-gray-400">{events.length} events</span>
      </div>
      
      <div 
        ref={containerRef}
        className="terminal-font text-sm space-y-1 overflow-auto max-h-[calc(100vh-350px)]"
      >
        {events.map((event, i) => {
          const time = new Date(event.timestamp).toLocaleTimeString();
          const colorClass = eventColors[event.type] || 'text-gray-300';
          
          return (
            <div key={i} className="flex gap-2">
              <span className="text-gray-500 flex-shrink-0">[{time}]</span>
              <span className={`${colorClass} flex-shrink-0`}>{event.type}:</span>
              <span className="text-gray-300 break-all">
                {typeof event.data === 'object' 
                  ? JSON.stringify(event.data) 
                  : String(event.data)}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

