import React from 'react';
import { Copy, FileText, ChevronDown, ExternalLink } from 'lucide-react';
import { useLocation } from '@docusaurus/router';
import useIsBrowser from '@docusaurus/useIsBrowser';
import styles from './styles.module.css';

type ExternalAction = {
  label: string;
  href: string;
};

type DocMarkdownActionsProps = {
  className?: string;
};

async function copyToClipboard(text: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }

  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.style.position = 'fixed';
  textarea.style.top = '0';
  textarea.style.left = '0';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  document.execCommand('copy');
  document.body.removeChild(textarea);
}

export default function DocMarkdownActions({ className }: DocMarkdownActionsProps) {
  const { pathname } = useLocation();
  const isBrowser = useIsBrowser();
  const [copyState, setCopyState] = React.useState<'idle' | 'copied' | 'error'>('idle');

  const mdPath = React.useMemo(() => {
    const cleanPath = pathname.endsWith('/') && pathname !== '/' ? pathname.slice(0, -1) : pathname;
    return `${cleanPath}.md`;
  }, [pathname]);

  const pageUrl = isBrowser ? window.location.href.split('#')[0] : '';
  const prompt = isBrowser
    ? `Could you read this document about Agent Manager ${pageUrl} so I can ask questions about it?`
    : '';
  const query = encodeURIComponent(prompt);

  const externalActions: ExternalAction[] = isBrowser
    ? [
        { label: 'Open in ChatGPT', href: `https://chatgpt.com/?q=${query}` },
        { label: 'Open in Claude', href: `https://claude.ai/new?q=${query}` },
        { label: 'Open in Perplexity', href: `https://www.perplexity.ai/?q=${query}` },
      ]
    : [];

  const handleCopy = async () => {
    try {
      const response = await fetch(mdPath, { credentials: 'same-origin' });
      if (!response.ok) throw new Error('Failed to fetch markdown');
      const text = await response.text();
      await copyToClipboard(text);
      setCopyState('copied');
      window.setTimeout(() => setCopyState('idle'), 1500);
    } catch (error) {
      console.error(error);
      setCopyState('error');
      window.setTimeout(() => setCopyState('idle'), 2000);
    }
  };

  const containerClassName = className ? `${styles.container} ${className}` : styles.container;

  return (
    <div className={containerClassName}>
      <details className={styles.dropdown}>
        <summary className={styles.summary} aria-label="Copy page">
          <Copy size={14} />
          <span>Copy page</span>
          <ChevronDown size={14} className={styles.chevron} />
        </summary>
        <div className={styles.menu} role="menu">
          <button className={styles.item} onClick={handleCopy} type="button" role="menuitem">
            <Copy size={16} />
            <span>
              <strong>Copy page</strong>
              <small>
                {copyState === 'copied'
                  ? 'Copied!'
                  : copyState === 'error'
                  ? 'Copy failed'
                  : 'Copy page as Markdown for LLMs'}
              </small>
            </span>
          </button>
          <a className={styles.item} href={mdPath} target="_blank" rel="noreferrer" role="menuitem">
            <FileText size={16} />
            <span>
              <strong>View as Markdown</strong>
              <small>View this page as plain text</small>
            </span>
          </a>
          <div className={styles.separator} role="separator" />
          {externalActions.map((action) => (
            <a
              key={action.label}
              className={styles.item}
              href={action.href}
              target="_blank"
              rel="noreferrer"
              role="menuitem"
            >
              <ExternalLink size={16} />
              <span>
                <strong>{action.label}</strong>
                <small>Ask questions about this page</small>
              </span>
            </a>
          ))}
        </div>
      </details>
    </div>
  );
}
