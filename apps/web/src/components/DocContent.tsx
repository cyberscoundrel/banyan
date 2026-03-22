import React from 'react';

interface DocContentProps {
  content: string;
  title?: string;
}

const DocContent: React.FC<DocContentProps> = ({ content, title }) => {
  return (
    <div className="prose prose-gray dark:prose-invert max-w-none">
      {title && (
        <h1 className="text-3xl font-bold text-gray-900 dark:text-white mb-6">
          {title}
        </h1>
      )}
      <div className="text-gray-600 dark:text-gray-400 leading-relaxed whitespace-pre-wrap font-mono text-sm bg-gray-50 dark:bg-gray-800 p-4 rounded-lg overflow-x-auto">
        {content}
      </div>
    </div>
  );
};

export default DocContent;
