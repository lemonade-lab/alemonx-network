import { useMemo, useState, type FormEvent, type ReactNode } from 'react'
import { runActionAndPoll, type ActionResult } from './api'
import { splitStatusLines, type StatusLine } from './status'

type View = 'diagnostics' | 'forwarding' | 'firewall'

const statusColor: Record<StatusLine['kind'], string> = {
  ok: 'text-[var(--theme-success-text)]',
  fail: 'text-[var(--theme-danger-text)]',
  warn: 'text-[var(--theme-warning-text)]',
  plain: 'text-[var(--theme-text-primary)]'
}

const statusSymbol: Record<StatusLine['kind'], string> = {
  ok: '✓',
  fail: '!',
  warn: '?',
  plain: ''
}

function StatusLineRow({ line }: { line: StatusLine }) {
  return (
    <div
      className={
        'grid grid-cols-[1.125rem_1fr] items-baseline gap-1.5 py-1 text-[11px] leading-5 ' +
        statusColor[line.kind]
      }
    >
      <span className="text-center font-bold" aria-hidden>
        {statusSymbol[line.kind]}
      </span>
      <span
        className={
          line.kind === 'plain' ? 'whitespace-pre-wrap font-mono' : undefined
        }
      >
        {line.text || ' '}
      </span>
    </div>
  )
}

function ResultPanel({
  state,
  title
}: {
  state: 'idle' | 'running' | 'done' | 'failed'
  title: string
  result?: ActionResult
}) {
  const lines = useMemo(
    () => splitStatusLines(result?.output ?? ''),
    [result]
  )
  if (state === 'idle') return null
  return (
    <section className="grid gap-2 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3 text-xs">
      <header className="flex items-center justify-between gap-3">
        <strong className="flex items-center gap-1.5 text-sm font-semibold text-[var(--theme-text-strong)]">
          {state === 'running' && (
            <span className="size-3.5 animate-spin rounded-full border-2 border-[var(--theme-border-strong)] border-t-[var(--theme-accent)]" />
          )}
          {state === 'running'
            ? '正在执行…'
            : state === 'failed'
              ? `${title}未完成`
              : `${title}完成`}
        </strong>
      </header>
      {state === 'failed' && result?.error && (
        <div className="rounded-md bg-[var(--theme-danger-soft)] px-2 py-1.5 font-semibold text-[var(--theme-danger-text)]">
          {result.error}
        </div>
      )}
      {state === 'running' && !result?.output ? (
        <div className="grid gap-2 py-1">
          <div className="h-3 w-2/3 animate-pulse rounded bg-[var(--theme-surface-hover)]" />
          <div className="h-3 w-1/2 animate-pulse rounded bg-[var(--theme-surface-hover)]" />
        </div>
      ) : (
        <div className="grid">
          {lines.map((line, index) => (
            <StatusLineRow key={index} line={line} />
          ))}
        </div>
      )}
    </section>
  )
}

// Modal-based confirmation for dangerous actions (never window.confirm).
function ConfirmModal({
  title,
  description,
  detail,
  onConfirm,
  onCancel
}: {
  title: string
  description: string
  detail: string
  onConfirm: () => void
  onCancel: () => void
}) {
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-[var(--theme-surface-overlay)]">
      <div className="w-[min(460px,calc(100vw-32px))] rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-4 shadow-[var(--theme-shadow-pop)]">
        <h3 className="m-0 mb-1 text-[15px] font-semibold text-[var(--theme-text-strong)]">
          {title}
        </h3>
        <p className="m-0 mb-2 text-[13px] text-[var(--theme-text-muted)]">
          {description}
        </p>
        <pre className="m-0 mb-3 rounded-md bg-[var(--theme-surface-hover)] p-2 text-[11px] leading-5 text-[var(--theme-text-secondary)]">
          {detail}
        </pre>
        <div className="flex justify-end gap-2">
          <button className="secondary-button" onClick={onCancel}>
            取消
          </button>
          <button className="danger-button" onClick={onConfirm}>
            确认执行
          </button>
        </div>
      </div>
    </div>
  )
}

