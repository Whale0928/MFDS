import { hintFor } from '../glossary'

// 용어 옆에 각주를 붙인다. 마우스를 올리거나 키보드 초점이 가면 설명이 뜬다.
export function Hint({ text, children }: { text: string; children: React.ReactNode }) {
  if (!text) return <>{children}</>
  return <span className="hint" tabIndex={0} role="note" aria-label={text}>
    {children}
    <span className="hint__bubble">{text}</span>
  </span>
}

// 용어사전에 등록된 설명을 자동으로 찾아 붙인다.
export function Term({ children }: { children: string }) {
  return <Hint text={hintFor(children)}>{children}</Hint>
}
