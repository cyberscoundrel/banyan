import { } from 'react';
import { HelpCircle, Terminal, Keyboard } from 'lucide-react';

export default function HelpView() {
  return (
    <div className="space-y-6 max-w-4xl">
      <div className="bg-gray-800 rounded-lg p-6 border border-banyan-700">
        <h2 className="text-xl font-semibold mb-4 flex items-center gap-2">
          <HelpCircle className="w-6 h-6 text-accent-500" />
          Banyan CLI Help
        </h2>
        
        <div className="space-y-6">
          {/* Navigation */}
          <section>
            <h3 className="text-lg font-semibold mb-2 flex items-center gap-2">
              <Keyboard className="w-5 h-5 text-gray-400" />
              Navigation
            </h3>
            <div className="bg-gray-700 rounded p-4 space-y-1 text-sm">
              <KeyCommand keys={['Tab']} description="Switch between views" />
              <KeyCommand keys={['1', '2', '3', '4', '5']} description="Jump to specific view" />
              <KeyCommand keys={['↑', '↓']} description="Navigate command history" />
            </div>
          </section>

          {/* Connection Commands */}
          <section>
            <h3 className="text-lg font-semibold mb-2 flex items-center gap-2">
              <Terminal className="w-5 h-5 text-gray-400" />
              Connection Commands
            </h3>
            <CommandList commands={[
              { cmd: 'connect <address>', desc: 'Connect to a node (e.g., http://localhost:8080)' },
              { cmd: 'status', desc: 'Refresh node status' },
            ]} />
          </section>

          {/* Peer Commands */}
          <section>
            <h3 className="text-lg font-semibold mb-2">Peer Commands</h3>
            <CommandList commands={[
              { cmd: 'peers', desc: 'List all connected peers' },
              { cmd: 'peer connect <id>', desc: 'Connect to a peer by ID or alias' },
              { cmd: 'add <id> <addr>', desc: 'Add a peer with multiaddr [--connect]' },
            ]} />
          </section>

          {/* Service Commands */}
          <section>
            <h3 className="text-lg font-semibold mb-2">Service Commands</h3>
            <CommandList commands={[
              { cmd: 'services', desc: 'List configured services' },
              { cmd: 'figs', desc: 'List service figs' },
              { cmd: 'find <key|alias>', desc: 'Find a service' },
              { cmd: 'serve start <file>', desc: 'Start service beacon' },
              { cmd: 'serve stop [hash]', desc: 'Stop service beacon' },
            ]} />
          </section>

          {/* Proxy Commands */}
          <section>
            <h3 className="text-lg font-semibold mb-2">Proxy Commands</h3>
            <CommandList commands={[
              { cmd: 'proxy peer <id> <path> [method]', desc: 'Proxy request through peer' },
              { cmd: 'proxy service <key> <path> [method]', desc: 'Proxy request through service' },
            ]} />
          </section>

          {/* Other Commands */}
          <section>
            <h3 className="text-lg font-semibold mb-2">Other Commands</h3>
            <CommandList commands={[
              { cmd: 'route list', desc: 'List configured routes' },
              { cmd: 'route add <p> <t>', desc: 'Add a route (path -> target)' },
              { cmd: 'clear', desc: 'Clear event log' },
              { cmd: 'help', desc: 'Show this help' },
            ]} />
          </section>
        </div>
      </div>

      {/* Views */}
      <div className="bg-gray-800 rounded-lg p-6 border border-banyan-700">
        <h3 className="text-lg font-semibold mb-3">Views</h3>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <ViewCard num="1" name="Status" desc="Node status and statistics" />
          <ViewCard num="2" name="Events" desc="Live WebSocket event log" />
          <ViewCard num="3" name="Peers" desc="Connected peers list" />
          <ViewCard num="4" name="Services" desc="Service management" />
          <ViewCard num="5" name="Help" desc="This help screen" />
        </div>
      </div>
    </div>
  );
}

function KeyCommand({ keys, description }: { keys: string[]; description: string }) {
  return (
    <div className="flex items-center gap-2">
      <div className="flex gap-1">
        {keys.map((key) => (
          <kbd key={key} className="px-2 py-0.5 bg-gray-600 rounded text-xs">{key}</kbd>
        ))}
      </div>
      <span className="text-gray-400">- {description}</span>
    </div>
  );
}

function CommandList({ commands }: { commands: { cmd: string; desc: string }[] }) {
  return (
    <div className="bg-gray-700 rounded p-4 space-y-2 text-sm border border-banyan-800">
      {commands.map(({ cmd, desc }) => (
        <div key={cmd} className="flex gap-4">
          <code className="terminal-font text-accent-500 w-48 flex-shrink-0">{cmd}</code>
          <span className="text-gray-400">{desc}</span>
        </div>
      ))}
    </div>
  );
}

function ViewCard({ num, name, desc }: { num: string; name: string; desc: string }) {
  return (
    <div className="bg-gray-700 rounded p-3 flex items-center gap-3">
      <kbd className="px-2 py-1 bg-gray-600 rounded text-sm">{num}</kbd>
      <div>
        <p className="font-semibold">{name}</p>
        <p className="text-xs text-gray-400">{desc}</p>
      </div>
    </div>
  );
}