function Field({
  label,
  name,
  type = 'text',
  defaultValue,
  children,
  hint
}: {
  label: string
  name: string
  type?: string
  defaultValue?: string
  children?: ReactNode
  hint?: string
}) {
  return (
    <label className="grid min-w-[180px] flex-1 gap-1 text-xs font-semibold text-[var(--theme-text-secondary)]">
      {label}
      {children ?? (
        <input
          className="min-h-9 rounded-control border border-[var(--theme-border-default)] bg-[var(--theme-surface-input)] px-2.5 text-sm font-normal text-[var(--theme-text-primary)] outline-none focus:border-[var(--theme-accent)]"
          type={type}
          name={name}
          defaultValue={defaultValue}
        />
      )}
      {hint && (
        <span className="font-normal leading-4 text-[var(--theme-text-muted)]">
          {hint}
        </span>
      )}
    </label>
  )
}

export default function App() {
  const [view, setView] = useState<View>('diagnostics')
  const [result, setResult] = useState<ActionResult | undefined>()
  const [state, setState] = useState<'idle' | 'running' | 'done' | 'failed'>(
    'idle'
  )
  const [pendingConfirm, setPendingConfirm] = useState<{
    title: string
    description: string
    detail: string
    action: () => Promise<void>
  } | null>(null)

  const run = async (action: string, params: Record<string, string>, confirm = false) => {
    setState('running')
    setResult(undefined)
    try {
      const outcome = await runActionAndPoll(action, params, confirm)
      setResult(outcome)
      setState(outcome.error ? 'failed' : 'done')
    } catch (reason) {
      setResult({ output: '', error: reason instanceof Error ? reason.message : String(reason) })
      setState('failed')
    }
  }

  const tabs: Array<{ id: View; label: string }> = [
    { id: 'diagnostics', label: '诊断' },
    { id: 'forwarding', label: '端口转发' },
    { id: 'firewall', label: '防火墙' }
  ]

  return (
    <div className="mx-auto grid max-w-[860px] gap-4 p-4">
      <header className="flex items-baseline justify-between gap-3 border-b border-[var(--theme-border-default)] pb-3">
        <div>
          <h1 className="m-0 text-base font-semibold tracking-tight text-[var(--theme-text-strong)]">
            网络、端口转发与防火墙
          </h1>
          <p className="m-0 mt-0.5 text-xs text-[var(--theme-text-muted)]">
            ALemonX 系统插件界面
          </p>
        </div>
      </header>

      <nav className="flex gap-1 border-b border-[var(--theme-border-default)] pb-2">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            className={
              'min-h-8 rounded-control border px-3 text-xs font-semibold transition ' +
              (view === tab.id
                ? 'border-[var(--theme-border-strong)] bg-[var(--theme-surface-panel)] text-[var(--theme-text-strong)]'
                : 'border-transparent text-[var(--theme-text-muted)] hover:bg-[var(--theme-surface-hover)] hover:text-[var(--theme-text-strong)]')
            }
            onClick={() => {
              setView(tab.id)
              setState('idle')
              setResult(undefined)
            }}
          >
            {tab.label}
          </button>
        ))}
      </nav>

      {view === 'diagnostics' && (
        <div className="grid gap-2">
          <div className="flex flex-wrap gap-2">
            <button
              className="primary-button"
              disabled={state === 'running'}
              onClick={() => void run('network-check')}
            >
              检查网络
            </button>
            <button
              className="secondary-button"
              disabled={state === 'running'}
              onClick={() => void run('firewall-status')}
            >
              查看防火墙
            </button>
            <button
              className="secondary-button"
              disabled={state === 'running'}
              onClick={() => void run('mirror-check')}
            >
              检查下载源
            </button>
          </div>
          <ResultPanel state={state} title="检查" result={result} />
        </div>
      )}

      {view === 'forwarding' && (
        <div className="grid gap-3">
          <button
            className="secondary-button w-fit"
            disabled={state === 'running'}
            onClick={() => void run('forward-list')}
          >
            刷新转发列表
          </button>
          <ResultPanel state={state} title="端口转发" result={result} />

          <form
            className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3"
            onSubmit={(event: FormEvent<HTMLFormElement>) => {
              event.preventDefault()
              const data = new FormData(event.currentTarget)
              const params = {
                listenPort: String(data.get('listenPort') || ''),
                targetIP: String(data.get('targetIP') || ''),
                protocol: String(data.get('protocol') || 'tcp'),
                targetPort: String(data.get('targetPort') || '')
              }
              setPendingConfirm({
                title: '添加端口转发',
                description: `把本机 ${params.listenPort} 端口收到的连接，转发到 ${params.targetIP}。这会开放一个入站端口。`,
                detail: `本机端口 ${params.listenPort} → ${params.targetIP}:${params.targetPort || params.listenPort}（${params.protocol}）`,
                action: () => run('forward-add', params, true)
              })
            }}
          >
            <Field label="本机端口" name="listenPort" type="number" defaultValue="17117" />
            <Field label="目标设备 IP" name="targetIP" hint="接收转发的设备地址，如 192.168.1.100。" />
            <Field label="目标端口（留空=同本机端口）" name="targetPort" type="number" />
            <Field label="协议" name="protocol">
              <select
                className="min-h-9 rounded-control border border-[var(--theme-border-default)] bg-[var(--theme-surface-input)] px-2.5 text-sm font-normal text-[var(--theme-text-primary)] outline-none focus:border-[var(--theme-accent)]"
                name="protocol"
                defaultValue="tcp"
              >
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
              </select>
            </Field>
            <button className="primary-button self-end" type="submit">
              添加
            </button>
          </form>
        </div>
      )}

      {view === 'firewall' && (
        <div className="grid gap-3">
          <form
            className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3"
            onSubmit={(event: FormEvent<HTMLFormElement>) => {
              event.preventDefault()
              const data = new FormData(event.currentTarget)
              const params = {
                port: String(data.get('port') || ''),
                protocol: String(data.get('protocol') || 'tcp')
              }
              setPendingConfirm({
                title: '开放入站端口',
                description: '允许外部设备访问本机的指定端口。',
                detail: `端口 ${params.port}/${params.protocol}`,
                action: () => run('open-port', params, true)
              })
            }}
          >
            <Field label="端口" name="port" type="number" defaultValue="17117" />
            <Field label="协议" name="protocol">
              <select
                className="min-h-9 rounded-control border border-[var(--theme-border-default)] bg-[var(--theme-surface-input)] px-2.5 text-sm font-normal text-[var(--theme-text-primary)] outline-none focus:border-[var(--theme-accent)]"
                name="protocol"
                defaultValue="tcp"
              >
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
              </select>
            </Field>
            <button className="primary-button self-end" type="submit">
              开放端口
            </button>
          </form>

          <form
            className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3"
            onSubmit={(event: FormEvent<HTMLFormElement>) => {
              event.preventDefault()
              const data = new FormData(event.currentTarget)
              const params = {
                port: String(data.get('port') || ''),
                protocol: String(data.get('protocol') || 'tcp')
              }
              setPendingConfirm({
                title: '关闭入站端口',
                description: '移除本插件之前开放的端口规则。',
                detail: `端口 ${params.port}/${params.protocol}`,
                action: () => run('close-port', params, true)
              })
            }}
          >
            <Field label="端口" name="port" type="number" defaultValue="17117" />
            <Field label="协议" name="protocol">
              <select
                className="min-h-9 rounded-control border border-[var(--theme-border-default)] bg-[var(--theme-surface-input)] px-2.5 text-sm font-normal text-[var(--theme-text-primary)] outline-none focus:border-[var(--theme-accent)]"
                name="protocol"
                defaultValue="tcp"
              >
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
              </select>
            </Field>
            <button className="danger-button self-end" type="submit">
              关闭端口
            </button>
          </form>

          <ResultPanel state={state} title="防火墙" result={result} />
        </div>
      )}

      {pendingConfirm && (
        <ConfirmModal
          title={pendingConfirm.title}
          description={pendingConfirm.description}
          detail={pendingConfirm.detail}
          onConfirm={() => {
            void pendingConfirm.action()
            setPendingConfirm(null)
          }}
          onCancel={() => setPendingConfirm(null)}
        />
      )}
    </div>
  )
}
