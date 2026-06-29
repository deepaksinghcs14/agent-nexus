'use client'

import CodeMirror, { type ReactCodeMirrorProps } from '@uiw/react-codemirror'
import { javascript } from '@codemirror/lang-javascript'

type Props = Omit<ReactCodeMirrorProps, 'extensions'> & {
  value: string
  onChange: (value: string) => void
}

export default function CodeEditor(props: Props) {
  return <CodeMirror {...props} extensions={[javascript()]} />
}
