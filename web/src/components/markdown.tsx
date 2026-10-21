import { useMemo } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkBreaks from 'remark-breaks'
import remarkGfm from 'remark-gfm'
import { remarkMentions } from '@/lib/remark-mentions'

// Markdown renders agent text. A single newline is a line break, as in chat.
// react-markdown never renders raw HTML, so a message cannot inject markup.
export function Markdown({ text, names }: { text: string; names: ReadonlySet<string> }) {
  const plugins = useMemo(() => [remarkGfm, remarkBreaks, remarkMentions(names)], [names])
  return (
    <div className="prose prose-sm prose-neutral max-w-none break-words prose-p:my-1 prose-ul:my-1 prose-ol:my-1 prose-li:my-0 prose-headings:mb-1 prose-headings:mt-3 prose-headings:text-sm prose-pre:my-2 prose-pre:bg-neutral-50 prose-pre:text-neutral-800 prose-pre:border prose-code:rounded prose-code:bg-neutral-100 prose-code:px-1 prose-code:py-0.5 prose-code:font-normal prose-code:before:content-none prose-code:after:content-none [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_.mention]:rounded [&_.mention]:bg-neutral-100 [&_.mention]:px-1 [&_.mention]:font-medium [&_.mention]:text-neutral-900">
      <ReactMarkdown
        remarkPlugins={plugins}
        components={{ a: (props) => <a {...props} target="_blank" rel="noreferrer" /> }}
      >
        {text}
      </ReactMarkdown>
    </div>
  )
}
