import { findAndReplace } from 'mdast-util-find-and-replace'
import type { Root } from 'mdast'

// remarkMentions marks each @name of a known participant, so the renderer
// can style it. The mark is an emphasis node renamed to a span: a text node
// would merge with the text that follows it.
export function remarkMentions(names: ReadonlySet<string>) {
  return () => (tree: Root) => {
    findAndReplace(tree, [
      /(?<![\w@])@([a-z][a-z0-9-]*)/g,
      (match: string, name: string) =>
        names.has(name) && {
          type: 'emphasis',
          data: { hName: 'span', hProperties: { className: ['mention'] } },
          children: [{ type: 'text', value: match }],
        },
    ])
  }
}
