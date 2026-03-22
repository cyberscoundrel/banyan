import React from 'react';
import { useParams, Navigate } from 'react-router-dom';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

const docFiles: Record<string, () => Promise<{ default: string }>> = {
  types: () => import('../../../node/docs/generated/types.md?raw').then(m => ({ default: m.default as string })),
  interfaces: () => import('../../../node/docs/generated/interfaces.md?raw').then(m => ({ default: m.default as string })),
  crypto: () => import('../../../node/docs/generated/crypto.md?raw').then(m => ({ default: m.default as string })),
  discovery: () => import('../../../node/docs/generated/discovery.md?raw').then(m => ({ default: m.default as string })),
  connection: () => import('../../../node/docs/generated/connection.md?raw').then(m => ({ default: m.default as string })),
  service: () => import('../../../node/docs/generated/service.md?raw').then(m => ({ default: m.default as string })),
  events: () => import('../../../node/docs/generated/events.md?raw').then(m => ({ default: m.default as string })),
  tunnel: () => import('../../../node/docs/generated/tunnel.md?raw').then(m => ({ default: m.default as string })),
  'libp2p-http': () => import('../../../node/docs/generated/libp2p-http.md?raw').then(m => ({ default: m.default as string })),
  'management-server': () => import('../../../node/docs/generated/management-server.md?raw').then(m => ({ default: m.default as string })),
  node: () => import('../../../node/docs/generated/node.md?raw').then(m => ({ default: m.default as string })),
  'addon': () => import('../../../node/docs/generated/addon.md?raw').then(m => ({ default: m.default as string })),
  'addon/sdk': () => import('../../../node/docs/generated/addon/sdk.md?raw').then(m => ({ default: m.default as string })),
  'addon/proxy': () => import('../../../node/docs/generated/addon/proxy.md?raw').then(m => ({ default: m.default as string })),
  common: () => import('../../../node/docs/generated/common.md?raw').then(m => ({ default: m.default as string })),
  transport: () => import('../../../node/docs/generated/transport.md?raw').then(m => ({ default: m.default as string })),
  websocket: () => import('../../../node/docs/generated/websocket.md?raw').then(m => ({ default: m.default as string })),
  'management-server/handlers': () => import('../../../node/docs/generated/management-server/handlers.md?raw').then(m => ({ default: m.default as string })),
};

const PackageDoc: React.FC = () => {
  const { '*' : packageName } = useParams<{ '*' : string }>();
  const [content, setContent] = React.useState<string>('');
  const [loading, setLoading] = React.useState(true);
  const [notFound, setNotFound] = React.useState(false);

  React.useEffect(() => {
    const key = packageName || '';
    if (!docFiles[key]) {
      setNotFound(true);
      setLoading(false);
      return;
    }

    setLoading(true);
    setNotFound(false);
    docFiles[key]()
      .then(module => {
        setContent(module.default);
        setLoading(false);
      })
      .catch(() => {
        setNotFound(true);
        setLoading(false);
      });
  }, [packageName]);

  if (notFound) {
    return <Navigate to="/docs/api" replace />;
  }

  if (loading) {
    return (
      <div className="prose">
        <p className="text-gray-600 dark:text-gray-400">Loading documentation...</p>
      </div>
    );
  }

  return (
    <div className="prose prose-gray dark:prose-invert max-w-none">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
    </div>
  );
};

export default PackageDoc;
